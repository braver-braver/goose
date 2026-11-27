package lock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sethvargo/go-retry"
)

// oracleMaxUserLockID is the highest lock id DBMS_LOCK allows for user locks. Lock ids from 0 to
// 1073741823 (2^30-1) are reserved for user locks; higher ids are reserved for Oracle internal
// use. See https://docs.oracle.com/en/database/oracle/oracle-database/19/arpls/DBMS_LOCK.html
const oracleMaxUserLockID = 1 << 30

// NewOracleSessionLocker returns a SessionLocker that utilizes Oracle's DBMS_LOCK package to
// acquire an exclusive session-level user lock.
//
// The lock is requested with release_on_commit => FALSE, so it survives the transactions goose
// runs on the same connection and is held until it is explicitly released or the session ends.
// If the process dies, Oracle releases the lock automatically when the session terminates.
//
// The database user requires the EXECUTE privilege on SYS.DBMS_LOCK:
//
//	GRANT EXECUTE ON SYS.DBMS_LOCK TO <user>;
//
// Note, DBMS_LOCK restricts user lock ids to the range 0..1073741823 (2^30-1), so the configured
// lock id (including [DefaultLockID]) is reduced modulo 2^30 before use.
//
// The lock acquisition is retried until it is successfully acquired or until the failure
// threshold is reached. The default lock duration is set to 5 minutes, and the default unlock
// duration is set to 1 minute. See [SessionLockerOption] for configuration options.
func NewOracleSessionLocker(opts ...SessionLockerOption) (SessionLocker, error) {
	cfg := sessionLockerConfig{
		lockID: DefaultLockID,
		lockProbe: probe{
			intervalDuration: 5 * time.Second,
			failureThreshold: 60,
		},
		unlockProbe: probe{
			intervalDuration: 2 * time.Second,
			failureThreshold: 30,
		},
	}
	for _, opt := range opts {
		if err := opt.apply(&cfg); err != nil {
			return nil, err
		}
	}
	return &oracleSessionLocker{
		lockID: oracleUserLockID(cfg.lockID),
		retryLock: retry.WithMaxRetries(
			cfg.lockProbe.failureThreshold,
			retry.NewConstant(cfg.lockProbe.intervalDuration),
		),
		retryUnlock: retry.WithMaxRetries(
			cfg.unlockProbe.failureThreshold,
			retry.NewConstant(cfg.unlockProbe.intervalDuration),
		),
	}, nil
}

// oracleUserLockID maps an arbitrary lock id into the DBMS_LOCK user lock range 0..2^30-1.
func oracleUserLockID(lockID int64) int64 {
	return int64(uint64(lockID) % oracleMaxUserLockID)
}

type oracleSessionLocker struct {
	lockID      int64
	retryLock   retry.Backoff
	retryUnlock retry.Backoff
}

var _ SessionLocker = (*oracleSessionLocker)(nil)

func (l *oracleSessionLocker) SessionLock(ctx context.Context, conn *sql.Conn) error {
	return retry.Do(ctx, l.retryLock, func(ctx context.Context) error {
		// Request an exclusive user lock without waiting (timeout => 0), retrying in Go, which
		// mirrors the pg_try_advisory_lock behavior of the Postgres session locker.
		q := `BEGIN :1 := SYS.DBMS_LOCK.REQUEST(id => :2, lockmode => SYS.DBMS_LOCK.X_MODE, timeout => 0, release_on_commit => FALSE); END;`
		var status int64
		if _, err := conn.ExecContext(ctx, q, sql.Out{Dest: &status}, l.lockID); err != nil {
			return fmt.Errorf("failed to execute DBMS_LOCK.REQUEST: %w", err)
		}
		switch status {
		case 0, 4:
			// 0: success, 4: this session already owns the lock.
			return nil
		case 1, 2:
			// 1: timeout (lock held by another session), 2: deadlock detected. Both are
			// transient; keep retrying until the lock is acquired or retries are exhausted.
			return retry.RetryableError(errors.New("failed to acquire lock"))
		default:
			// 3: parameter error, 5: illegal lock handle.
			return fmt.Errorf("DBMS_LOCK.REQUEST returned status %d", status)
		}
	})
}

func (l *oracleSessionLocker) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	return retry.Do(ctx, l.retryUnlock, func(ctx context.Context) error {
		q := `BEGIN :1 := SYS.DBMS_LOCK.RELEASE(id => :2); END;`
		var status int64
		if _, err := conn.ExecContext(ctx, q, sql.Out{Dest: &status}, l.lockID); err != nil {
			return retry.RetryableError(fmt.Errorf("failed to execute DBMS_LOCK.RELEASE: %w", err))
		}
		switch status {
		case 0:
			return nil
		default:
			// 3: parameter error, 4: session does not own the lock, 5: illegal lock handle.
			// None of these resolve on retry. Note, unlike advisory locks in Postgres, Oracle
			// releases the lock automatically when the session ends, so a leaked lock cannot
			// outlive its connection.
			return fmt.Errorf("DBMS_LOCK.RELEASE returned status %d", status)
		}
	})
}
