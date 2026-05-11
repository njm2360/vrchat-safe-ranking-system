package api_test

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/clock"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/idgen"
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
)

// extractActionToken finds the <form> on the portal page whose action URL is
// /auth/<action> and returns the session token from that form's hidden "token"
// input. Empty string if no such form is rendered (e.g. action suppressed).
func extractActionToken(body, action string) string {
	formRe := regexp.MustCompile(`(?s)<form[^>]*action="/auth/` + action + `"[^>]*>(.*?)</form>`)
	tokenRe := regexp.MustCompile(`name="token"\s+value="([^"]*)"`)
	for _, m := range formRe.FindAllStringSubmatch(body, -1) {
		if tm := tokenRe.FindStringSubmatch(m[1]); len(tm) >= 2 {
			return html.UnescapeString(tm[1])
		}
	}
	return ""
}

// portalPost submits the named action against /auth/<action> with the
// given session token.
func portalPost(t *testing.T, h http.Handler, token, action string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	return postForm(t, h, "/auth/"+action, url.Values{"token": {token}})
}

func TestAuthStart_RedirectsToProvider(t *testing.T) {
	store := &fakeAuthStore{}
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-1")
	h := newServerFull(&fakeSaveStore{}, store, &fakeJWT{}, fakeIDGen{}, provider, nil)

	rr, _ := get(t, h, "/auth/start?name=alice")
	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "code=code-1") {
		t.Errorf("Location = %q, missing code=code-1", loc)
	}
	if len(store.insertCalls) != 1 || store.insertCalls[0].ProposedName != "alice" {
		t.Errorf("insert calls = %+v", store.insertCalls)
	}
	parsed, _ := url.Parse(loc)
	state := parsed.Query().Get("state")
	if state == "" || state != store.insertCalls[0].State {
		t.Errorf("state in URL %q does not match stored state %q", state, store.insertCalls[0].State)
	}
}

func TestAuthStart_RequiresValidName(t *testing.T) {
	h := newServerFull(&fakeSaveStore{}, &fakeAuthStore{}, &fakeJWT{}, fakeIDGen{}, oauth.NewFake("", "c", "d"), nil)
	for _, bad := range []string{""} {
		rr, _ := get(t, h, "/auth/start?name="+url.QueryEscape(bad))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("name=%q: status = %d, want 400", bad, rr.Code)
		}
	}
}

func TestAuthStart_BannedDisplayName_Rejected(t *testing.T) {
	store := &fakeAuthStore{dnBanned: true}
	h := newServerFull(&fakeSaveStore{}, store, &fakeJWT{}, fakeIDGen{}, oauth.NewFake("", "c", "d"), nil)

	rr, _ := get(t, h, "/auth/start?name=alice")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if len(store.insertCalls) != 0 {
		t.Errorf("state was inserted despite banned display name")
	}
}

