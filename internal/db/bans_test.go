package db_test

import (
	"context"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

func TestBanIsBannedUnban(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if banned, _ := d.IsDiscordIDBanned(ctx, "119548486276710402"); banned {
		t.Error("not banned initially")
	}
	if err := d.BanDiscordID(ctx, "119548486276710402","test"); err != nil {
		t.Fatal(err)
	}
	if banned, _ := d.IsDiscordIDBanned(ctx, "119548486276710402"); !banned {
		t.Error("expected banned after BanDiscordID()")
	}

	// Re-ban should not error (UPSERT on reason)
	if err := d.BanDiscordID(ctx, "119548486276710402","updated reason"); err != nil {
		t.Errorf("re-ban failed: %v", err)
	}

	if err := d.UnbanDiscordID(ctx, "119548486276710402"); err != nil {
		t.Fatal(err)
	}
	if banned, _ := d.IsDiscordIDBanned(ctx, "119548486276710402"); banned {
		t.Error("expected not banned after UnbanDiscordID()")
	}

	// UnbanDiscordID of non-existent is a no-op (no error)
	if err := d.UnbanDiscordID(ctx, "119548486276710999"); err != nil {
		t.Errorf("unban of non-existent failed: %v", err)
	}
}

func TestBanDisplayName_IsBannedUnban(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if banned, _ := d.IsDisplayNameBanned(ctx, "alice"); banned {
		t.Error("not banned initially")
	}
	if err := d.BanDisplayName(ctx, "alice", "cheating"); err != nil {
		t.Fatal(err)
	}
	if banned, _ := d.IsDisplayNameBanned(ctx, "alice"); !banned {
		t.Error("expected banned after BanDisplayName()")
	}

	// Re-ban should update reason without error.
	if err := d.BanDisplayName(ctx, "alice", "updated reason"); err != nil {
		t.Errorf("re-ban failed: %v", err)
	}

	if err := d.UnbanDisplayName(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	if banned, _ := d.IsDisplayNameBanned(ctx, "alice"); banned {
		t.Error("expected not banned after UnbanDisplayName()")
	}

	// UnbanDiscordID of non-existent is a no-op.
	if err := d.UnbanDisplayName(ctx, "ghost"); err != nil {
		t.Errorf("unban of non-existent failed: %v", err)
	}
}

func TestBanDisplayName_HidesFromRanking(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.UpsertUserAndIssue(ctx, "119548486276710400", "alice", "j1", "jwt1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Save(ctx, "alice", &savedata.Data{Score: 500}, "j1"); err != nil {
		t.Fatal(err)
	}

	rows, _ := d.Ranking(ctx, 10)
	if len(rows) != 1 {
		t.Fatalf("expected 1 ranking row before ban, got %d", len(rows))
	}

	if err := d.BanDisplayName(ctx, "alice", "cheat"); err != nil {
		t.Fatal(err)
	}
	rows, _ = d.Ranking(ctx, 10)
	if len(rows) != 0 {
		t.Errorf("expected empty ranking after display name ban, got %+v", rows)
	}

	if err := d.UnbanDisplayName(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	rows, _ = d.Ranking(ctx, 10)
	if len(rows) != 1 || rows[0].DisplayName != "alice" {
		t.Errorf("expected alice back in ranking after unban, got %+v", rows)
	}
}
