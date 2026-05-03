package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

// seedIssuedToken inserts a row directly so latest_saves.jti FK resolves.
func seedIssuedToken(t *testing.T, d *db.DB, jti, discordID, displayName string) {
	t.Helper()
	if _, err := d.ExecContext(context.Background(),
		`INSERT INTO issued_tokens (jti, discord_id, display_name, jwt, issued_at) VALUES (?,?,?,?,0)`,
		jti, discordID, displayName, "jwt-blob"); err != nil {
		t.Fatalf("seed token: %v", err)
	}
}

func TestSaveAppendsHistoryAndUpdatesLatest(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	seedIssuedToken(t, d, "jti-1", "discord-1", "alice")

	id1, err := d.Save(ctx, "alice", 100, "jti-1")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := d.Save(ctx, "alice", 200, "jti-1")
	if err != nil {
		t.Fatal(err)
	}
	if id2 <= id1 {
		t.Errorf("history ids not monotonic: %d, %d", id1, id2)
	}

	got, err := d.GetLatestSave(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.Score != 200 {
		t.Errorf("Score = %d, want 200", got.Score)
	}

	hist, err := d.GetSaveHistory(ctx, "alice", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 || hist[0].Score != 200 || hist[1].Score != 100 {
		t.Errorf("history = %+v, want [200, 100] DESC", hist)
	}
}

func TestSaveWithoutJWTExcludedFromRanking(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	seedIssuedToken(t, d, "jti-1", "discord-1", "alice")

	if _, err := d.Save(ctx, "alice", 100, "jti-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Save(ctx, "anon", 9999, ""); err != nil {
		t.Fatal(err)
	}

	rows, err := d.Ranking(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].DisplayName != "alice" {
		t.Errorf("ranking = %+v, want only [alice]", rows)
	}
}

func TestRankingFiltersBlacklistedJTI(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	seedIssuedToken(t, d, "jti-good", "discord-1", "alice")
	seedIssuedToken(t, d, "jti-bad", "discord-2", "bob")

	_, _ = d.Save(ctx, "alice", 100, "jti-good")
	_, _ = d.Save(ctx, "bob", 999, "jti-bad")

	if err := d.BlacklistJTI(ctx, "jti-bad", "test"); err != nil {
		t.Fatal(err)
	}

	rows, _ := d.Ranking(ctx, 10)
	if len(rows) != 1 || rows[0].DisplayName != "alice" {
		t.Errorf("ranking = %+v, want only [alice]", rows)
	}
}

func TestRankingFiltersBannedDiscordID(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	seedIssuedToken(t, d, "jti-a", "good-id", "alice")
	seedIssuedToken(t, d, "jti-b", "banned-id", "bob")

	_, _ = d.Save(ctx, "alice", 100, "jti-a")
	_, _ = d.Save(ctx, "bob", 999, "jti-b")

	if err := d.Ban(ctx, "banned-id", "test"); err != nil {
		t.Fatal(err)
	}

	rows, _ := d.Ranking(ctx, 10)
	if len(rows) != 1 || rows[0].DisplayName != "alice" {
		t.Errorf("ranking = %+v, want only [alice]", rows)
	}
}

func TestRankingOrdering(t *testing.T) {
	fc := clock.NewFake(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	d := newTestDB(t, fc)
	ctx := context.Background()

	seedIssuedToken(t, d, "j1", "d1", "alice")
	seedIssuedToken(t, d, "j2", "d2", "bob")
	seedIssuedToken(t, d, "j3", "d3", "charlie")

	_, _ = d.Save(ctx, "alice", 500, "j1")
	fc.Advance(time.Second)
	_, _ = d.Save(ctx, "bob", 1000, "j2")
	fc.Advance(time.Second)
	_, _ = d.Save(ctx, "charlie", 1000, "j3") // tie with bob, but later

	rows, _ := d.Ranking(ctx, 10)
	if len(rows) != 3 {
		t.Fatalf("len = %d, want 3", len(rows))
	}
	if rows[0].DisplayName != "bob" {
		t.Errorf("rank[0] = %s, want bob (tie-break by earlier updated_at)", rows[0].DisplayName)
	}
	if rows[1].DisplayName != "charlie" {
		t.Errorf("rank[1] = %s, want charlie", rows[1].DisplayName)
	}
	if rows[2].DisplayName != "alice" {
		t.Errorf("rank[2] = %s, want alice", rows[2].DisplayName)
	}
}

// Documents the design choice: a save with a blacklisted JTI is still
// recorded (history + latest), it is only excluded from /ranking. This lets
// admins blacklist a token without bricking the user's local progress.
func TestSaveWithBlacklistedJTIIsStillRecordedButHidden(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	seedIssuedToken(t, d, "jti-revoked", "discord-1", "alice")
	if err := d.BlacklistJTI(ctx, "jti-revoked", "test"); err != nil {
		t.Fatal(err)
	}

	if _, err := d.Save(ctx, "alice", 100, "jti-revoked"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := d.GetLatestSave(ctx, "alice")
	if err != nil {
		t.Fatalf("GetLatestSave: %v", err)
	}
	if got.Score != 100 {
		t.Errorf("save was not recorded: %+v", got)
	}

	rows, _ := d.Ranking(ctx, 10)
	if len(rows) != 0 {
		t.Errorf("blacklisted-jti save should not appear in ranking, got %+v", rows)
	}
}

func TestRankingLimitClamp(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	// 0 → defaults to 100, > 1000 → defaults to 100
	for _, lim := range []int{-1, 0, 99999} {
		if _, err := d.Ranking(ctx, lim); err != nil {
			t.Errorf("Ranking(%d) returned error: %v", lim, err)
		}
	}
}

func TestGetSaveHistoryLimitClamp(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	for _, lim := range []int{-1, 0, 99999} {
		if _, err := d.GetSaveHistory(ctx, "anyone", lim); err != nil {
			t.Errorf("GetSaveHistory(%d) returned error: %v", lim, err)
		}
	}
}

func TestGetLatestSaveNotFound(t *testing.T) {
	d := newTestDB(t, nil)
	if _, err := d.GetLatestSave(context.Background(), "nobody"); !errors.Is(err, db.ErrSaveNotFound) {
		t.Errorf("err = %v, want ErrSaveNotFound", err)
	}
}
