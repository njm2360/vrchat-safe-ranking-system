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
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
)

func newServer(saves api.SaveStore, jwt api.JWTVerifier, idgen api.IDGen) http.Handler {
	return newServerFull(saves, &fakeAuthStore{jtiOwner: true}, jwt, idgen, nil, nil)
}

func newServerFull(saves api.SaveStore, authDB api.AuthStore, jwt api.JWTVerifier, idgen api.IDGen, provider oauth.Provider, regSvc *registration.Service) http.Handler {
	cfg := api.Config{
		HMACSaveSecret: []byte("save-secret"),
		HMACLoadSecret: []byte("load-secret"),
		OAuthStateTTL:  5 * time.Minute,
		SessionTTL:     15 * time.Minute,
	}
	return api.New(cfg, saves, authDB, jwt, idgen, provider, regSvc, nil).Handler()
}

func newMockServer(saves api.SaveStore, authDB api.AuthStore, jwt api.JWTVerifier, idgen api.IDGen, regSvc *registration.Service) http.Handler {
	cfg := api.Config{
		HMACSaveSecret: []byte("save-secret"),
		HMACLoadSecret: []byte("load-secret"),
		OAuthStateTTL:  5 * time.Minute,
		SessionTTL:     15 * time.Minute,
		MockOAuth:      true,
	}
	return api.New(cfg, saves, authDB, jwt, idgen, oauth.NewFakeEcho(), regSvc, nil).Handler()
}

func get(t *testing.T, h http.Handler, target string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	return rr, string(body)
}

// postForm submits an application/x-www-form-urlencoded POST.
func postForm(t *testing.T, h http.Handler, target string, form url.Values) (*httptest.ResponseRecorder, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	return rr, string(body)
}

// followCallback drives /auth/callback. On a 303 (the happy path now),
// it follows the Location header to the portal-view page and returns
// that response. On any other status it returns the original response,
// so error paths (400/403/etc.) are still observable.
func followCallback(t *testing.T, h http.Handler, target string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rr, body := get(t, h, target)
	if rr.Code == http.StatusSeeOther {
		return get(t, h, rr.Header().Get("Location"))
	}
	return rr, body
}
