package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func TestInsertAndConsumeAuthSession(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.InsertAuthSession(ctx, "tok-1", "119548486276710400", "alice.dev", "alice", time.Hour); err != nil {
		t.Fatalf("InsertAuthSession: %v", err)
	}
	got, err := d.ConsumeAuthSession(ctx, "tok-1")
	if err != nil {
		t.Fatalf("ConsumeAuthSession: %v", err)
	}
	if got.DiscordID != "119548486276710400" || got.DiscordUsername != "alice.dev" || got.ProposedName != "alice" {
		t.Errorf("unexpected session: %+v", got)
	}
}

func TestConsumeAuthSessionNotFound(t *testing.T) {
	d := newTestDB(t, nil)
	if _, err := d.ConsumeAuthSession(context.Background(), "missing"); !errors.Is(err, db.ErrAuthSessionNotFound) {
		t.Errorf("err = %v, want ErrAuthSessionNotFound", err)
	}
}

func TestConsumeAuthSessionExpired(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()
	if err := d.InsertAuthSession(ctx, "t", "119548486276710402", "u", "alice", time.Minute); err != nil {
		t.Fatal(err)
	}
	fc.Advance(2 * time.Minute)
	if _, err := d.ConsumeAuthSession(ctx, "t"); !errors.Is(err, db.ErrAuthSessionExpired) {
		t.Errorf("err = %v, want ErrAuthSessionExpired", err)
	}
	if _, err := d.ConsumeAuthSession(ctx, "t"); !errors.Is(err, db.ErrAuthSessionNotFound) {
		t.Errorf("second err = %v, want ErrAuthSessionNotFound", err)
	}
}

func TestConsumeAuthSessionSingleUse(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	if err := d.InsertAuthSession(ctx, "once", "119548486276710402", "u", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConsumeAuthSession(ctx, "once"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ConsumeAuthSession(ctx, "once"); !errors.Is(err, db.ErrAuthSessionNotFound) {
		t.Errorf("err = %v, want ErrAuthSessionNotFound on reuse", err)
	}
}

func TestGetAuthSession_DoesNotConsume(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	if err := d.InsertAuthSession(ctx, "peek", "119548486276710400", "alice.dev", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		got, err := d.GetAuthSession(ctx, "peek")
		if err != nil {
			t.Fatalf("GetAuthSession #%d: %v", i, err)
		}
		if got.DiscordID != "119548486276710400" || got.ProposedName != "alice" {
			t.Errorf("session = %+v", got)
		}
	}
	if _, err := d.ConsumeAuthSession(ctx, "peek"); err != nil {
		t.Errorf("ConsumeAuthSession after peek: %v", err)
	}
	if _, err := d.GetAuthSession(ctx, "peek"); !errors.Is(err, db.ErrAuthSessionNotFound) {
		t.Errorf("err = %v, want ErrAuthSessionNotFound after consume", err)
	}
}

func TestGetAuthSessionExpired(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()
	if err := d.InsertAuthSession(ctx, "t", "119548486276710402", "u", "alice", time.Minute); err != nil {
		t.Fatal(err)
	}
	fc.Advance(2 * time.Minute)
	if _, err := d.GetAuthSession(ctx, "t"); !errors.Is(err, db.ErrAuthSessionExpired) {
		t.Errorf("err = %v, want ErrAuthSessionExpired", err)
	}
}

func TestDeleteExpiredAuthSessions(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()

	_ = d.InsertAuthSession(ctx, "old", "119548486276710402", "u", "alice", time.Minute)
	fc.Advance(2 * time.Minute)
	_ = d.InsertAuthSession(ctx, "new", "119548486276710402", "u", "bob", time.Hour)

	n, err := d.DeleteExpiredAuthSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("deleted %d, want 1", n)
	}
	if _, err := d.ConsumeAuthSession(ctx, "new"); err != nil {
		t.Errorf("expected 'new' to survive: %v", err)
	}
}
