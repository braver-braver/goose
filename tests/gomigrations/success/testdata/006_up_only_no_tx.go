package gomigrations

import (
	"database/sql"

	"github.com/braver-braver/goose/v3"
)

func init() {
	goose.AddMigrationNoTx(up006, nil)
}

func up006(db *sql.DB) error {
	return createTable(db, "delta")
}
