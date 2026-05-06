package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

// seedIssuedToken creates a users row (if needed) then inserts an issued_tokens
// row, mirroring the order required by issued_tokens.discord_id → users FK.
func seedIssuedToken(t *testing.T, d *db.DB, jti, discordID, displayName string) {
	t.Helper()
	ctx := context.Background()
	const ts = "2025-01-01T00:00:00Z"
	if _, err := d.ExecContext(ctx,
		`INSERT INTO users (discord_id, display_name, current_jti, created_at, updated_at)
		 VALUES (?,?,NULL,?,?)
		 ON CONFLICT(discord_id) DO UPDATE SET current_jti = NULL`,
		discordID, displayName, ts, ts); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`INSERT INTO issued_tokens (jti, discord_id, display_name, jwt, issued_at) VALUES (?,?,?,?,0)`,
		jti, discordID, displayName, "jwt-blob"); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`UPDATE users SET current_jti = ? WHERE discord_id = ?`, jti, discordID); err != nil {
		t.Fatalf("seed user current_jti: %v", err)
	}
}

func TestSaveAppendsHistoryAndUpdatesLatest(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	seedIssuedToken(t, d, "jti-1", "119548486276710400", "alice")

	id1, err := d.Save(ctx, "alice", &savedata.Data{Score: 100}, "jti-1")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := d.Save(ctx, "alice", &savedata.Data{Score: 200}, "jti-1")
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
	if got.Data.Score != 200 {
		t.Errorf("Score = %d, want 200", got.Data.Score)
	}

}

func TestSaveWithoutJWTExcludedFromRanking(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	seedIssuedToken(t, d, "jti-1", "119548486276710400", "alice")

	if _, err := d.Save(ctx, "alice", &savedata.Data{Score: 100}, "jti-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Save(ctx, "anon", &savedata.Data{Score: 9999}, ""); err != nil {
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
	seedIssuedToken(t, d, "jti-good", "119548486276710400", "alice")
	seedIssuedToken(t, d, "jti-bad", "119548486276710401", "bob")

	_, _ = d.Save(ctx, "alice", &savedata.Data{Score: 100}, "jti-good")
	_, _ = d.Save(ctx, "bob", &savedata.Data{Score: 999}, "jti-bad")

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

	_, _ = d.Save(ctx, "alice", &savedata.Data{Score: 100}, "jti-a")
	_, _ = d.Save(ctx, "bob", &savedata.Data{Score: 999}, "jti-b")

	if err := d.BanDiscordID(ctx, "banned-id", "test"); err != nil {
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

	_, _ = d.Save(ctx, "alice", &savedata.Data{Score: 500}, "j1")
	fc.Advance(time.Second)
	_, _ = d.Save(ctx, "bob", &savedata.Data{Score: 1000}, "j2")
	fc.Advance(time.Second)
	_, _ = d.Save(ctx, "charlie", &savedata.Data{Score: 1000}, "j3") // tie with bob, but later

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

func TestGetLatestSaveNotFound(t *testing.T) {
	d := newTestDB(t, nil)
	if _, err := d.GetLatestSave(context.Background(), "nobody"); !errors.Is(err, db.ErrSaveNotFound) {
		t.Errorf("err = %v, want ErrSaveNotFound", err)
	}
}
