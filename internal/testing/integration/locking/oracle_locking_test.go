package locking_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/braver-braver/goose/v3/internal/testing/testdb"
	"github.com/braver-braver/goose/v3/lock"
	"github.com/stretchr/testify/require"
)

func TestOracleSessionLocker(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	db, cleanup, err := testdb.NewOracle()
	require.NoError(t, err)
	t.Cleanup(cleanup)

	// Do not run subtests in parallel, because they are using the same database.

	t.Run("lock_and_unlock", func(t *testing.T) {
		locker, err := lock.NewOracleSessionLocker(
			lock.WithLockID(123456789),
			lock.WithLockTimeout(1, 4),   // 4 second timeout
			lock.WithUnlockTimeout(1, 4), // 4 second timeout
		)
		require.NoError(t, err)
		ctx := context.Background()
		conn, err := db.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, conn.Close())
		})
		err = locker.SessionLock(ctx, conn)
		require.NoError(t, err)
		// Re-acquiring on the same session must succeed (DBMS_LOCK status 4: already owned).
		err = locker.SessionLock(ctx, conn)
		require.NoError(t, err)
		err = locker.SessionUnlock(ctx, conn)
		require.NoError(t, err)
	})
	t.Run("mutual_exclusion", func(t *testing.T) {
		newLocker := func() lock.SessionLocker {
			locker, err := lock.NewOracleSessionLocker(
				lock.WithLockID(987654321),
				lock.WithLockTimeout(1, 2),   // 2 second timeout
				lock.WithUnlockTimeout(1, 2), // 2 second timeout
			)
			require.NoError(t, err)
			return locker
		}
		ctx := context.Background()
		conn1, err := db.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, conn1.Close())
		})
		conn2, err := db.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, conn2.Close())
		})
		locker1, locker2 := newLocker(), newLocker()
		// Acquire on the first session; the second session must fail to acquire and time out.
		require.NoError(t, locker1.SessionLock(ctx, conn1))
		// The session lock must survive a transaction commit.
		tx, err := conn1.BeginTx(ctx, nil)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
		// Reusing the same waiter must provide the full retry budget on every call.
		for call := 1; call <= 2; call++ {
			start := time.Now()
			err = locker2.SessionLock(ctx, conn2)
			require.ErrorContains(t, err, "failed to acquire lock")
			require.GreaterOrEqual(t, time.Since(start), 2*time.Second, "lock call %d", call)
		}
		// Release on the first session; now the second session must acquire promptly.
		require.NoError(t, locker1.SessionUnlock(ctx, conn1))
		require.NoError(t, locker2.SessionLock(ctx, conn2))
		require.NoError(t, locker2.SessionUnlock(ctx, conn2))
	})
	t.Run("unlock_retries_on_every_call", func(t *testing.T) {
		locker, err := lock.NewOracleSessionLocker(
			lock.WithLockID(111111111),
			lock.WithUnlockTimeout(1, 1),
		)
		require.NoError(t, err)
		ctx := t.Context()
		conn, err := db.Conn(ctx)
		require.NoError(t, err)
		require.NoError(t, conn.Close())
		// A closed connection returns a retryable execution error on every release attempt.
		for call := 1; call <= 2; call++ {
			start := time.Now()
			err := locker.SessionUnlock(ctx, conn)
			require.ErrorIs(t, err, sql.ErrConnDone)
			require.GreaterOrEqual(t, time.Since(start), time.Second, "unlock call %d", call)
		}
	})
	t.Run("released_when_session_ends", func(t *testing.T) {
		locker, err := lock.NewOracleSessionLocker(
			lock.WithLockID(555555555),
			lock.WithLockTimeout(1, 10),  // 10 second timeout
			lock.WithUnlockTimeout(1, 2), // 2 second timeout
		)
		require.NoError(t, err)
		ctx := t.Context()
		// With no idle connections, closing a sql.Conn closes the physical session instead
		// of returning it to the pool. Reserve the waiter while the owner is still connected.
		db.SetMaxIdleConns(0)
		conn1, err := db.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn1.Close() })
		conn2, err := db.Conn(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, conn2.Close())
		})
		require.NoError(t, locker.SessionLock(ctx, conn1))
		// Close the owner without unlocking. The distinct waiter must acquire the lock.
		require.NoError(t, conn1.Close())
		require.Equal(t, 1, db.Stats().OpenConnections)
		acquireCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		require.NoError(t, locker.SessionLock(acquireCtx, conn2))
		require.NoError(t, locker.SessionUnlock(ctx, conn2))
	})
}
