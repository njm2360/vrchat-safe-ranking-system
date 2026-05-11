package vrcclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
	"github.com/njm2360/vrchat-ranking-system/internal/vrcclient"
)

const (
	saveSecret = "save-secret-test"
	loadSecret = "load-secret-test"
	authSecret = "auth-secret-test"
)

func TestSaveURLIncludesValidSig(t *testing.T) {
	c := vrcclient.New("https://x", []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	u, err := c.SaveURL(vrcclient.SaveParams{Data: &savedata.Data{Score: 1234, GeneratedAt: time.Unix(9999, 0).UTC()}, JWT: "tok", DisplayName: "testuser"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("jwt") != "tok" {
		t.Errorf("jwt param = %q", q.Get("jwt"))
	}
	if q.Get("data") != `{"score":1234,"generated_at":"1970-01-01T02:46:39Z"}` {
		t.Errorf("data = %q, want canonical JSON", q.Get("data"))
	}
	if q.Get("display_name") != "testuser" {
		t.Errorf("display_name = %q, want 'testuser'", q.Get("display_name"))
	}
	if !auth.VerifyHex([]byte(saveSecret), q.Get("sig"), []byte(q.Get("data")), []byte(q.Get("display_name"))) {
		t.Error("sig does not verify against save secret over data+display_name")
	}
}

func TestSaveURLNilDataRejected(t *testing.T) {
	c := vrcclient.New("https://x", []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	if _, err := c.SaveURL(vrcclient.SaveParams{JWT: "tok", DisplayName: "u"}); err == nil {
		t.Error("expected error for nil Data")
	}
}

func TestLoadURLContainsDisplayNameAndSig(t *testing.T) {
	c := vrcclient.New("https://x", []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	u := c.LoadURL(vrcclient.LoadParams{JWT: "my.jwt.token", DisplayName: "alice"})
	parsed, _ := url.Parse(u)
	q := parsed.Query()
	if q.Get("jwt") != "my.jwt.token" {
		t.Errorf("jwt param = %q", q.Get("jwt"))
	}
	if q.Get("display_name") != "alice" {
		t.Errorf("display_name = %q, want 'alice'", q.Get("display_name"))
	}
	if !auth.VerifyHex([]byte(loadSecret), q.Get("sig"), []byte("alice")) {
		t.Error("sig does not verify against load secret over display_name")
	}
}

func TestAuthStartURLIncludesValidSig(t *testing.T) {
	c := vrcclient.New("https://x", []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	u := c.AuthStartURL(vrcclient.AuthStartParams{
		DisplayName:   "alice",
		FakeDiscordID: "100000000000000001",
		FakeUsername:  "alice.dev",
	})
	parsed, _ := url.Parse(u)
	q := parsed.Query()
	if q.Get("display_name") != "alice" {
		t.Errorf("display_name = %q", q.Get("display_name"))
	}
	if !auth.VerifyHex([]byte(authSecret), q.Get("sig"), []byte("alice")) {
		t.Error("sig does not verify against auth secret over display_name")
	}
	if q.Get("fake_discord_id") != "100000000000000001" {
		t.Errorf("fake_discord_id = %q", q.Get("fake_discord_id"))
	}
	if q.Get("fake_username") != "alice.dev" {
		t.Errorf("fake_username = %q", q.Get("fake_username"))
	}
}

func TestSaveAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		dataStr := q.Get("data")
		displayName := q.Get("display_name")
		if !auth.VerifyHex([]byte(saveSecret), q.Get("sig"), []byte(dataStr), []byte(displayName)) {
			http.Error(w, "bad sig", http.StatusUnauthorized)
			return
		}
		var d savedata.Data
		if err := json.Unmarshal([]byte(dataStr), &d); err != nil {
			http.Error(w, "bad data", http.StatusBadRequest)
			return
		}
		if d.Score != 1234 {
			http.Error(w, "wrong score", http.StatusBadRequest)
			return
		}
		w.Write([]byte("success"))
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	body, err := c.Save(context.Background(), vrcclient.SaveParams{Data: &savedata.Data{Score: 1234}, JWT: "tok", DisplayName: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if body != "success" {
		t.Errorf("body = %q, want 'success'", body)
	}
}

func TestSaveReturnsErrorOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	if _, err := c.Save(context.Background(), vrcclient.SaveParams{Data: &savedata.Data{Score: 1}, JWT: "tok", DisplayName: "u"}); err == nil {
		t.Fatal("expected error on 5xx")
	}
}

func TestSaveReturnsErrorOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad sig", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	if _, err := c.Save(context.Background(), vrcclient.SaveParams{Data: &savedata.Data{Score: 1}, JWT: "tok", DisplayName: "u"}); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestBaseURLTrailingSlashStripped(t *testing.T) {
	c := vrcclient.New("https://api.example.com///", []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	got := c.LoadURL(vrcclient.LoadParams{JWT: "j", DisplayName: "u"})
	if !strings.HasPrefix(got, "https://api.example.com/load") {
		t.Errorf("URL = %q", got)
	}
}

func TestLoadReturnsNilOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusNotFound)
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	got, err := c.Load(context.Background(), vrcclient.LoadParams{JWT: "my.jwt", DisplayName: "u"})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil for 404, got %+v", got)
	}
}

func TestLoadAgainstFakeServer(t *testing.T) {
	dataBytes := []byte(`{"score":9999,"generated_at":"1970-01-01T00:00:00Z"}`)
	sig := auth.SignHex([]byte(loadSecret), dataBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":` + string(dataBytes) + `,"sig":"` + sig + `"}`))
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	got, err := c.Load(context.Background(), vrcclient.LoadParams{JWT: "tok", DisplayName: "alice"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil || got.Score != 9999 {
		t.Errorf("Load = %+v, want Score=9999", got)
	}
}

func TestLoadRejectsInvalidSig(t *testing.T) {
	// MITM: server returns a tampered data field while reusing a sig
	// that was generated for a different payload.
	legit := auth.SignHex([]byte(loadSecret), []byte(`{"score":100,"generated_at":"1970-01-01T00:00:00Z"}`))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"score":99999,"generated_at":"1970-01-01T00:00:00Z"},"sig":"` + legit + `"}`))
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret), []byte(authSecret))
	if _, err := c.Load(context.Background(), vrcclient.LoadParams{JWT: "tok", DisplayName: "alice"}); err == nil {
		t.Fatal("expected error for tampered response, got nil")
	}
}
