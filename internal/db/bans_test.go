package db_test

import (
	"context"
	"testing"
)

func TestBanIsBannedUnban(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if banned, _ := d.IsBanned(ctx, "d"); banned {
		t.Error("not banned initially")
	}
	if err := d.Ban(ctx, "d", "test"); err != nil {
		t.Fatal(err)
	}
	if banned, _ := d.IsBanned(ctx, "d"); !banned {
		t.Error("expected banned after Ban()")
	}

	// Re-ban should not error (UPSERT on reason)
	if err := d.Ban(ctx, "d", "updated reason"); err != nil {
		t.Errorf("re-ban failed: %v", err)
	}

	if err := d.Unban(ctx, "d"); err != nil {
		t.Fatal(err)
	}
	if banned, _ := d.IsBanned(ctx, "d"); banned {
		t.Error("expected not banned after Unban()")
	}

	// Unban of non-existent is a no-op (no error)
	if err := d.Unban(ctx, "ghost"); err != nil {
		t.Errorf("unban of non-existent failed: %v", err)
	}
}
