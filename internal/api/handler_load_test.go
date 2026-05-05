package api_test

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func loadURL(name, jwt, sigOverride string) string {
	q := url.Values{}
	q.Set("user_id", name)
	if jwt != "" {
		q.Set("jwt", jwt)
	}
	if sigOverride != "" {
		q.Set("sig", sigOverride)
	} else {
		q.Set("sig", auth.SignHex([]byte("load-secret"), auth.LoadSigMessage(name)))
	}
	return "/load?" + q.Encode()
}

func TestLoad_Success(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1234}}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, body := get(t, h, loadURL("alice", "any.jwt.value", ""))
	if rr.Code != http.StatusOK || body != "1234" {
		t.Errorf("status=%d body=%q, want 200 '1234'", rr.Code, body)
	}
}

func TestLoad_MissingJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1}}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, _ := get(t, h, loadURL("alice", "", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestLoad_InvalidJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1}}
	jwt := &fakeJWT{err: errors.New("bad")}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, _ := get(t, h, loadURL("alice", "bad.token", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestLoad_JWTNameMismatch_Rejected(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1}}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "bob", JTI: "j"}}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, _ := get(t, h, loadURL("alice", "any.jwt.value", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestLoad_NotFound(t *testing.T) {
	saves := &fakeSaveStore{latestErr: db.ErrSaveNotFound}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, _ := get(t, h, loadURL("alice", "any.jwt.value", ""))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestLoad_InvalidSig(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1}}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, _ := get(t, h, loadURL("alice", "", "deadbeef"))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}
