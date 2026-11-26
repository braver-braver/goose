package testdb

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

const (
	// https://hub.docker.com/r/starrocks/allin1-ubuntu
	STARROCKS_IMAGE   = "starrocks/allin1-ubuntu"
	STARROCKS_VERSION = "3.5.11"

	STARROCKS_USER    = "root"
	STARROCKS_INIT_DB = "migrations"
)

func newStarrocks(opts ...OptionsFunc) (*sql.DB, func(), error) {
	option := &options{}
	for _, f := range opts {
		f(option)
	}
	// Uses a sensible default on windows (tcp/http) and linux/osx (socket).
	pool, err := dockertest.NewPool("")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to docker: %v", err)
	}

	options := &dockertest.RunOptions{
		Repository:   STARROCKS_IMAGE,
		Tag:          STARROCKS_VERSION,
		Labels:       map[string]string{"goose_test": "1"},
		PortBindings: make(map[docker.Port][]docker.PortBinding),
		ExposedPorts: []string{"9030/tcp"},
	}
	if option.bindPort > 0 {
		options.PortBindings[docker.Port("9030/tcp")] = []docker.PortBinding{
			{HostPort: strconv.Itoa(option.bindPort)},
		}
	}
	container, err := pool.RunWithOptions(
		options,
		func(config *docker.HostConfig) {
			// Set AutoRemove to true so that stopped container goes away by itself.
			config.AutoRemove = true
			config.RestartPolicy = docker.RestartPolicy{Name: "no"}
		},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create docker container: %v", err)
	}
	cleanup := func() {
		if option.debug {
			// User must manually delete the Docker container.
			return
		}
		if err := pool.Purge(container); err != nil {
			log.Printf("failed to purge resource: %v", err)
		}
	}
	dsn := fmt.Sprintf("%s:%s@(%s:%s)/%s?parseTime=true&interpolateParams=true",
		STARROCKS_USER,
		"",
		"localhost",
		container.GetPort("9030/tcp"), // Fetch port dynamically assigned to container,
		"",
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("open StarRocks database: %w", err)
	}

	// The SQL frontend can start accepting connections before a backend is ready.
	// Reuse the same connection pool while waiting.
	pool.MaxWait = 2 * time.Minute
	if err := waitForStarrocks(db, pool.Retry); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("could not connect to docker database: %w", err)
	}

	return db, func() {
		if err := db.Close(); err != nil {
			log.Printf("failed to close StarRocks database: %v", err)
		}
		cleanup()
	}, nil
}

func waitForStarrocks(db *sql.DB, retry func(func() error) error) error {
	err := retry(func() error {
		if _, err := db.Exec("CREATE DATABASE IF NOT EXISTS " + STARROCKS_INIT_DB); err != nil {
			return fmt.Errorf("could not create initial database: %w", err)
		}
		if _, err := db.Exec("USE " + STARROCKS_INIT_DB); err != nil {
			return fmt.Errorf("could not set default initial database: %w", err)
		}
		if err := db.Ping(); err != nil {
			return err
		}
		return checkStarrocksBackendReady(db)
	})
	if err != nil {
		return errors.Join(err, db.Close())
	}
	return nil
}

// checkStarrocksBackendReady waits for an alive backend, not merely the SQL frontend.
// StarRocks may accept connections and CREATE DATABASE while CREATE TABLE still fails
// with "Cluster has no available capacity".
func checkStarrocksBackendReady(db *sql.DB) error {
	rows, err := db.Query("SHOW BACKENDS")
	if err != nil {
		return fmt.Errorf("query StarRocks backends: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("read StarRocks backend columns: %w", err)
	}
	aliveColumn := -1
	for i, column := range columns {
		if strings.EqualFold(column, "Alive") {
			aliveColumn = i
			break
		}
	}
	if aliveColumn < 0 {
		return errors.New("StarRocks SHOW BACKENDS response lacks Alive column")
	}
	values := make([]sql.RawBytes, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(targets...); err != nil {
			return fmt.Errorf("scan StarRocks backend: %w", err)
		}
		alive := string(values[aliveColumn])
		if strings.EqualFold(alive, "true") || alive == "1" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate StarRocks backends: %w", err)
	}
	return errors.New("StarRocks has no alive backend yet")
}
