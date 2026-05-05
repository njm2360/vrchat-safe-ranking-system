// Package e2e wires together the real DB, real HTTP handlers, real
// registration service and the real vrcclient to catch wiring bugs that
// per-package unit tests miss.
package e2e

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/api"
	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/idgen"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
	"github.com/njm2360/vrchat-ranking-system/internal/vrcclient"
)

const (
	jwtSecret  = "e2e-jwt-secret-padded-to-32-bytes-yes"
	saveSecret = "e2e-save-secret-padded-to-32-bytes-pls"
	loadSecret = "e2e-load-secret-padded-to-32-bytes-pls"
)

type harness struct {
	t       *testing.T
	clock   *clock.Fake
	idgen   *idgen.Sequential
	db      *db.DB
	issuer  *auth.JWTIssuer
	regSvc  *registration.Service
	server  *httptest.Server
	client  *vrcclient.Client
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	fc := clock.NewFake(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
	ig := idgen.NewSequential("id")
	path := filepath.Join(t.TempDir(), "e2e.db")

	d, err := db.Open(path, db.WithClock(fc))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	issuer := auth.NewJWTIssuer([]byte(jwtSecret), auth.WithClock(fc), auth.WithIDGen(ig))
	regSvc := registration.New(d, issuer)

	apiCfg := api.Config{
		HMACSaveSecret:   []byte(saveSecret),
		HMACLoadSecret:   []byte(loadSecret),
		TicketTTL:        5 * time.Minute,
		ChallengeRateTTL: 100 * time.Millisecond,
	}
	silentLog := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(api.New(apiCfg, d, d, issuer, ig, silentLog).Handler())
	t.Cleanup(srv.Close)

	client := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))

	return &harness{
		t:      t,
		clock:  fc,
		idgen:  ig,
		db:     d,
		issuer: issuer,
		regSvc: regSvc,
		server: srv,
		client: client,
	}
}

// register simulates the full Discord-bot path: VRC → /challenge → Discord
// /register → bot calls registration.Service.
func (h *harness) register(discordID, displayName string) string {
	h.t.Helper()
	uuid, err := h.client.RequestChallenge(context.Background(), displayName)
	if err != nil {
		h.t.Fatalf("RequestChallenge: %v", err)
	}
	res, err := h.regSvc.Register(context.Background(), discordID, uuid)
	if err != nil {
		h.t.Fatalf("Register: %v", err)
	}
	return res.JWT
}

func TestE2E_HappyPath(t *testing.T) {
	h := newHarness(t)
	jwt := h.register("discord-1", "alice")

	body, err := h.client.Save(context.Background(), vrcclient.SaveParams{UserID: "alice", Score: 1234, JWT: jwt})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if body != "OK ranked" {
		t.Errorf("save body = %q, want 'OK ranked'", body)
	}

	loaded, err := h.client.Load(context.Background(), vrcclient.LoadParams{UserID: "alice", JWT: jwt})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded != "1234" {
		t.Errorf("loaded = %q, want '1234'", loaded)
	}

	rows, _ := h.db.Ranking(context.Background(), 10)
	if len(rows) != 1 || rows[0].DisplayName != "alice" || rows[0].Score != 1234 {
		t.Errorf("ranking = %+v", rows)
	}
}

func TestE2E_RenameInvalidatesOldEntry(t *testing.T) {
	h := newHarness(t)

	jwt1 := h.register("discord-1", "alice")
	_, _ = h.client.Save(context.Background(), vrcclient.SaveParams{UserID: "alice", Score: 100, JWT: jwt1})

	// Rate limit on challenge — advance fake clock past the 100ms window.
	h.clock.Advance(time.Second)

	jwt2 := h.register("discord-1", "alice2")
	_, _ = h.client.Save(context.Background(), vrcclient.SaveParams{UserID: "alice2", Score: 999, JWT: jwt2})

	rows, _ := h.db.Ranking(context.Background(), 10)
	if len(rows) != 1 {
		t.Fatalf("ranking len = %d, want 1 (old entry should be excluded)", len(rows))
	}
	if rows[0].DisplayName != "alice2" {
		t.Errorf("ranking[0] = %s, want alice2", rows[0].DisplayName)
	}
}

func TestE2E_BanHidesUserFromRanking(t *testing.T) {
	h := newHarness(t)
	jwt := h.register("discord-1", "alice")
	_, _ = h.client.Save(context.Background(), vrcclient.SaveParams{UserID: "alice", Score: 1234, JWT: jwt})

	if err := h.db.Ban(context.Background(), "discord-1", "test"); err != nil {
		t.Fatal(err)
	}

	rows, _ := h.db.Ranking(context.Background(), 10)
	if len(rows) != 0 {
		t.Errorf("ranking should be empty after ban, got %+v", rows)
	}
}

func TestE2E_BannedUserCannotRegister(t *testing.T) {
	h := newHarness(t)
	if err := h.db.Ban(context.Background(), "discord-banned", "test"); err != nil {
		t.Fatal(err)
	}

	uuid, err := h.client.RequestChallenge(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.regSvc.Register(context.Background(), "discord-banned", uuid)
	if !errors.Is(err, registration.ErrBanned) {
		t.Fatalf("err = %v, want ErrBanned", err)
	}

	// Ticket should still be unconsumed (registration was rejected before consume)
	t2, err := h.db.GetTicket(context.Background(), uuid)
	if err != nil {
		t.Fatal(err)
	}
	if t2.ConsumedAt != nil {
		t.Error("ticket should remain unconsumed when registration is rejected by ban")
	}
}

func TestE2E_SaveWithoutJWT_Rejected(t *testing.T) {
	h := newHarness(t)

	_, err := h.client.Save(context.Background(), vrcclient.SaveParams{UserID: "anon", Score: 9999})
	if err == nil {
		t.Fatal("expected error for save without jwt, got nil")
	}
}

func TestE2E_LoadWithoutJWT_Rejected(t *testing.T) {
	h := newHarness(t)

	_, err := h.client.Load(context.Background(), vrcclient.LoadParams{UserID: "anon"})
	if err == nil {
		t.Fatal("expected error for load without jwt, got nil")
	}
}

func TestE2E_TicketReuseRejected(t *testing.T) {
	h := newHarness(t)
	uuid, err := h.client.RequestChallenge(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := h.regSvc.Register(context.Background(), "discord-1", uuid); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err = h.regSvc.Register(context.Background(), "discord-2", uuid)
	if !errors.Is(err, registration.ErrTicketUsed) {
		t.Errorf("second register err = %v, want ErrTicketUsed", err)
	}
}

func TestE2E_TicketExpiryRejected(t *testing.T) {
	h := newHarness(t)
	uuid, err := h.client.RequestChallenge(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(10 * time.Minute)

	_, err = h.regSvc.Register(context.Background(), "d", uuid)
	if !errors.Is(err, registration.ErrTicketExpired) {
		t.Errorf("err = %v, want ErrTicketExpired", err)
	}
}

func TestE2E_ChallengeRateLimit(t *testing.T) {
	h := newHarness(t)

	if _, err := h.client.RequestChallenge(context.Background(), "alice"); err != nil {
		t.Fatal(err)
	}
	// Within rate window (100ms): expect error
	if _, err := h.client.RequestChallenge(context.Background(), "alice"); err == nil {
		t.Error("expected rate-limit error on second challenge")
	}
	// Advance past window: should succeed
	h.clock.Advance(time.Second)
	if _, err := h.client.RequestChallenge(context.Background(), "alice"); err != nil {
		t.Errorf("after window err = %v", err)
	}
}
