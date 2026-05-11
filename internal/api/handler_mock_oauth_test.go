package api_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
)

// Mock OAuth flow: /auth/start (mock) → 302 to /auth/mock-login (renders form) →
// POST /auth/mock-login → 302 to /auth/callback → 303 to /auth/portal → POST to commit.

func TestMockOAuth_StartRedirectsToMockLogin_ExplicitOverrides(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newMockServer(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, regSvc)

	rr, _ := get(t, h, "/auth/start?name=alice&fake_discord_id=100000000000000001&fake_username=alice.dev")
	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rr.Code)
	}
	parsed, _ := url.Parse(rr.Header().Get("Location"))
	if !strings.HasPrefix(rr.Header().Get("Location"), "/auth/mock-login?") {
		t.Errorf("Location prefix wrong: %q", rr.Header().Get("Location"))
	}
	if got := parsed.Query().Get("discord_id"); got != "100000000000000001" {
		t.Errorf("discord_id = %q", got)
	}
	if got := parsed.Query().Get("username"); got != "alice.dev" {
		t.Errorf("username = %q", got)
	}
	if parsed.Query().Get("state") == "" {
		t.Error("state empty in mock-login URL")
	}
}

// Without overrides: discord_id is auto-generated as a random 18-digit
// snowflake, username defaults to the supplied `name`.
func TestMockOAuth_StartAutoDefaults(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newMockServer(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, regSvc)

	rr, _ := get(t, h, "/auth/start?name=alice")
	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rr.Code)
	}
	parsed, _ := url.Parse(rr.Header().Get("Location"))
	id := parsed.Query().Get("discord_id")
	if len(id) != 18 {
		t.Errorf("discord_id length = %d (%q), want 18", len(id), id)
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			t.Errorf("discord_id contains non-digit: %q", id)
			break
		}
	}
	if got := parsed.Query().Get("username"); got != "alice" {
		t.Errorf("username = %q, want alice (defaulted from name)", got)
	}
}

// Two consecutive sessions without overrides must yield distinct random IDs.
func TestMockOAuth_StartAutoIDIsRandom(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newMockServer(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, regSvc)

	rr1, _ := get(t, h, "/auth/start?name=alice")
	rr2, _ := get(t, h, "/auth/start?name=bob")
	id1 := mustQuery(t, rr1.Header().Get("Location"), "discord_id")
	id2 := mustQuery(t, rr2.Header().Get("Location"), "discord_id")
	if id1 == id2 {
		t.Errorf("two auto-generated discord_ids matched: %q", id1)
	}
}

func mustQuery(t *testing.T, raw, key string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get(key)
}

func TestMockOAuth_MockLoginRendersForm(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newMockServer(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, regSvc)

	rr, body := get(t, h, "/auth/mock-login?state=anystate&discord_id=123456&username=alice.dev")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(body, `value="123456"`) {
		t.Errorf("body missing pre-filled discord_id; body=%q", body)
	}
	if !strings.Contains(body, `name="state"`) {
		t.Errorf("body missing state field; body=%q", body)
	}
}

func TestMockOAuth_MockLoginPostRedirectsToCallback(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newMockServer(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, regSvc)

	rr, _ := postForm(t, h, "/auth/mock-login", url.Values{
		"state":      {"anystate"},
		"discord_id": {"123456"},
		"username":   {"alice.dev"},
	})
	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rr.Code)
	}
	loc := rr.Header().Get("Location")
	parsed, _ := url.Parse(loc)
	if parsed.Path != "/auth/callback" {
		t.Errorf("path = %q, want /auth/callback", parsed.Path)
	}
	if parsed.Query().Get("code") != "123456|alice.dev" {
		t.Errorf("code = %q, want 123456|alice.dev", parsed.Query().Get("code"))
	}
	if parsed.Query().Get("state") != "anystate" {
		t.Errorf("state = %q", parsed.Query().Get("state"))
	}
}

func TestMockOAuth_FullFlow_Register(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newMockServer(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, regSvc)

	rr1, _ := get(t, h, "/auth/start?name=alice&fake_discord_id=200000000000000001&fake_username=alice.dev")
	if rr1.Code != http.StatusFound {
		t.Fatalf("start status = %d", rr1.Code)
	}
	loc1 := rr1.Header().Get("Location")

	rr2get, _ := get(t, h, loc1)
	if rr2get.Code != http.StatusOK {
		t.Fatalf("mock-login GET status = %d, want 200", rr2get.Code)
	}
	parsed1, _ := url.Parse(loc1)
	q1 := parsed1.Query()
	rr2, _ := postForm(t, h, "/auth/mock-login", url.Values{
		"state":      {q1.Get("state")},
		"discord_id": {q1.Get("discord_id")},
		"username":   {q1.Get("username")},
	})
	if rr2.Code != http.StatusFound {
		t.Fatalf("mock-login POST status = %d, want 302", rr2.Code)
	}
	loc2 := rr2.Header().Get("Location")

	rr3, body3, cookie := followCallback(t, h, loc2)
	if rr3.Code != http.StatusOK {
		t.Fatalf("portal-view status = %d body=%q", rr3.Code, body3)
	}
	if !strings.Contains(body3, "alice.dev") {
		t.Errorf("portal body missing username; body=%q", body3)
	}
	if !strings.Contains(body3, "alice") {
		t.Errorf("portal body missing display name; body=%q", body3)
	}
	if _, err := d.GetUserByDiscordID(t.Context(), "200000000000000001"); err == nil {
		t.Error("user row should not exist before /auth/portal POST")
	}

	if !hasActionForm(body3, "register") {
		t.Fatalf("no register form in portal body: %s", body3)
	}
	rr4, body4 := portalPost(t, h, cookie, "register")
	if rr4.Code != http.StatusOK {
		t.Fatalf("portal status = %d body=%q", rr4.Code, body4)
	}
	if !strings.Contains(body4, "jwt-token") {
		t.Errorf("final body missing JWT block; body=%q", body4)
	}

	user, err := d.GetUserByDiscordID(t.Context(), "200000000000000001")
	if err != nil {
		t.Fatalf("user lookup: %v", err)
	}
	if user.DisplayName != "alice" {
		t.Errorf("display_name = %q", user.DisplayName)
	}
}

func TestMockOAuth_RejectsNonNumericDiscordID(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newMockServer(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, regSvc)

	for _, bad := range []string{"alice", "user-1", "123abc", "with space", "with/slash", "123-456"} {
		rr, _ := get(t, h, "/auth/start?name=alice&fake_discord_id="+url.QueryEscape(bad))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("fake_discord_id=%q: status = %d, want 400", bad, rr.Code)
		}
	}
}

func TestMockOAuth_RejectsInvalidUsername(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newMockServer(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, regSvc)

	// Reject the '|' separator. Other Unicode / uppercase characters are allowed.
	for _, bad := range []string{"alice|bar"} {
		rr, _ := get(t, h, "/auth/start?name=alice&fake_username="+url.QueryEscape(bad))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("fake_username=%q: status = %d, want 400", bad, rr.Code)
		}
	}
}

// In Discord (non-mock) mode, /auth/mock-login must not be registered.
func TestMockOAuth_DisabledRouteWhenNotMockMode(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, oauth.NewFake("https://x/cb", "c", "d"), regSvc)
	rr, _ := get(t, h, "/auth/mock-login?state=s&discord_id=123&username=alice.dev")
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 404/405 (route should be unregistered)", rr.Code)
	}
}
