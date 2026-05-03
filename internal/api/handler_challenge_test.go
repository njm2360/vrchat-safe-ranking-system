package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/api"
)

func newServer(tickets api.TicketStore, saves api.SaveStore, jwt api.JWTVerifier, idgen api.IDGen) http.Handler {
	cfg := api.Config{
		HMACSaveSecret:   []byte("save-secret"),
		HMACLoadSecret:   []byte("load-secret"),
		TicketTTL:        5 * time.Minute,
		ChallengeRateTTL: time.Minute,
	}
	return api.New(cfg, tickets, saves, jwt, idgen, nil).Handler()
}

func get(t *testing.T, h http.Handler, target string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	return rr, string(body)
}

func TestChallenge_Success(t *testing.T) {
	tickets := &fakeTicketStore{allowed: true}
	h := newServer(tickets, &fakeSaveStore{}, &fakeJWT{}, fakeIDGen{ID: "ticket-uuid-1"})

	rr, body := get(t, h, "/challenge?name=alice")
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%q", rr.Code, body)
	}
	if body != "ticket-uuid-1" {
		t.Errorf("body = %q, want ticket-uuid-1", body)
	}
	if len(tickets.insertCalls) != 1 || tickets.insertCalls[0].DisplayName != "alice" {
		t.Errorf("insert calls = %+v", tickets.insertCalls)
	}
	if tickets.upsertCalls != 1 {
		t.Errorf("upsert called %d times, want 1", tickets.upsertCalls)
	}
}

func TestChallenge_RateLimited(t *testing.T) {
	tickets := &fakeTicketStore{allowed: false}
	h := newServer(tickets, &fakeSaveStore{}, &fakeJWT{}, fakeIDGen{ID: "X"})

	rr, _ := get(t, h, "/challenge?name=alice")
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", rr.Code)
	}
	if len(tickets.insertCalls) != 0 {
		t.Error("InsertTicket should not be called when rate-limited")
	}
}

func TestChallenge_InvalidName(t *testing.T) {
	cases := []struct {
		label, raw string
	}{
		{"empty", ""},
		{"too long", strings.Repeat("a", 65)},
		{"null byte", "with%00null"},
		{"newline", "with%0Anewline"},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			h := newServer(&fakeTicketStore{allowed: true}, &fakeSaveStore{}, &fakeJWT{}, fakeIDGen{})
			rr, _ := get(t, h, "/challenge?name="+tc.raw)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rr.Code)
			}
		})
	}
}
