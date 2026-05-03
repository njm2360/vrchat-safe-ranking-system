package registration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
)

// fakeStore implements registration.Store with controllable behaviour.
type fakeStore struct {
	banned      bool
	bannedErr   error
	consumeRet  *db.Ticket
	consumeErr  error
	consumeCalls int
	getUserRet  *db.User
	getUserErr  error
	upsertCalls []upsertCall
	upsertErr   error
}

type upsertCall struct{ DiscordID, DisplayName, JTI, JWT, Reason string }

func (f *fakeStore) IsBanned(_ context.Context, _ string) (bool, error) {
	return f.banned, f.bannedErr
}
func (f *fakeStore) ConsumeTicket(_ context.Context, _ string) (*db.Ticket, error) {
	f.consumeCalls++
	return f.consumeRet, f.consumeErr
}
func (f *fakeStore) GetUserByDiscordID(_ context.Context, _ string) (*db.User, error) {
	return f.getUserRet, f.getUserErr
}
func (f *fakeStore) UpsertUserAndIssue(_ context.Context, did, dn, jti, jwt, reason string) error {
	f.upsertCalls = append(f.upsertCalls, upsertCall{did, dn, jti, jwt, reason})
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
	store := &fakeStore{
		consumeRet: &db.Ticket{UUID: "u", DisplayName: "alice", IssuedAt: time.Now()},
		getUserErr: db.ErrUserNotFound,
	}
	issuer := &fakeIssuer{jwtVal: "jwt-blob", jtiVal: "jti-1"}
	svc := registration.New(store, issuer)

	res, err := svc.Register(context.Background(), "discord-1", "u")
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
		consumeRet: &db.Ticket{UUID: "u", DisplayName: "alice2"},
		getUserRet: &db.User{DiscordID: "discord-1", DisplayName: "alice", CurrentJTI: "jti-old"},
	}
	issuer := &fakeIssuer{jwtVal: "new-jwt", jtiVal: "jti-new"}
	svc := registration.New(store, issuer)

	res, err := svc.Register(context.Background(), "discord-1", "u")
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
	store := &fakeStore{
		consumeRet: &db.Ticket{UUID: "u", DisplayName: "alice"},
		getUserErr: db.ErrUserNotFound,
	}
	svc := registration.New(store, &fakeIssuer{jwtVal: "j", jtiVal: "x"})
	res, err := svc.Register(context.Background(), "d", "u")
	if err != nil {
		t.Fatal(err)
	}
	if res.PrevDisplayName != "" {
		t.Errorf("PrevDisplayName should be empty for new user, got %q", res.PrevDisplayName)
	}
}

func TestRegister_BannedRejected(t *testing.T) {
	store := &fakeStore{
		banned: true,
		// These would be returned if the ban check were skipped, but they
		// must NOT be reached.
		consumeRet: &db.Ticket{DisplayName: "alice"},
	}
	svc := registration.New(store, &fakeIssuer{jwtVal: "j", jtiVal: "x"})

	_, err := svc.Register(context.Background(), "discord-banned", "u")
	if !errors.Is(err, registration.ErrBanned) {
		t.Fatalf("err = %v, want ErrBanned", err)
	}
	if store.consumeCalls != 0 {
		t.Errorf("ticket should not be consumed when user is banned (got %d calls)", store.consumeCalls)
	}
}

func TestRegister_TicketErrors(t *testing.T) {
	cases := []struct {
		name       string
		consumeErr error
		want       error
	}{
		{"not found", db.ErrTicketNotFound, registration.ErrTicketNotFound},
		{"expired", db.ErrTicketExpired, registration.ErrTicketExpired},
		{"used", db.ErrTicketUsed, registration.ErrTicketUsed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := registration.New(&fakeStore{consumeErr: tc.consumeErr}, &fakeIssuer{})
			_, err := svc.Register(context.Background(), "d", "u")
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestRegister_UpsertError(t *testing.T) {
	store := &fakeStore{
		consumeRet: &db.Ticket{DisplayName: "alice"},
		getUserErr: db.ErrUserNotFound,
		upsertErr:  errors.New("display_name conflict"),
	}
	issuer := &fakeIssuer{jwtVal: "j", jtiVal: "x"}
	svc := registration.New(store, issuer)
	if _, err := svc.Register(context.Background(), "d", "u"); err == nil {
		t.Fatal("expected error from upsert")
	}
}

func TestRegister_GetUserUnexpectedError(t *testing.T) {
	store := &fakeStore{
		consumeRet: &db.Ticket{DisplayName: "alice"},
		getUserErr: errors.New("db down"),
	}
	svc := registration.New(store, &fakeIssuer{})
	if _, err := svc.Register(context.Background(), "d", "u"); err == nil {
		t.Fatal("expected db error to propagate")
	}
}

func TestRegister_IssuerError(t *testing.T) {
	store := &fakeStore{
		consumeRet: &db.Ticket{DisplayName: "alice"},
		getUserErr: db.ErrUserNotFound,
	}
	issuer := &fakeIssuer{err: errors.New("boom")}
	svc := registration.New(store, issuer)

	if _, err := svc.Register(context.Background(), "d", "u"); err == nil {
		t.Fatal("expected error from issuer")
	}
	if len(store.upsertCalls) != 0 {
		t.Error("upsert should not be called when issuer fails")
	}
}
