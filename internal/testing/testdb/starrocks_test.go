package testdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWaitForStarrocksReusesPool(t *testing.T) {
	c := &starrocksConnector{readyAfter: 2}
	db := sql.OpenDB(c)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	err := waitForStarrocks(db, func(check func() error) error {
		require.ErrorContains(t, check(), "no alive backend")
		return check()
	})
	require.NoError(t, err)
	require.Equal(t, 2, c.checks)
	require.Equal(t, 1, c.opens)
	require.Zero(t, c.closes)
}

func TestWaitForStarrocksClosesPoolOnTimeout(t *testing.T) {
	c := &starrocksConnector{readyAfter: 2}
	db := sql.OpenDB(c)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	timeout := errors.New("readiness deadline")
	err := waitForStarrocks(db, func(check func() error) error {
		require.ErrorContains(t, check(), "no alive backend")
		return timeout
	})
	require.ErrorIs(t, err, timeout)
	require.Equal(t, 1, c.closes)
	require.ErrorContains(t, db.Ping(), "database is closed")
}

type starrocksConnector struct {
	readyAfter, checks, opens, closes int
}

func (c *starrocksConnector) Connect(context.Context) (driver.Conn, error) {
	c.opens++
	return &starrocksConn{c}, nil
}
func (c *starrocksConnector) Driver() driver.Driver { return nil }

type starrocksConn struct{ connector *starrocksConnector }

func (*starrocksConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare")
}
func (*starrocksConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected Begin") }
func (c *starrocksConn) Close() error {
	c.connector.closes++
	return nil
}
func (*starrocksConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}
func (c *starrocksConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	s := c.connector
	s.checks++
	return &starrocksBackendRow{alive: s.checks >= s.readyAfter}, nil
}

type starrocksBackendRow struct{ alive, read bool }

func (*starrocksBackendRow) Columns() []string { return []string{"Alive"} }
func (*starrocksBackendRow) Close() error      { return nil }
func (r *starrocksBackendRow) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	dest[0] = r.alive
	return nil
}
