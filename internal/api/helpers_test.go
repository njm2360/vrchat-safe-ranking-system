package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/api"
	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
)

var (
	testSaveSecret = []byte("save-secret-16by")
	testLoadSecret = []byte("load-secret-16by")
	testAuthSecret = []byte("auth-secret-16by")
)

// authStartURL builds a signed /auth/start URL for tests. extra carries
// optional mock-mode query params (fake_discord_id, fake_username).
func authStartURL(displayName string, extra url.Values) string {
	q := url.Values{}
	q.Set("display_name", displayName)
	q.Set("sig", auth.SignHex(testAuthSecret, []byte(displayName)))
	for k, vs := range extra {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	return "/auth/start?" + q.Encode()
}

func newServer(saves api.SaveStore, jwt api.JWTVerifier, idgen api.IDGen) http.Handler {
	return newServerFull(saves, &fakeAuthStore{jtiOwner: true}, jwt, idgen, nil, nil)
}

func newServerWithKeys(saves api.SaveStore, jwt api.JWTVerifier, save, load, authKeys auth.KeySet) http.Handler {
	cfg := api.Config{
		SaveKeys:      save,
		LoadKeys:      load,
		AuthKeys:      authKeys,
		OAuthStateTTL: 5 * time.Minute,
		SessionTTL:    15 * time.Minute,
	}
	return api.New(cfg, saves, &fakeAuthStore{jtiOwner: true}, jwt, fakeIDGen{}, nil, nil, nil).Handler()
}

func newServerFull(saves api.SaveStore, authDB api.AuthStore, jwt api.JWTVerifier, idgen api.IDGen, provider oauth.Provider, regSvc *registration.Service) http.Handler {
	cfg := api.Config{
		SaveKeys:      auth.KeySet{Current: testSaveSecret},
		LoadKeys:      auth.KeySet{Current: testLoadSecret},
		AuthKeys:      auth.KeySet{Current: testAuthSecret},
		OAuthStateTTL: 5 * time.Minute,
		SessionTTL:    15 * time.Minute,
	}
	return api.New(cfg, saves, authDB, jwt, idgen, provider, regSvc, nil).Handler()
}

func newMockServer(saves api.SaveStore, authDB api.AuthStore, jwt api.JWTVerifier, idgen api.IDGen, regSvc *registration.Service) http.Handler {
	cfg := api.Config{
		SaveKeys:      auth.KeySet{Current: testSaveSecret},
		LoadKeys:      auth.KeySet{Current: testLoadSecret},
		AuthKeys:      auth.KeySet{Current: testAuthSecret},
		OAuthStateTTL: 5 * time.Minute,
		SessionTTL:    15 * time.Minute,
		MockOAuth:     true,
	}
	return api.New(cfg, saves, authDB, jwt, idgen, oauth.NewFakeEcho(), regSvc, nil).Handler()
}

func get(t *testing.T, h http.Handler, target string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	return getWithCookie(t, h, target, "")
}

func getWithCookie(t *testing.T, h http.Handler, target, sessionCookie string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if sessionCookie != "" {
		req.AddCookie(&http.Cookie{Name: "vsrs_portal_session", Value: sessionCookie})
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	return rr, string(body)
}

// postForm submits an application/x-www-form-urlencoded POST.
func postForm(t *testing.T, h http.Handler, target string, form url.Values) (*httptest.ResponseRecorder, string) {
	t.Helper()
	return postFormWithCookie(t, h, target, form, "")
}

func postFormWithCookie(t *testing.T, h http.Handler, target string, form url.Values, sessionCookie string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if sessionCookie != "" {
		req.AddCookie(&http.Cookie{Name: "vsrs_portal_session", Value: sessionCookie})
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	return rr, string(body)
}

// portalSessionCookie returns the value of the vsrs_portal_session cookie
// from a response, or "" if the cookie wasn't set or was cleared.
func portalSessionCookie(rr *httptest.ResponseRecorder) string {
	for _, ck := range rr.Result().Cookies() {
		if ck.Name != "vsrs_portal_session" {
			continue
		}
		if ck.MaxAge < 0 || ck.Value == "" {
			return ""
		}
		return ck.Value
	}
	return ""
}

// followCallback drives /auth/callback. On a 303 (the happy path now),
// it follows the Location header to the portal-view page using the
// session cookie set by the callback, and returns the portal response
// along with the cookie value. On any other status it returns the
// original response, so error paths (400/403/etc.) are still observable.
func followCallback(t *testing.T, h http.Handler, target string) (*httptest.ResponseRecorder, string, string) {
	t.Helper()
	rr, body := get(t, h, target)
	if rr.Code == http.StatusSeeOther {
		cookie := portalSessionCookie(rr)
		rr2, body2 := getWithCookie(t, h, rr.Header().Get("Location"), cookie)
		return rr2, body2, cookie
	}
	return rr, body, ""
}
