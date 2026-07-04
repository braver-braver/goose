package testdb

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	go_ora "github.com/sijms/go-ora/v2"
)

const (
	// https://hub.docker.com/r/gvenzl/oracle-free
	//
	// There is no freely redistributable 19c image; Oracle Database Free (23ai) is used
	// instead. The dialect queries and test migrations only rely on 12c+ features, so they
	// are exercised identically.
	ORACLE_IMAGE   = "gvenzl/oracle-free"
	ORACLE_VERSION = "23-slim-faststart"

	ORACLE_SERVICE      = "FREEPDB1"
	ORACLE_USER         = "gooseuser"
	ORACLE_PASSWORD     = "password1"
	ORACLE_SYS_PASSWORD = "password1"
)

func newOracle(opts ...OptionsFunc) (*sql.DB, func(), error) {
	option := &options{}
	for _, f := range opts {
		f(option)
	}
	// Uses a sensible default on windows (tcp/http) and linux/osx (socket).
	pool, err := dockertest.NewPool("")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to docker: %v", err)
	}
	runOptions := &dockertest.RunOptions{
		Repository: ORACLE_IMAGE,
		Tag:        ORACLE_VERSION,
		Env: []string{
			"ORACLE_PASSWORD=" + ORACLE_SYS_PASSWORD,
			"APP_USER=" + ORACLE_USER,
			"APP_USER_PASSWORD=" + ORACLE_PASSWORD,
		},
		Labels:       map[string]string{"goose_test": "1"},
		PortBindings: make(map[docker.Port][]docker.PortBinding),
		// The image does not declare EXPOSE, so the port must be exposed explicitly for
		// dockertest's PublishAllPorts to map it to a host port.
		ExposedPorts: []string{"1521/tcp"},
	}
	if option.bindPort > 0 {
		runOptions.PortBindings[docker.Port("1521/tcp")] = []docker.PortBinding{
			{HostPort: strconv.Itoa(option.bindPort)},
		}
	}
	container, err := pool.RunWithOptions(
		runOptions,
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
	port, err := strconv.Atoi(container.GetPort("1521/tcp"))
	if err != nil {
		return nil, cleanup, fmt.Errorf("failed to parse container port: %v", err)
	}
	dsn := go_ora.BuildUrl("localhost", port, ORACLE_SERVICE, ORACLE_USER, ORACLE_PASSWORD, nil)
	var db *sql.DB
	// Exponential backoff-retry, because the database in the container takes a while to
	// initialize on first boot.
	pool.MaxWait = time.Minute * 5
	if err := pool.Retry(
		func() error {
			var err error
			db, err = sql.Open("oracle", dsn)
			if err != nil {
				return err
			}
			return db.Ping()
		},
	); err != nil {
		return nil, cleanup, fmt.Errorf("could not connect to docker database: %v", err)
	}
	return db, cleanup, nil
}
