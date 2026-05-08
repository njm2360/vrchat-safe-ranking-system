package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

func TestUpsertUserAndIssue_NewUser(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.UpsertUserAndIssue(ctx, "119548486276710400", "alice", "jti-1", "jwt-blob-1", "init"); err != nil {
		t.Fatalf("UpsertUserAndIssue: %v", err)
	}

	u, err := d.GetUserByDiscordID(ctx, "119548486276710400")
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

	if err := d.UpsertUserAndIssue(ctx, "119548486276710400", "alice", "jti-1", "jwt-1", "init"); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertUserAndIssue(ctx, "119548486276710400", "alice2", "jti-2", "jwt-2", "rename"); err != nil {
		t.Fatal(err)
	}

	u, _ := d.GetUserByDiscordID(ctx, "119548486276710400")
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

	if err := d.UpsertUserAndIssue(ctx, "119548486276710400", "alice", "j1", "jwt1", ""); err != nil {
		t.Fatal(err)
	}
	err := d.UpsertUserAndIssue(ctx, "119548486276710401", "alice", "j2", "jwt2", "")
	if !errors.Is(err, db.ErrDisplayNameTaken) {
		t.Fatalf("err = %v, want ErrDisplayNameTaken", err)
	}
}

func TestGetCurrentJWT(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.UpsertUserAndIssue(ctx, "119548486276710400", "alice", "j1", "jwt-blob", ""); err != nil {
		t.Fatal(err)
	}
	gotJWT, gotName, err := d.GetCurrentJWT(ctx, "119548486276710400")
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
	if err := d.UpsertUserAndIssue(ctx, "119548486276710400", "alice", "j1", "jwt", ""); err != nil {
		t.Fatal(err)
	}
	u, err := d.GetUserByDisplayName(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.DiscordID != "119548486276710400" {
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
	if err := d.UpsertUserAndIssue(ctx, "119548486276710402","alice", "j1", "jwt1", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.UpsertUserAndIssue(ctx, "119548486276710402","alice", "j2", "jwt2", "reissue"); err != nil {
		t.Fatal(err)
	}

	u, _ := d.GetUserByDiscordID(ctx, "119548486276710402")
	if u.CurrentJTI != "j2" {
		t.Errorf("CurrentJTI = %q, want j2", u.CurrentJTI)
	}
	if black, _ := d.IsJTIBlacklisted(ctx, "j1"); !black {
		t.Error("old jti should be blacklisted on reissue")
	}
}

// Token reissue blacklists the old JTI, so the save made under it is no longer
// valid. The user drops from /ranking until they save again with the new token.
func TestUpsertUserAndIssue_ReissueDropsFromRanking(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.UpsertUserAndIssue(ctx, "119548486276710402", "alice", "j1", "jwt1", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Save(ctx, "alice", &savedata.Data{Score: 100}, "j1"); err != nil {
		t.Fatal(err)
	}

	// Reissue token — j1 gets blacklisted, so the save tied to j1 becomes invalid.
	if err := d.UpsertUserAndIssue(ctx, "119548486276710402", "alice", "j2", "jwt2", "reissue"); err != nil {
		t.Fatal(err)
	}

	rows, err := d.Ranking(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("ranking after reissue = %v, want empty until next save", rows)
	}
}

// Name change must NOT carry the new JTI to the old latest_saves row; the old
// name drops from /ranking until the user saves again under the new name.
func TestUpsertUserAndIssue_RenameDropsOldNameFromRanking(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()

	if err := d.UpsertUserAndIssue(ctx, "119548486276710402","alice", "j1", "jwt1", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Save(ctx, "alice", &savedata.Data{Score: 100}, "j1"); err != nil {
		t.Fatal(err)
	}

	// Rename: j1 gets blacklisted, new name is "alice2".
	if err := d.UpsertUserAndIssue(ctx, "119548486276710402","alice2", "j2", "jwt2", "rename"); err != nil {
		t.Fatal(err)
	}

	rows, err := d.Ranking(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("ranking after rename = %v, want empty until next save", rows)
	}
}

func TestUnregister_BlacklistsCurrentJTI(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	if err := d.UpsertUserAndIssue(ctx, "119548486276710402","alice", "j1", "jwt1", ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Unregister(ctx, "119548486276710402"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if black, _ := d.IsJTIBlacklisted(ctx, "j1"); !black {
		t.Error("current jti should be blacklisted after Unregister")
	}
	// users row preserved so the display_name stays reserved, but current_jti must be cleared.
	u, err := d.GetUserByDiscordID(ctx, "119548486276710402")
	if err != nil {
		t.Fatalf("user row should still exist: %v", err)
	}
	if u.DisplayName != "alice" {
		t.Errorf("DisplayName = %q, want alice", u.DisplayName)
	}
	if u.CurrentJTI != "" {
		t.Errorf("CurrentJTI = %q, want empty after Unregister", u.CurrentJTI)
	}
}

func TestUnregister_NotRegistered(t *testing.T) {
	d := newTestDB(t, nil)
	if err := d.Unregister(context.Background(), "119548486276710999"); !errors.Is(err, db.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

func TestReleaseDisplayName(t *testing.T) {
	d := newTestDB(t, nil)
	ctx := context.Background()
	if err := d.UpsertUserAndIssue(ctx, "119548486276710403", "victim_name", "j_atk", "jwt", ""); err != nil {
		t.Fatal(err)
	}
	prior, err := d.ReleaseDisplayName(ctx, "victim_name", "hijack")
	if err != nil {
		t.Fatalf("ReleaseDisplayName: %v", err)
	}
	if prior != "119548486276710403" {
		t.Errorf("prior = %q, want 119548486276710403", prior)
	}
	if black, _ := d.IsJTIBlacklisted(ctx, "j_atk"); !black {
		t.Error("attacker jti should be blacklisted")
	}
	// users row gone — legitimate owner can now register the name.
	if err := d.UpsertUserAndIssue(ctx, "119548486276710404", "victim_name", "j_vic", "jwt2", ""); err != nil {
		t.Fatalf("legitimate registration after release should succeed: %v", err)
	}
}

func TestReleaseDisplayName_NotFound(t *testing.T) {
	d := newTestDB(t, nil)
	if _, err := d.ReleaseDisplayName(context.Background(), "ghost", "x"); !errors.Is(err, db.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

func TestGetUserByDiscordIDNotFound(t *testing.T) {
	d := newTestDB(t, nil)
	if _, err := d.GetUserByDiscordID(context.Background(), "119548486276711000"); !errors.Is(err, db.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}
