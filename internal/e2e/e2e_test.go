// Package e2e wires together the real DB, real HTTP handlers, real
// registration service, a fake OAuth provider, and the real vrcclient to
// catch wiring bugs that per-package unit tests miss.
package e2e

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/api"
	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/idgen"
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
	"github.com/njm2360/vrchat-ranking-system/internal/vrcclient"
)

const (
	jwtSecret  = "e2e-jwt-secret-padded-to-32-bytes-yes"
	saveSecret = "e2e-save-secret-padded-to-32-bytes-pls"
	loadSecret = "e2e-load-secret-padded-to-32-bytes-pls"
)

type harness struct {
	t        *testing.T
	clock    *clock.Fake
	idgen    *idgen.Sequential
	db       *db.DB
	issuer   *auth.JWTIssuer
	regSvc   *registration.Service
	provider *oauth.Fake
	server   *httptest.Server
	client   *vrcclient.Client
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
	provider := oauth.NewFake("placeholder", "default-code", "default-discord")

	apiCfg := api.Config{
		HMACSaveSecret: []byte(saveSecret),
		HMACLoadSecret: []byte(loadSecret),
		OAuthStateTTL:  5 * time.Minute,
		SessionTTL:     15 * time.Minute,
	}
	silentLog := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(api.New(apiCfg, d, d, issuer, ig, provider, regSvc, silentLog).Handler())
	t.Cleanup(srv.Close)
	provider.CallbackURL = srv.URL + "/auth/callback"

	client := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))

	return &harness{
		t: t, clock: fc, idgen: ig, db: d, issuer: issuer, regSvc: regSvc,
		provider: provider, server: srv, client: client,
	}
}

// newBrowserClient returns an http.Client that follows redirects and
// retains cookies between requests — i.e. acts like a real browser
// driving the portal flow.
func (h *harness) newBrowserClient() *http.Client {
	h.t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		h.t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{Jar: jar}
}

