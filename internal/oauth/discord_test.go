package oauth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
)

func TestDiscord_AuthURL(t *testing.T) {
	p := oauth.NewDiscord(oauth.DiscordConfig{
		ClientID:     "client-x",
		ClientSecret: "secret",
		RedirectURL:  "https://example.com/auth/callback",
	})
	got := p.AuthURL("state-abc")
	for _, want := range []string{
		"response_type=code",
		"client_id=client-x",
		"redirect_uri=https%3A%2F%2Fexample.com%2Fauth%2Fcallback",
		"scope=identify",
		"state=state-abc",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("AuthURL %q missing %q", got, want)
		}
	}
}

func TestDiscord_Exchange_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "the-code" {
			t.Errorf("code = %q", r.Form.Get("code"))
		}
		if r.Form.Get("client_id") != "client-x" || r.Form.Get("client_secret") != "secret" {
			t.Error("missing client credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"AT","token_type":"Bearer"}`))
	})
	mux.HandleFunc("/users/@me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer AT" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"123456789","username":"alice.discord"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := oauth.NewDiscord(oauth.DiscordConfig{
		ClientID:     "client-x",
		ClientSecret: "secret",
		RedirectURL:  "https://example.com/auth/callback",
		TokenURL:     srv.URL + "/oauth2/token",
		UserURL:      srv.URL + "/users/@me",
	})
	u, err := p.Exchange(context.Background(), "the-code")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if u.ID != "123456789" || u.Username != "alice.discord" {
		t.Errorf("user = %+v", u)
	}
}

func TestDiscord_Exchange_TokenFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad code", http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := oauth.NewDiscord(oauth.DiscordConfig{
		ClientID: "x", ClientSecret: "y", RedirectURL: "z",
		TokenURL: srv.URL,
	})
	if _, err := p.Exchange(context.Background(), "bad"); err == nil {
		t.Fatal("expected error for non-200 token response")
	}
}

func TestDiscord_Exchange_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p := oauth.NewDiscord(oauth.DiscordConfig{
		ClientID: "x", ClientSecret: "y", RedirectURL: "z",
		TokenURL: srv.URL,
	})
	if _, err := p.Exchange(context.Background(), "code"); err == nil {
		t.Fatal("expected error for empty access_token")
	}
}

func TestFake_RoundTrip(t *testing.T) {
	f := oauth.NewFake("https://app/auth/callback", "code-1", "discord-42")
	got := f.AuthURL("state-xyz")
	if !strings.Contains(got, "code=code-1") || !strings.Contains(got, "state=state-xyz") {
		t.Errorf("AuthURL = %q", got)
	}
	u, err := f.Exchange(context.Background(), "code-1")
	if err != nil || u.ID != "discord-42" {
		t.Errorf("Exchange = (%+v, %v)", u, err)
	}
	if _, err := f.Exchange(context.Background(), "missing"); err == nil {
		t.Error("expected error for unknown code")
	}
}

func TestFakeEcho_DecodesIdAndUsername(t *testing.T) {
	f := oauth.NewFakeEcho()
	u, err := f.Exchange(context.Background(), "100|alice")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "100" || u.Username != "alice" {
		t.Errorf("user = %+v", u)
	}
	// id alone (no '|') is allowed; username is empty.
	u2, err := f.Exchange(context.Background(), "200")
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != "200" || u2.Username != "" {
		t.Errorf("user = %+v", u2)
	}
	// empty id is rejected.
	if _, err := f.Exchange(context.Background(), "|alice"); err == nil {
		t.Error("expected error for empty id")
	}
}
