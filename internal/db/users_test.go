package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func TestUpsertUserAndIssue_NewUser(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.UpsertUserAndIssue(ctx, "discord-1", "alice", "jti-1", "jwt-blob-1", "init"); err != nil {
		t.Fatalf("UpsertUserAndIssue: %v", err)
	}

	u, err := d.GetUserByDiscordID(ctx, "discord-1")
	if err != nil {
		t.Fatal(err)
	}
	if u.DisplayName != "alice" || u.CurrentJTI != "jti-1" {
		t.Errorf("user = %+v", u)
	}

	black, err := d.IsJTIBlacklisted(ctx, "jti-1")
	if err != nil {
		t.Fatal(err)
	}
	if black {
		t.Error("new jti should not be blacklisted")
	}
}

func TestUpsertUserAndIssue_RenewBlacklistsOldJTI(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.UpsertUserAndIssue(ctx, "discord-1", "alice", "jti-1", "jwt-1", "init"); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertUserAndIssue(ctx, "discord-1", "alice2", "jti-2", "jwt-2", "rename"); err != nil {
		t.Fatal(err)
	}

	u, _ := d.GetUserByDiscordID(ctx, "discord-1")
	if u.DisplayName != "alice2" || u.CurrentJTI != "jti-2" {
		t.Errorf("after renew, user = %+v", u)
	}
	black, _ := d.IsJTIBlacklisted(ctx, "jti-1")
	if !black {
		t.Error("old jti-1 should be blacklisted after renewal")
	}
}

func TestUpsertUserAndIssue_RejectsDisplayNameStolenByOtherDiscordID(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.UpsertUserAndIssue(ctx, "discord-1", "alice", "j1", "jwt1", ""); err != nil {
		t.Fatal(err)
	}
	err := d.UpsertUserAndIssue(ctx, "discord-2", "alice", "j2", "jwt2", "")
	if !errors.Is(err, db.ErrDisplayNameTaken) {
		t.Fatalf("err = %v, want ErrDisplayNameTaken", err)
	}
}

func TestGetCurrentJWT(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.UpsertUserAndIssue(ctx, "discord-1", "alice", "j1", "jwt-blob", ""); err != nil {
		t.Fatal(err)
	}
	gotJWT, gotName, err := d.GetCurrentJWT(ctx, "discord-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotJWT != "jwt-blob" || gotName != "alice" {
		t.Errorf("got (%q, %q), want (jwt-blob, alice)", gotJWT, gotName)
	}
}

func TestGetUserByDisplayName(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	if err := d.UpsertUserAndIssue(ctx, "discord-1", "alice", "j1", "jwt", ""); err != nil {
		t.Fatal(err)
	}
	u, err := d.GetUserByDisplayName(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.DiscordID != "discord-1" {
		t.Errorf("DiscordID = %q", u.DiscordID)
	}
	if _, err := d.GetUserByDisplayName(ctx, "missing"); !errors.Is(err, db.ErrUserNotFound) {
		t.Errorf("missing user err = %v", err)
	}
}

// Re-issuing for the same (discord_id, display_name) without rename:
// new jti replaces current_jti, old jti is blacklisted just like a rename.
func TestUpsertUserAndIssue_ReissueWithoutRename(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	if err := d.UpsertUserAndIssue(ctx, "d", "alice", "j1", "jwt1", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertUserAndIssue(ctx, "d", "alice", "j2", "jwt2", "reissue"); err != nil {
		t.Fatal(err)
	}

	u, _ := d.GetUserByDiscordID(ctx, "d")
	if u.CurrentJTI != "j2" {
		t.Errorf("CurrentJTI = %q, want j2", u.CurrentJTI)
	}
	if black, _ := d.IsJTIBlacklisted(ctx, "j1"); !black {
		t.Error("old jti should be blacklisted on reissue")
	}
}

func TestUnregister_BlacklistsCurrentJTI(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	if err := d.UpsertUserAndIssue(ctx, "d", "alice", "j1", "jwt1", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Unregister(ctx, "d"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if black, _ := d.IsJTIBlacklisted(ctx, "j1"); !black {
		t.Error("current jti should be blacklisted after Unregister")
	}
	// users row preserved so the display_name stays reserved.
	u, err := d.GetUserByDiscordID(ctx, "d")
	if err != nil {
		t.Fatalf("user row should still exist: %v", err)
	}
	if u.DisplayName != "alice" {
		t.Errorf("DisplayName = %q, want alice", u.DisplayName)
	}
}

func TestUnregister_NotRegistered(t *testing.T) {
	d := newTestDB(t, nil)
	if err := d.Unregister(context.Background(), "ghost"); !errors.Is(err, db.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

func TestGetUserByDiscordIDNotFound(t *testing.T) {
	d := newTestDB(t, nil)
	if _, err := d.GetUserByDiscordID(context.Background(), "missing"); !errors.Is(err, db.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}
