package vrcclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/vrcclient"
)

const (
	saveSecret = "save-secret-test"
	loadSecret = "load-secret-test"
)

func TestChallengeURLFormat(t *testing.T) {
	c := vrcclient.New("https://api.example.com/", []byte(saveSecret), []byte(loadSecret))
	got := c.ChallengeURL("alice")
	if got != "https://api.example.com/challenge?name=alice" {
		t.Errorf("ChallengeURL = %q", got)
	}
}

func TestSaveURLIncludesValidHMAC(t *testing.T) {
	c := vrcclient.New("https://x", []byte(saveSecret), []byte(loadSecret))
	u := c.SaveURL(vrcclient.SaveParams{Score: 1234, JWT: "tok"})

	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("score") != "1234" || q.Get("jwt") != "tok" {
		t.Errorf("params = %v", q)
	}
	if _, ok := q["user_id"]; ok {
		t.Error("user_id must not appear in save URL")
	}
	if !auth.VerifyHex([]byte(saveSecret), auth.SaveSigMessage(1234), q.Get("sig")) {
		t.Error("sig does not verify against save secret")
	}
}

func TestLoadURLContainsJWTAndNoSig(t *testing.T) {
	c := vrcclient.New("https://x", []byte(saveSecret), []byte(loadSecret))
	u := c.LoadURL(vrcclient.LoadParams{JWT: "my.jwt.token"})
	parsed, _ := url.Parse(u)
	q := parsed.Query()
	if q.Get("jwt") != "my.jwt.token" {
		t.Errorf("jwt param = %q, want 'my.jwt.token'", q.Get("jwt"))
	}
	if _, ok := q["sig"]; ok {
		t.Error("sig must not appear in load URL")
	}
	if _, ok := q["user_id"]; ok {
		t.Error("user_id must not appear in load URL")
	}
}

func TestRequestChallengeAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/challenge" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte("ticket-uuid-1"))
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	uuid, err := c.RequestChallenge(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if uuid != "ticket-uuid-1" {
		t.Errorf("uuid = %q", uuid)
	}
}

func TestSaveAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		score, _ := strconv.ParseInt(q.Get("score"), 10, 64)
		if !auth.VerifyHex([]byte(saveSecret), auth.SaveSigMessage(score), q.Get("sig")) {
			http.Error(w, "bad sig", http.StatusUnauthorized)
			return
		}
		w.Write([]byte("OK ranked"))
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	body, err := c.Save(context.Background(), vrcclient.SaveParams{Score: 1234, JWT: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(body, "OK") {
		t.Errorf("body = %q", body)
	}
}

func TestSaveReturnsErrorOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	if _, err := c.Save(context.Background(), vrcclient.SaveParams{Score: 1, JWT: "tok"}); err == nil {
		t.Fatal("expected error on 5xx")
	}
}

func TestSaveReturnsErrorOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad sig", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	if _, err := c.Save(context.Background(), vrcclient.SaveParams{Score: 1, JWT: "tok"}); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestChallengeReturnsErrorOn429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	if _, err := c.RequestChallenge(context.Background(), "alice"); err == nil {
		t.Fatal("expected error on 429")
	}
}

func TestBaseURLTrailingSlashStripped(t *testing.T) {
	c := vrcclient.New("https://api.example.com///", []byte(saveSecret), []byte(loadSecret))
	got := c.ChallengeURL("a")
	if !strings.HasPrefix(got, "https://api.example.com/challenge") {
		t.Errorf("URL = %q", got)
	}
}

func TestLoadReturnsEmptyOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusNotFound)
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	got, err := c.Load(context.Background(), vrcclient.LoadParams{JWT: "my.jwt"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty string for 404, got %q", got)
	}
}

func TestLoadAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		sig := auth.SignHex([]byte(loadSecret), auth.LoadSigMessage(9999))
		w.Write([]byte(`{"score":9999,"sig":"` + sig + `"}`))
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	got, err := c.Load(context.Background(), vrcclient.LoadParams{JWT: "tok"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != "9999" {
		t.Errorf("score = %q, want '9999'", got)
	}
}

func TestLoadRejectsInvalidSig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// MITM: returns a tampered score with the original sig (score mismatch)
		w.Header().Set("Content-Type", "application/json")
		legitimateSig := auth.SignHex([]byte(loadSecret), auth.LoadSigMessage(100))
		w.Write([]byte(`{"score":99999,"sig":"` + legitimateSig + `"}`))
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	if _, err := c.Load(context.Background(), vrcclient.LoadParams{JWT: "tok"}); err == nil {
		t.Fatal("expected error for tampered response, got nil")
	}
}
