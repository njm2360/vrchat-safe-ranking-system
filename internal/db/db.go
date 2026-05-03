package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/njm2360/vrchat-ranking-system/internal/clock"
)

//go:embed schema.sql
var schemaSQL string

type DB struct {
	*sql.DB
	clock clock.Clock
}

type Option func(*DB)

// WithClock injects a Clock so tests can control time-of-creation timestamps.
// Defaults to clock.System if not supplied.
func WithClock(c clock.Clock) Option {
	return func(d *DB) { d.clock = c }
}

// Open opens (or creates) the SQLite database at path with WAL mode and
// applies the schema.
func Open(path string, opts ...Option) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." && path != ":memory:" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	dsn := buildDSN(path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if _, err := sqlDB.ExecContext(context.Background(), schemaSQL); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	d := &DB{DB: sqlDB, clock: clock.System{}}
	for _, opt := range opts {
		opt(d)
	}
	return d, nil
}

// OpenInMemory opens an isolated SQLite memory database for tests.
func OpenInMemory(opts ...Option) (*DB, error) {
	// Each :memory: connection is isolated; modernc.org/sqlite needs the
	// shared-cache + named URI form for multi-statement reuse.
	return Open("file::memory:?cache=shared&_pragma=foreign_keys(on)", opts...)
}

func buildDSN(path string) string {
	if u, err := url.Parse(path); err == nil && u.Scheme == "file" {
		// Already a URI; assume caller knows what they want.
		return path
	}
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(on)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return path + "?" + q.Encode()
}

// nowUnix returns the current epoch second from the injected clock.
func (db *DB) nowUnix() int64 { return db.clock.Now().Unix() }
