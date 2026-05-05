package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func loadURL(jwt string) string {
	q := url.Values{}
	if jwt != "" {
		q.Set("jwt", jwt)
	}
	return "/load?" + q.Encode()
}

func TestLoad_Success(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1234}}
	jwtV := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeTicketStore{}, saves, jwtV, fakeIDGen{})

	rr, body := get(t, h, loadURL("any.jwt.value"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q, want 200", rr.Code, body)
	}

	var resp struct {
		Score int64  `json:"score"`
		Sig   string `json:"sig"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v (body=%q)", err, body)
	}
	if resp.Score != 1234 {
		t.Errorf("score = %d, want 1234", resp.Score)
	}
	if !auth.VerifyHex([]byte("load-secret"), auth.LoadSigMessage(resp.Score), resp.Sig) {
		t.Errorf("sig does not verify against load secret")
	}
}

func TestLoad_MissingJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1}}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, _ := get(t, h, loadURL(""))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestLoad_InvalidJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Score: 1}}
	jwtV := &fakeJWT{err: errors.New("bad")}
	h := newServer(&fakeTicketStore{}, saves, jwtV, fakeIDGen{})

	rr, _ := get(t, h, loadURL("bad.token"))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestLoad_NotFound(t *testing.T) {
	saves := &fakeSaveStore{latestErr: db.ErrSaveNotFound}
	jwtV := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeTicketStore{}, saves, jwtV, fakeIDGen{})

	rr, _ := get(t, h, loadURL("any.jwt.value"))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}
