package db_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

// newTestDB returns a fresh DB backed by a temp file (modernc.org/sqlite's
// file::memory: shared cache survives one t.Run only, and we want strict
// isolation between subtests). Caller need not close — t.Cleanup handles it.
func newTestDB(t *testing.T, fc clock.Clock) *db.DB {
	t.Helper()
	if fc == nil {
		fc = clock.NewFake(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	}
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path, db.WithClock(fc))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}
