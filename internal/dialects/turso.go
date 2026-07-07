package dialects

import "github.com/braver-braver/goose/v3/database/dialect"

// NewTurso returns a [dialect.Querier] for Turso dialect.
func NewTurso() dialect.Querier {
	return &turso{}
}

type turso struct {
	sqlite3
}

var _ dialect.Querier = (*turso)(nil)
