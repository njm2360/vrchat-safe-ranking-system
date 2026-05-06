package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func TestInsertAndConsumeOAuthState(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.InsertOAuthState(ctx, "state-1", "alice", 5*time.Minute); err != nil {
		t.Fatalf("InsertOAuthState: %v", err)
	}
	got, err := d.ConsumeOAuthState(ctx, "state-1")
	if err != nil {
		t.Fatalf("ConsumeOAuthState: %v", err)
	}
	if got.ProposedName != "alice" {
		t.Errorf("unexpected state: %+v", got)
	}
}

func TestConsumeOAuthStateNotFound(t *testing.T) {
	d := newTestDB(t, nil)
	if _, err := d.ConsumeOAuthState(context.Background(), "missing"); !errors.Is(err, db.ErrOAuthStateNotFound) {
		t.Errorf("err = %v, want ErrOAuthStateNotFound", err)
	}
}

func TestConsumeOAuthStateExpired(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()

	if err := d.InsertOAuthState(ctx, "s", "alice", time.Minute); err != nil {
		t.Fatal(err)
	}
	fc.Advance(2 * time.Minute)
	if _, err := d.ConsumeOAuthState(ctx, "s"); !errors.Is(err, db.ErrOAuthStateExpired) {
		t.Errorf("err = %v, want ErrOAuthStateExpired", err)
	}
	if _, err := d.ConsumeOAuthState(ctx, "s"); !errors.Is(err, db.ErrOAuthStateNotFound) {
		t.Errorf("second err = %v, want ErrOAuthStateNotFound", err)
	}
}

func TestConsumeOAuthStateSingleUse(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	if err := d.InsertOAuthState(ctx, "once", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConsumeOAuthState(ctx, "once"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConsumeOAuthState(ctx, "once"); !errors.Is(err, db.ErrOAuthStateNotFound) {
		t.Errorf("err = %v, want ErrOAuthStateNotFound on reuse", err)
	}
}

func TestDeleteExpiredOAuthStates(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()

	_ = d.InsertOAuthState(ctx, "old", "a", time.Minute)
	fc.Advance(2 * time.Minute)
	_ = d.InsertOAuthState(ctx, "new", "b", time.Hour)

	n, err := d.DeleteExpiredOAuthStates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("deleted %d, want 1", n)
	}
	if _, err := d.ConsumeOAuthState(ctx, "new"); err != nil {
		t.Errorf("expected 'new' to survive: %v", err)
	}
}
