package registration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
)

type fakeStore struct {
	banned       bool
	bannedErr    error
	dnBanned     bool
	dnBannedErr  error
	getUserRet   *db.User
	getUserErr   error
	upsertCalls  []upsertCall
	upsertErr    error
}

type upsertCall struct{ DiscordID, DisplayName, JTI, Reason string }

func (f *fakeStore) IsDiscordIDBanned(_ context.Context, _ string) (bool, error) {
	return f.banned, f.bannedErr
}
func (f *fakeStore) IsDisplayNameBanned(_ context.Context, _ string) (bool, error) {
	return f.dnBanned, f.dnBannedErr
}
func (f *fakeStore) GetUserByDiscordID(_ context.Context, _ string) (*db.User, error) {
	return f.getUserRet, f.getUserErr
}
func (f *fakeStore) UpsertUserAndIssue(_ context.Context, did, dn, jti, reason string) error {
	f.upsertCalls = append(f.upsertCalls, upsertCall{did, dn, jti, reason})
	return f.upsertErr
}

type fakeIssuer struct {
	jwtVal, jtiVal string
	err            error
	calls          int
}

func (f *fakeIssuer) Issue(_, _ string) (string, string, error) {
	f.calls++
	return f.jwtVal, f.jtiVal, f.err
}

func TestRegister_NewUser(t *testing.T) {
	store := &fakeStore{getUserErr: db.ErrUserNotFound}
	issuer := &fakeIssuer{jwtVal: "jwt-blob", jtiVal: "jti-1"}
	svc := registration.New(store, issuer)

	res, err := svc.Register(context.Background(), "discord-1", "alice")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.IsRenewal {
		t.Error("expected IsRenewal=false for new user")
	}
	if res.JWT != "jwt-blob" || res.JTI != "jti-1" || res.DisplayName != "alice" {
		t.Errorf("result = %+v", res)
	}
	if len(store.upsertCalls) != 1 {
		t.Fatalf("upsert called %d times, want 1", len(store.upsertCalls))
	}
	if store.upsertCalls[0].JTI != "jti-1" || store.upsertCalls[0].DisplayName != "alice" {
		t.Errorf("upsert call = %+v", store.upsertCalls[0])
	}
}

func TestRegister_Renewal(t *testing.T) {
	store := &fakeStore{
		getUserRet: &db.User{DiscordID: "discord-1", DisplayName: "alice", CurrentJTI: "jti-old"},
	}
	issuer := &fakeIssuer{jwtVal: "new-jwt", jtiVal: "jti-new"}
	svc := registration.New(store, issuer)

	res, err := svc.Register(context.Background(), "discord-1", "alice2")
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsRenewal {
		t.Error("expected IsRenewal=true")
	}
	if res.PrevDisplayName != "alice" {
		t.Errorf("PrevDisplayName = %q, want alice", res.PrevDisplayName)
	}
	if res.DisplayName != "alice2" {
		t.Errorf("DisplayName = %q, want alice2", res.DisplayName)
	}
}

func TestRegister_NewUser_PrevDisplayNameEmpty(t *testing.T) {
	store := &fakeStore{getUserErr: db.ErrUserNotFound}
	svc := registration.New(store, &fakeIssuer{jwtVal: "j", jtiVal: "x"})
	res, err := svc.Register(context.Background(), "d", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if res.PrevDisplayName != "" {
		t.Errorf("PrevDisplayName should be empty for new user, got %q", res.PrevDisplayName)
	}
}

func TestRegister_BannedRejected(t *testing.T) {
	store := &fakeStore{banned: true}
	issuer := &fakeIssuer{jwtVal: "j", jtiVal: "x"}
	svc := registration.New(store, issuer)

	_, err := svc.Register(context.Background(), "discord-banned", "alice")
	if !errors.Is(err, registration.ErrBanned) {
		t.Fatalf("err = %v, want ErrBanned", err)
	}
	if issuer.calls != 0 {
		t.Errorf("issuer should not be called when banned (got %d calls)", issuer.calls)
	}
	if len(store.upsertCalls) != 0 {
		t.Error("upsert should not be called when banned")
	}
}

func TestRegister_DisplayNameBannedRejected(t *testing.T) {
	store := &fakeStore{getUserErr: db.ErrUserNotFound, dnBanned: true}
	issuer := &fakeIssuer{jwtVal: "j", jtiVal: "x"}
	svc := registration.New(store, issuer)

	_, err := svc.Register(context.Background(), "discord-1", "banned-name")
	if !errors.Is(err, registration.ErrDisplayNameBanned) {
		t.Fatalf("err = %v, want ErrDisplayNameBanned", err)
	}
	if issuer.calls != 0 {
		t.Errorf("issuer should not be called when display name is banned (got %d calls)", issuer.calls)
	}
}

func TestRegister_DisplayNameTakenMapped(t *testing.T) {
	store := &fakeStore{
		getUserErr: db.ErrUserNotFound,
		upsertErr:  db.ErrDisplayNameTaken,
	}
	svc := registration.New(store, &fakeIssuer{jwtVal: "j", jtiVal: "x"})
	_, err := svc.Register(context.Background(), "d", "alice")
	if !errors.Is(err, registration.ErrDisplayNameTaken) {
		t.Fatalf("err = %v, want ErrDisplayNameTaken", err)
	}
}

func TestRegister_UpsertError(t *testing.T) {
	store := &fakeStore{
		getUserErr: db.ErrUserNotFound,
		upsertErr:  errors.New("display_name conflict"),
	}
	issuer := &fakeIssuer{jwtVal: "j", jtiVal: "x"}
	svc := registration.New(store, issuer)
	if _, err := svc.Register(context.Background(), "d", "alice"); err == nil {
		t.Fatal("expected error from upsert")
	}
}

func TestRegister_GetUserUnexpectedError(t *testing.T) {
	store := &fakeStore{getUserErr: errors.New("db down")}
	svc := registration.New(store, &fakeIssuer{})
	if _, err := svc.Register(context.Background(), "d", "alice"); err == nil {
		t.Fatal("expected db error to propagate")
	}
}

func TestRegister_IssuerError(t *testing.T) {
	store := &fakeStore{getUserErr: db.ErrUserNotFound}
	issuer := &fakeIssuer{err: errors.New("boom")}
	svc := registration.New(store, issuer)

	if _, err := svc.Register(context.Background(), "d", "alice"); err == nil {
		t.Fatal("expected error from issuer")
	}
	if len(store.upsertCalls) != 0 {
		t.Error("upsert should not be called when issuer fails")
	}
}
