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
	u := c.SaveURL(vrcclient.SaveParams{UserID: "alice", Score: 1234, JWT: "tok"})

	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("user_id") != "alice" || q.Get("score") != "1234" || q.Get("jwt") != "tok" {
		t.Errorf("params = %v", q)
	}
	if !auth.VerifyHex([]byte(saveSecret), auth.SaveSigMessage("alice", 1234), q.Get("sig")) {
		t.Error("sig does not verify against save secret")
	}
}

func TestSaveURLOmitsJWTWhenEmpty(t *testing.T) {
	c := vrcclient.New("https://x", []byte(saveSecret), []byte(loadSecret))
	u := c.SaveURL(vrcclient.SaveParams{UserID: "alice", Score: 1})
	parsed, _ := url.Parse(u)
	if _, ok := parsed.Query()["jwt"]; ok {
		t.Error("jwt param should not be present when empty")
	}
}

func TestLoadURLIncludesValidHMAC(t *testing.T) {
	c := vrcclient.New("https://x", []byte(saveSecret), []byte(loadSecret))
	u := c.LoadURL(vrcclient.LoadParams{UserID: "alice"})
	parsed, _ := url.Parse(u)
	q := parsed.Query()
	if !auth.VerifyHex([]byte(loadSecret), auth.LoadSigMessage("alice"), q.Get("sig")) {
		t.Error("sig does not verify against load secret")
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
		if !auth.VerifyHex([]byte(saveSecret), auth.SaveSigMessage(q.Get("user_id"), score), q.Get("sig")) {
			http.Error(w, "bad sig", http.StatusUnauthorized)
			return
		}
		w.Write([]byte("OK ranked"))
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	body, err := c.Save(context.Background(), vrcclient.SaveParams{UserID: "alice", Score: 1234, JWT: "tok"})
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
	if _, err := c.Save(context.Background(), vrcclient.SaveParams{UserID: "alice", Score: 1}); err == nil {
		t.Fatal("expected error on 5xx")
	}
}

func TestSaveReturnsErrorOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad sig", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := vrcclient.New(srv.URL, []byte(saveSecret), []byte(loadSecret))
	if _, err := c.Save(context.Background(), vrcclient.SaveParams{UserID: "alice", Score: 1}); err == nil {
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
	got, err := c.Load(context.Background(), vrcclient.LoadParams{UserID: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty string for 404, got %q", got)
	}
}
