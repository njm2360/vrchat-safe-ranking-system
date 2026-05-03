package db_test

import (
	"context"
	"testing"
)

func TestBlacklistJTI(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	seedIssuedToken(t, d, "j1", "d1", "alice")

	if black, _ := d.IsJTIBlacklisted(ctx, "j1"); black {
		t.Error("not blacklisted initially")
	}
	if err := d.BlacklistJTI(ctx, "j1", "test"); err != nil {
		t.Fatal(err)
	}
	if black, _ := d.IsJTIBlacklisted(ctx, "j1"); !black {
		t.Error("expected blacklisted")
	}

	// Re-blacklisting is a no-op (ON CONFLICT DO NOTHING)
	if err := d.BlacklistJTI(ctx, "j1", "different reason"); err != nil {
		t.Errorf("re-blacklist failed: %v", err)
	}

	// Unknown jti returns false, not error
	if black, err := d.IsJTIBlacklisted(ctx, "missing"); err != nil || black {
		t.Errorf("unknown jti: black=%v err=%v", black, err)
	}
}