// portalGet drives /auth/start → 302 to provider → fake provider redirects
// back to /auth/callback → 303 to /auth/portal. Returns the cookie-bearing
// client (so the caller can POST register/unregister) and the portal HTML.
func (h *harness) portalGet(displayName, discordID string) (*http.Client, string) {
	h.t.Helper()
	code := "code-" + discordID + "-" + displayName
	h.provider.CodeToUser[code] = &oauth.User{ID: discordID}
	h.provider.NextCode = code

	client := h.newBrowserClient()
	startURL := h.server.URL + "/auth/start?name=" + url.QueryEscape(displayName)
	resp, err := client.Get(startURL)
	if err != nil {
		h.t.Fatalf("auth start: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("portal view status = %d: %s", resp.StatusCode, string(body))
	}
	return client, string(body)
}

// portalAct fetches the portal page, confirms the form for the given
// action (register / unregister) is rendered, and submits it via POST
// using the browser-like client that carries the portal session cookie.
// Returns the result body.
func (h *harness) portalAct(displayName, discordID, action string) string {
	h.t.Helper()
	client, portal := h.portalGet(displayName, discordID)
	if !hasActionForm(portal, action) {
		h.t.Fatalf("no %s action form in portal body: %s", action, portal)
	}
	resp, err := client.PostForm(h.server.URL+"/auth/"+action, url.Values{})
	if err != nil {
		h.t.Fatalf("portal %s: %v", action, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		h.t.Fatalf("portal %s status = %d: %s", action, resp.StatusCode, string(body))
	}
	return string(body)
}

// register drives the full register flow (start → callback → portal click)
// and returns the freshly-minted JWT.
func (h *harness) register(discordID, displayName string) string {
	h.t.Helper()
	body := h.portalAct(displayName, discordID, "register")
	jwt := extractJWT(body)
	if jwt == "" {
		h.t.Fatalf("could not extract JWT from response body: %s", body)
	}
	return jwt
}

// hasActionForm reports whether the portal page renders a <form> whose
// action attribute matches /auth/<action>. With the session token in a
// cookie, the form no longer carries it, so this is a presence check.
func hasActionForm(body, action string) bool {
	re := regexp.MustCompile(`<form[^>]*action="/auth/` + regexp.QuoteMeta(action) + `"`)
	return re.MatchString(body)
}

// extractJWT pulls the JWT out of the id="jwt-token" block in the success
// page rendered by handler_auth.go.
func extractJWT(body string) string {
	const marker = `id="jwt-token"`
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	gt := strings.Index(body[start:], ">")
	if gt < 0 {
		return ""
	}
	rest := body[start+gt+1:]
	end := strings.Index(rest, "</div>")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func TestE2E_HappyPath(t *testing.T) {
	h := newHarness(t)
	jwt := h.register("discord-1", "alice")

	body, err := h.client.Save(context.Background(), vrcclient.SaveParams{Data: &savedata.Data{Score: 1234, GeneratedAt: time.Unix(1000, 0).UTC()}, JWT: jwt, DisplayName: "alice"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if body != "success" {
		t.Errorf("save body = %q, want 'success'", body)
	}

	loaded, err := h.client.Load(context.Background(), vrcclient.LoadParams{JWT: jwt, DisplayName: "alice"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil || loaded.Score != 1234 {
		t.Errorf("loaded = %+v, want Score=1234", loaded)
	}

	rows, _ := h.db.Ranking(context.Background(), 10, false)
	if len(rows) != 1 || rows[0].DisplayName != "alice" || rows[0].Score != 1234 {
		t.Errorf("ranking = %+v", rows)
	}
}

func TestE2E_RenameInvalidatesOldEntry(t *testing.T) {
	h := newHarness(t)

	jwt1 := h.register("discord-1", "alice")
	_, _ = h.client.Save(context.Background(), vrcclient.SaveParams{Data: &savedata.Data{Score: 100, GeneratedAt: time.Unix(1000, 0).UTC()}, JWT: jwt1, DisplayName: "alice"})

	jwt2 := h.register("discord-1", "alice2")
	_, _ = h.client.Save(context.Background(), vrcclient.SaveParams{Data: &savedata.Data{Score: 999, GeneratedAt: time.Unix(1001, 0).UTC()}, JWT: jwt2, DisplayName: "alice2"})

	rows, _ := h.db.Ranking(context.Background(), 10, false)
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
	_, _ = h.client.Save(context.Background(), vrcclient.SaveParams{Data: &savedata.Data{Score: 1234, GeneratedAt: time.Unix(1000, 0).UTC()}, JWT: jwt, DisplayName: "alice"})

	if err := h.db.BanDiscordID(context.Background(), "discord-1", "test"); err != nil {
		t.Fatal(err)
	}

	rows, _ := h.db.Ranking(context.Background(), 10, false)
	if len(rows) != 0 {
		t.Errorf("ranking should be empty after ban, got %+v", rows)
	}
}

func TestE2E_BannedUserCannotRegister(t *testing.T) {
	h := newHarness(t)
	if err := h.db.BanDiscordID(context.Background(), "discord-banned", "test"); err != nil {
		t.Fatal(err)
	}
	_, err := h.regSvc.Register(context.Background(), "discord-banned", "alice")
	if !errors.Is(err, registration.ErrBanned) {
		t.Fatalf("err = %v, want ErrBanned", err)
	}
}

func TestE2E_AnonymousSave_OK(t *testing.T) {
	h := newHarness(t)

	body, err := h.client.Save(context.Background(), vrcclient.SaveParams{Data: &savedata.Data{Score: 42, GeneratedAt: time.Unix(1000, 0).UTC()}, DisplayName: "ghost"})
	if err != nil {
		t.Fatalf("anonymous save should succeed: %v", err)
	}
	if body != "success" {
		t.Errorf("body = %q, want 'success'", body)
	}
}

func TestE2E_SaveWithoutJWT_Rejected_ForRegisteredUser(t *testing.T) {
	h := newHarness(t)
	h.register("discord-anon", "alice")

	_, err := h.client.Save(context.Background(), vrcclient.SaveParams{Data: &savedata.Data{Score: 9999, GeneratedAt: time.Unix(1000, 0).UTC()}, DisplayName: "alice"})
	if err == nil {
		t.Fatal("expected error for registered user save without jwt, got nil")
	}
}

func TestE2E_LoadWithoutJWT_Rejected_ForRegisteredUser(t *testing.T) {
	h := newHarness(t)
	h.register("discord-anon", "alice")

	_, err := h.client.Load(context.Background(), vrcclient.LoadParams{DisplayName: "alice"})
	if err == nil {
		t.Fatal("expected error for registered user load without jwt, got nil")
	}
}

func TestE2E_OAuthStateSingleUse(t *testing.T) {
	h := newHarness(t)

	// Issue a state but don't consume it via the normal flow.
	if err := h.db.InsertOAuthState(context.Background(), "manual-state", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	h.provider.CodeToUser["c"] = &oauth.User{ID: "discord-x"}

	noRedirect := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	// First callback succeeds — happy path is a 303 redirect to the portal.
	resp, err := noRedirect.Get(h.server.URL + "/auth/callback?code=c&state=manual-state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("first callback status = %d, want 303", resp.StatusCode)
	}
	// Second callback with the same state must fail (single-use).
	resp2, err := noRedirect.Get(h.server.URL + "/auth/callback?code=c&state=manual-state")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode == http.StatusSeeOther {
		t.Errorf("expected non-303 on state reuse, got %d", resp2.StatusCode)
	}
}
