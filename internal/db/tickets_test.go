package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func TestInsertAndConsumeTicket(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()

	if err := d.InsertTicket(ctx, "uuid-1", "alice", 5*time.Minute); err != nil {
		t.Fatalf("InsertTicket: %v", err)
	}
	got, err := d.ConsumeTicket(ctx, "uuid-1")
	if err != nil {
		t.Fatalf("ConsumeTicket: %v", err)
	}
	if got.DisplayName != "alice" || got.ConsumedAt == nil {
		t.Errorf("unexpected ticket: %+v", got)
	}
}

func TestConsumeTicketNotFound(t *testing.T) {
	d := newTestDB(t, nil)
	if _, err := d.ConsumeTicket(context.Background(), "missing"); !errors.Is(err, db.ErrTicketNotFound) {
		t.Errorf("err = %v, want ErrTicketNotFound", err)
	}
}

func TestConsumeTicketExpired(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()

	if err := d.InsertTicket(ctx, "u", "alice", time.Minute); err != nil {
		t.Fatal(err)
	}
	fc.Advance(2 * time.Minute) // past TTL
	if _, err := d.ConsumeTicket(ctx, "u"); !errors.Is(err, db.ErrTicketExpired) {
		t.Errorf("err = %v, want ErrTicketExpired", err)
	}
}

func TestConsumeTicketAlreadyUsed(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	if err := d.InsertTicket(ctx, "u", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConsumeTicket(ctx, "u"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConsumeTicket(ctx, "u"); !errors.Is(err, db.ErrTicketUsed) {
		t.Errorf("err = %v, want ErrTicketUsed", err)
	}
}

func TestChallengeRateLimit(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()
	window := time.Minute

	// First call: allowed (no row yet)
	_, allowed, err := d.CheckChallengeRate(ctx, "alice", window)
	if err != nil || !allowed {
		t.Fatalf("first check: allowed=%v err=%v", allowed, err)
	}
	if err := d.UpsertChallengeRate(ctx, "alice"); err != nil {
		t.Fatal(err)
	}

	// Within window: rate-limited
	fc.Advance(30 * time.Second)
	_, allowed, _ = d.CheckChallengeRate(ctx, "alice", window)
	if allowed {
		t.Error("expected rate-limited within window")
	}

	// Past window: allowed again
	fc.Advance(time.Minute)
	_, allowed, _ = d.CheckChallengeRate(ctx, "alice", window)
	if !allowed {
		t.Error("expected allowed past window")
	}

	// Other user: independent
	_, allowed, _ = d.CheckChallengeRate(ctx, "bob", window)
	if !allowed {
		t.Error("expected bob to be allowed")
	}
}

func TestDeleteExpiredTickets(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()

	_ = d.InsertTicket(ctx, "old", "alice", time.Minute)
	fc.Advance(2 * time.Minute)
	_ = d.InsertTicket(ctx, "new", "alice", time.Hour)

	n, err := d.DeleteExpiredTickets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("deleted %d, want 1", n)
	}
}
