package integration

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/braver-braver/goose/v3"
	"github.com/braver-braver/goose/v3/database"
	"github.com/braver-braver/goose/v3/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func TestPostgres(t *testing.T) {
	t.Parallel()

	db, cleanup, err := testdb.NewPostgres()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, db.Ping())

	testDatabase(t, database.DialectPostgres, db, "testdata/migrations/postgres")
}

func TestSpanner(t *testing.T) {
	t.Parallel()

	db, cleanup, err := testdb.NewSpanner()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, db.Ping())

	testDatabase(t, database.DialectSpanner, db, "testdata/migrations/spanner", goose.WithIsolateDDL(true))
}

func TestClickhouse(t *testing.T) {
	t.Parallel()

	db, cleanup, err := testdb.NewClickHouse()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, db.Ping())

	testDatabase(t, database.DialectClickHouse, db, "testdata/migrations/clickhouse")

	type result struct {
		customerID     string    `db:"customer_id"`
		timestamp      time.Time `db:"time_stamp"`
		clickEventType string    `db:"click_event_type"`
		countryCode    string    `db:"country_code"`
		sourceID       int64     `db:"source_id"`
	}
	rows, err := db.Query(`SELECT * FROM clickstream ORDER BY customer_id`)
	require.NoError(t, err)
	var results []result
	for rows.Next() {
		var r result
		err = rows.Scan(&r.customerID, &r.timestamp, &r.clickEventType, &r.countryCode, &r.sourceID)
		require.NoError(t, err)
		results = append(results, r)
	}
	require.Equal(t, len(results), 3)
	require.NoError(t, rows.Close())
	require.NoError(t, rows.Err())

	parseTime := func(t *testing.T, s string) time.Time {
		t.Helper()
		tm, err := time.Parse("2006-01-02", s)
		require.NoError(t, err)
		return tm
	}
	want := []result{
		{"customer1", parseTime(t, "2021-10-02"), "add_to_cart", "US", 568239},
		{"customer2", parseTime(t, "2021-10-30"), "remove_from_cart", "", 0},
		{"customer3", parseTime(t, "2021-11-07"), "checkout", "", 307493},
	}
	for i, result := range results {
		require.Equal(t, result.customerID, want[i].customerID)
		require.Equal(t, result.timestamp, want[i].timestamp)
		require.Equal(t, result.clickEventType, want[i].clickEventType)
		if result.countryCode != "" && want[i].countryCode != "" {
			require.Equal(t, result.countryCode, want[i].countryCode)
		}
		require.Equal(t, result.sourceID, want[i].sourceID)
	}
}

func TestClickhouseRemote(t *testing.T) {
	t.Parallel()

	db, cleanup, err := testdb.NewClickHouse()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, db.Ping())
	testDatabase(t, database.DialectClickHouse, db, "testdata/migrations/clickhouse-remote")

	// assert that the taxi_zone_dictionary table has been created and populated
	var count int
	err = db.QueryRow(`SELECT count(*) FROM taxi_zone_dictionary`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 265, count)
}

func TestMySQL(t *testing.T) {
	t.Parallel()

	db, cleanup, err := testdb.NewMariaDB()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, db.Ping())

	testDatabase(t, database.DialectMySQL, db, "testdata/migrations/mysql")
}

func TestOracle(t *testing.T) {
	t.Parallel()

	db, cleanup, err := testdb.NewOracle()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, db.Ping())

	testDatabase(t, database.DialectOracle, db, "testdata/migrations/oracle")

	// Verify the values round-trip through Oracle NUMBER/VARCHAR2 columns, and exercise the
	// PL/SQL procedure created by the last migration.
	_, err = db.Exec(`BEGIN insert_repository('acme/widgets', 'acme', 'organization'); END;`)
	require.NoError(t, err)
	var count int64
	err = db.QueryRow(`SELECT COUNT(*) FROM repos WHERE repo_full_name = :1`, "acme/widgets").Scan(&count)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	t.Run("failed_ddl_is_not_rolled_back", func(t *testing.T) {
		ctx := t.Context()
		fsys := fstest.MapFS{
			"00001_failure.sql": {Data: []byte(`-- +goose Up
CREATE TABLE goose_test_partial_ddl (id NUMBER PRIMARY KEY);
CREATE TABLE goose_test_partial_ddl (id NUMBER PRIMARY KEY);
-- +goose Down
DROP TABLE goose_test_partial_ddl;`)},
		}
		p, err := goose.NewProvider(database.DialectOracle, db, fsys,
			goose.WithTableName("goose_test_ddl_version"))
		require.NoError(t, err)
		_, err = p.Up(ctx)
		require.ErrorContains(t, err, "ORA-00955")
		version, err := p.GetDBVersion(ctx)
		require.NoError(t, err)
		require.Zero(t, version, "failed migration must not be recorded")
		var count int
		err = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM user_tables WHERE table_name = :1`,
			"GOOSE_TEST_PARTIAL_DDL").Scan(&count)
		require.NoError(t, err)
		require.Equal(t, 1, count, "the first DDL statement was implicitly committed")
	})
	t.Run("failed_dml_is_rolled_back", func(t *testing.T) {
		ctx := t.Context()
		_, err := db.ExecContext(ctx, `CREATE TABLE goose_test_dml_rollback (id NUMBER PRIMARY KEY)`)
		require.NoError(t, err)
		fsys := fstest.MapFS{
			"00001_failure.sql": {Data: []byte(`-- +goose Up
INSERT INTO goose_test_dml_rollback (id) VALUES (1);
INSERT INTO goose_test_dml_rollback (id) VALUES (1);
-- +goose Down
DELETE FROM goose_test_dml_rollback;`)},
		}
		p, err := goose.NewProvider(database.DialectOracle, db, fsys,
			goose.WithTableName("goose_test_dml_version"))
		require.NoError(t, err)
		_, err = p.Up(ctx)
		require.ErrorContains(t, err, "ORA-00001")
		version, err := p.GetDBVersion(ctx)
		require.NoError(t, err)
		require.Zero(t, version, "failed migration must not be recorded")
		var count int
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM goose_test_dml_rollback`).Scan(&count)
		require.NoError(t, err)
		require.Zero(t, count, "the first DML statement must be rolled back")
	})
}

func TestTurso(t *testing.T) {
	t.Parallel()

	db, cleanup, err := testdb.NewTurso()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, db.Ping())

	testDatabase(t, database.DialectTurso, db, "testdata/migrations/turso")
}

func TestYDB(t *testing.T) {
	t.Parallel()

	db, cleanup, err := testdb.NewYdb()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, db.Ping())

	testDatabase(t, database.DialectYdB, db, "testdata/migrations/ydb")
}

func TestStarrocks(t *testing.T) {
	t.Parallel()

	// t.Skip("Starrocks is flaky on CI, see https://github.com/pressly/goose/issues/881")

	db, cleanup, err := testdb.NewStarrocks()
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.NoError(t, db.Ping())

	testDatabase(t, database.DialectStarrocks, db, "testdata/migrations/starrocks", goose.WithIsolateDDL(true))
}
