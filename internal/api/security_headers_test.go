package api_test

import (
	"strings"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/api"
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
)

// baseline headers must appear on every response regardless of route.
func TestSecurityHeaders_BaselineOnEveryResponse(t *testing.T) {
	h := newServer(&fakeSaveStore{}, &fakeJWT{}, fakeIDGen{})
	rr, _ := get(t, h, "/ranking")

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for k, v := range want {
		if got := rr.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestSecurityHeaders_HSTSGatedOnHTTPS(t *testing.T) {
	cfg := api.Config{
		HMACSaveSecret: []byte("save-secret"),
		HMACLoadSecret: []byte("load-secret"),
		HMACAuthSecret: []byte("auth-secret"),
		OAuthStateTTL:  5 * time.Minute,
		SessionTTL:     15 * time.Minute,
	}
	// CookieSecure=false → no HSTS (we may be on HTTP)
	h := api.New(cfg, &fakeSaveStore{}, &fakeAuthStore{}, &fakeJWT{}, fakeIDGen{}, oauth.NewFake("", "c", "d"), nil, nil).Handler()
	rr, _ := get(t, h, "/ranking")
	if v := rr.Header().Get("Strict-Transport-Security"); v != "" {
		t.Errorf("HSTS leaked over HTTP: %q", v)
	}

	// CookieSecure=true → HSTS present
	cfg.CookieSecure = true
	h = api.New(cfg, &fakeSaveStore{}, &fakeAuthStore{}, &fakeJWT{}, fakeIDGen{}, oauth.NewFake("", "c", "d"), nil, nil).Handler()
	rr, _ = get(t, h, "/ranking")
	if v := rr.Header().Get("Strict-Transport-Security"); !strings.Contains(v, "max-age=") {
		t.Errorf("HSTS missing on HTTPS: %q", v)
	}
}
