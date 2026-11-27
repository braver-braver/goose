package locking_test

import (
	"context"
	"testing"
	"time"

	"github.com/braver-braver/goose/v3/internal/testing/testdb"
	"github.com/braver-braver/goose/v3/lock"
	"github.com/stretchr/testify/require"
)

// A reused locker must get a fresh retry budget on every call, even after
// an earlier attempt exhausted its retries while another session held the lock.
func TestOracleSessionLockerRetriesOnEveryCall(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	db, cleanup, err := testdb.NewOracle()
	require.NoError(t, err)
	t.Cleanup(cleanup)

	const lockID int64 = 456789123
	ctx := context.Background()
	holder, err := lock.NewOracleSessionLocker(lock.WithLockID(lockID))
	require.NoError(t, err)
	holderConn, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, holder.SessionUnlock(ctx, holderConn))
		require.NoError(t, holderConn.Close())
	})
	require.NoError(t, holder.SessionLock(ctx, holderConn))

	waiter, err := lock.NewOracleSessionLocker(
		lock.WithLockID(lockID),
		lock.WithLockTimeout(1, 1),
	)
	require.NoError(t, err)
	waiterConn, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, waiterConn.Close())
	})

	for call := 1; call <= 2; call++ {
		start := time.Now()
		err := waiter.SessionLock(ctx, waiterConn)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to acquire lock")
		require.GreaterOrEqual(t, time.Since(start), 900*time.Millisecond, "call %d", call)
	}
}

func TestPostgresSessionLockerRetriesOnEveryCall(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	db, cleanup, err := testdb.NewPostgres()
	require.NoError(t, err)
	t.Cleanup(cleanup)

	const lockID int64 = 456789123
	ctx := context.Background()
	holder, err := lock.NewPostgresSessionLocker(lock.WithLockID(lockID))
	require.NoError(t, err)
	holderConn, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, holder.SessionUnlock(ctx, holderConn))
		require.NoError(t, holderConn.Close())
	})
	require.NoError(t, holder.SessionLock(ctx, holderConn))

	waiter, err := lock.NewPostgresSessionLocker(
		lock.WithLockID(lockID),
		lock.WithLockTimeout(1, 1),
	)
	require.NoError(t, err)
	waiterConn, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, waiterConn.Close())
	})

	for call := 1; call <= 2; call++ {
		start := time.Now()
		err := waiter.SessionLock(ctx, waiterConn)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to acquire lock")
		require.GreaterOrEqual(t, time.Since(start), 900*time.Millisecond, "call %d", call)
	}
}
