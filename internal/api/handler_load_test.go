package api_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func loadURL(name, sigOverride string) string {
	q := url.Values{}
	q.Set("user_id", name)
	if sigOverride != "" {
		q.Set("sig", sigOverride)
	} else {
		q.Set("sig", auth.SignHex([]byte("load-secret"), auth.LoadSigMessage(name)))
	}
	return "/load?" + q.Encode()
}

func TestLoad_Success(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1234}}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, body := get(t, h, loadURL("alice", ""))
	if rr.Code != http.StatusOK || body != "1234" {
		t.Errorf("status=%d body=%q, want 200 '1234'", rr.Code, body)
	}
}

func TestLoad_NotFound(t *testing.T) {
	saves := &fakeSaveStore{latestErr: db.ErrSaveNotFound}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, _ := get(t, h, loadURL("alice", ""))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestLoad_InvalidSig(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1}}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, _ := get(t, h, loadURL("alice", "deadbeef"))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}