// helper to build a real registration.Service against a temp DB.
func newRegSvc(t *testing.T) (*registration.Service, *db.DB, *clock.Fake, *idgen.Sequential) {
	t.Helper()
	fc := clock.NewFake(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
	ig := idgen.NewSequential("jti")
	d, err := db.Open(filepath.Join(t.TempDir(), "auth.db"), db.WithClock(fc))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	issuer := auth.NewJWTIssuer([]byte("e2e-jwt-secret-padded-to-32-bytes-yes"), auth.WithClock(fc), auth.WithIDGen(ig))
	return registration.New(d, issuer), d, fc, ig
}

// driveRegister runs OAuth callback + portal "register" click against h
// for (discord_id, display_name). Returns the final response body. Uses
// the supplied fake provider; teaches it the code → discord_id mapping
// so callers don't need to pre-arrange CodeToUser.
func driveRegister(t *testing.T, h http.Handler, d *db.DB, provider *oauth.Fake, discordID, displayName string) string {
	t.Helper()
	code := "code-" + discordID + "-" + displayName
	provider.CodeToUser[code] = &oauth.User{ID: discordID}
	state := "rg-" + discordID + "-" + displayName
	if err := d.InsertOAuthState(t.Context(), state, displayName, time.Hour); err != nil {
		t.Fatal(err)
	}
	rr1, body1 := followCallback(t, h, "/auth/callback?code="+url.QueryEscape(code)+"&state="+state)
	if rr1.Code != http.StatusOK {
		t.Fatalf("callback status = %d body=%s", rr1.Code, body1)
	}
	tok := extractActionToken(body1, "register")
	if tok == "" {
		t.Fatalf("no register action form in portal: %s", body1)
	}
	rr2, body2 := portalPost(t, h, tok, "register")
	if rr2.Code != http.StatusOK {
		t.Fatalf("portal register status = %d body=%s", rr2.Code, body2)
	}
	return body2
}

func TestAuthCallback_NewUser_PortalShowsRegisterOnly(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-42")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)

	if err := d.InsertOAuthState(t.Context(), "s", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	rr, body := followCallback(t, h, "/auth/callback?code=code-1&state=s")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(body, "alice") {
		t.Errorf("portal missing proposed name; body=%q", body)
	}
	if !strings.Contains(body, "未登録") {
		t.Errorf("portal should say not registered; body=%q", body)
	}
	if extractActionToken(body, "register") == "" {
		t.Error("portal should offer register action")
	}
	if extractActionToken(body, "unregister") != "" {
		t.Error("portal should NOT offer unregister for a new user")
	}
	// And the user MUST NOT be registered until /auth/portal is hit.
	if _, err := d.GetUserByDiscordID(t.Context(), "discord-42"); err == nil {
		t.Error("user row created before portal click; commit must be deferred")
	}
}

func TestAuthPortal_Register_CommitsAfterClick(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-42")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)

	body := driveRegister(t, h, d, provider, "discord-42", "alice")
	if !strings.Contains(body, "jwt-token") || !strings.Contains(body, "登録完了") {
		t.Errorf("final body missing token block / heading; body=%s", body)
	}
	user, err := d.GetUserByDiscordID(t.Context(), "discord-42")
	if err != nil {
		t.Fatalf("GetUserByDiscordID: %v", err)
	}
	if user.DisplayName != "alice" {
		t.Errorf("display_name = %q, want alice", user.DisplayName)
	}
}

func TestAuthCallback_RegisteredSameName_PortalShowsReissue(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-42")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)

	driveRegister(t, h, d, provider, "discord-42", "alice")

	// Re-enter portal with the same name.
	provider.CodeToUser["c-mt"] = &oauth.User{ID: "discord-42"}
	if err := d.InsertOAuthState(t.Context(), "mt", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	rr, body := followCallback(t, h, "/auth/callback?code=c-mt&state=mt")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	// Both Reissue and Unregister must be offered.
	if !strings.Contains(body, "トークンを再発行") {
		t.Errorf("portal should offer Reissue; body=%q", body)
	}
	if extractActionToken(body, "unregister") == "" {
		t.Error("portal should offer Unregister for a registered user")
	}
}

func TestAuthCallback_RegisteredOtherName_PortalShowsRenamePreview(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-42")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)

	driveRegister(t, h, d, provider, "discord-42", "alice")

	provider.CodeToUser["c-rn"] = &oauth.User{ID: "discord-42"}
	if err := d.InsertOAuthState(t.Context(), "rn", "alice2", time.Hour); err != nil {
		t.Fatal(err)
	}
	rr, body := followCallback(t, h, "/auth/callback?code=c-rn&state=rn")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(body, "alice") || !strings.Contains(body, "alice2") {
		t.Errorf("portal should show both old and new names; body=%q", body)
	}
	if !strings.Contains(body, "ユーザー名を変更") {
		t.Errorf("portal should describe the name-change action; body=%q", body)
	}
}

func TestAuthCallback_NameTakenByOther_PortalSuppressesRegister(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-1")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)

	// discord-1 takes "alice" first.
	driveRegister(t, h, d, provider, "discord-1", "alice")

	// discord-other arrives, also wants "alice".
	provider.CodeToUser["c-other"] = &oauth.User{ID: "discord-other"}
	if err := d.InsertOAuthState(t.Context(), "other", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	rr, body := followCallback(t, h, "/auth/callback?code=c-other&state=other")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if !strings.Contains(body, "別のDiscordアカウントで登録済みです") {
		t.Errorf("portal should warn about name conflict; body=%q", body)
	}
	if extractActionToken(body, "register") != "" {
		t.Error("portal should NOT offer register when name is taken by another")
	}
	// discord-other isn't registered, so unregister must also be hidden.
	if extractActionToken(body, "unregister") != "" {
		t.Error("portal should not offer unregister for an unregistered user")
	}
}

func TestAuthCallback_UnknownState(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "c", "d")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)
	rr, _ := get(t, h, "/auth/callback?code=c&state=missing")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestAuthCallback_ExpiredState(t *testing.T) {
	regSvc, d, fc, _ := newRegSvc(t)
	if err := d.InsertOAuthState(t.Context(), "old", "alice", time.Minute); err != nil {
		t.Fatal(err)
	}
	fc.Advance(2 * time.Minute)
	provider := oauth.NewFake("https://app/auth/callback", "c", "d")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)
	rr, _ := get(t, h, "/auth/callback?code=c&state=old")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestAuthCallback_BannedRejected(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	if err := d.BanDiscordID(t.Context(), "discord-banned", "test"); err != nil {
		t.Fatal(err)
	}
	if err := d.InsertOAuthState(t.Context(), "s", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	provider := oauth.NewFake("https://app/auth/callback", "c", "discord-banned")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)
	rr, _ := get(t, h, "/auth/callback?code=c&state=s")
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestAuthPortal_BannedBetweenStepsRejected(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-1")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)

	if err := d.InsertOAuthState(t.Context(), "s", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	_, body := followCallback(t, h, "/auth/callback?code=code-1&state=s")
	tok := extractActionToken(body, "register")
	if tok == "" {
		t.Fatal("no register form")
	}

	// admin bans the account before the portal click
	if err := d.BanDiscordID(t.Context(), "discord-1", "test"); err != nil {
		t.Fatal(err)
	}

	rr, _ := portalPost(t, h, tok, "register")
	if rr.Code != http.StatusForbidden {
		t.Errorf("portal status = %d, want 403", rr.Code)
	}
	if _, err := d.GetUserByDiscordID(t.Context(), "discord-1"); err == nil {
		t.Error("user row should not exist; ban-after-callback must block commit")
	}
}

func TestAuthPortal_UnknownToken_View(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, oauth.NewFake("https://x/cb", "c", "d"), regSvc)
	rr, _ := get(t, h, "/auth/portal?token=missing")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestAuthPortal_UnknownToken_Commit(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, oauth.NewFake("https://x/cb", "c", "d"), regSvc)
	rr, _ := portalPost(t, h, "missing", "register")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestAuthPortal_SingleUseSession(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-1")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)

	if err := d.InsertOAuthState(t.Context(), "s", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	_, body := followCallback(t, h, "/auth/callback?code=code-1&state=s")
	tok := extractActionToken(body, "register")

	if rr, _ := portalPost(t, h, tok, "register"); rr.Code != http.StatusOK {
		t.Fatalf("first portal status = %d", rr.Code)
	}
	rr2, _ := portalPost(t, h, tok, "register")
	if rr2.Code != http.StatusBadRequest {
		t.Errorf("second portal status = %d, want 400 (session is single-use)", rr2.Code)
	}
}

// TestAuthPortal_ViewIsIdempotent guards the primary UX win: the portal
// view URL must survive browser reloads (and back-button navigation)
// without consuming the session.
func TestAuthPortal_ViewIsIdempotent(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-1")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)

	if err := d.InsertOAuthState(t.Context(), "s", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	rr1, _ := get(t, h, "/auth/callback?code=code-1&state=s")
	if rr1.Code != http.StatusSeeOther {
		t.Fatalf("callback status = %d, want 303", rr1.Code)
	}
	portalURL := rr1.Header().Get("Location")

	for i := 0; i < 3; i++ {
		rr, body := get(t, h, portalURL)
		if rr.Code != http.StatusOK {
			t.Fatalf("portal view #%d status = %d body=%q", i, rr.Code, body)
		}
		if extractActionToken(body, "register") == "" {
			t.Errorf("portal view #%d missing register form", i)
		}
	}
}

func TestAuthPortal_Unregister_Commits(t *testing.T) {
	regSvc, d, _, _ := newRegSvc(t)
	provider := oauth.NewFake("https://app/auth/callback", "code-1", "discord-1")
	h := newServerFull(&fakeSaveStore{}, d, &fakeJWT{}, fakeIDGen{}, provider, regSvc)

	driveRegister(t, h, d, provider, "discord-1", "alice")
	user, _ := d.GetUserByDiscordID(t.Context(), "discord-1")
	originalJTI := user.CurrentJTI
	if originalJTI == "" {
		t.Fatal("setup failed: no current_jti")
	}

	provider.CodeToUser["c-uns"] = &oauth.User{ID: "discord-1"}
	if err := d.InsertOAuthState(t.Context(), "uns", "alice", time.Hour); err != nil {
		t.Fatal(err)
	}
	_, body := followCallback(t, h, "/auth/callback?code=c-uns&state=uns")
	tok := extractActionToken(body, "unregister")
	if tok == "" {
		t.Fatalf("no unregister form; body=%q", body)
	}
	// JTI must NOT yet be blacklisted (portal display only).
	if bl, _ := d.IsJTIBlacklisted(t.Context(), originalJTI); bl {
		t.Error("jti was blacklisted before /auth/portal — destructive action leaked")
	}

	if rr, _ := portalPost(t, h, tok, "unregister"); rr.Code != http.StatusOK {
		t.Fatalf("portal unregister status = %d", rr.Code)
	}
	if bl, _ := d.IsJTIBlacklisted(t.Context(), originalJTI); !bl {
		t.Error("expected current jti to be blacklisted after portal unregister click")
	}
}
