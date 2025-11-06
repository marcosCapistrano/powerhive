package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Store wraps a SQLite connection and exposes helpers to manage PowerHive
// domain entities.
type Store struct {
	db *sql.DB
}

// New creates a Store and enables SQLite foreign keys on the supplied
// connection. Call Init on the returned store to install the schema.
func New(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Enable WAL mode for concurrent reads with writes
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// Set busy timeout to retry instead of immediate failure
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	return &Store{db: db}, nil
}

// Init installs the database schema. It is safe to call multiple times; every
// statement uses IF NOT EXISTS guards.
func (s *Store) Init(ctx context.Context) error {
	for i, stmt := range schemaStatements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			if isIgnorableSchemaError(err) {
				continue
			}
			return fmt.Errorf("apply schema statement %d: %w", i+1, err)
		}
	}
	return nil
}

// DB exposes the underlying database handle for read-only situations. Mutating
// callers should prefer Store helpers to keep the schema invariants intact.
func (s *Store) DB() *sql.DB {
	return s.db
}

func isIgnorableSchemaError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "duplicate column name") {
		return true
	}
	return false
}
