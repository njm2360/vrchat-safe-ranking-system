package api_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

var errBoom = errors.New("boom")

func TestSave_DBError(t *testing.T) {
	saves := &fakeSaveStore{saveErr: errBoom}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwt, fakeIDGen{})
	rr, _ := get(t, h, saveURL(1, time.Now().Add(-time.Minute).UTC(), "alice", "any.jwt.value", ""))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestLoad_DBError(t *testing.T) {
	saves := &fakeSaveStore{latestErr: errBoom}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwt, fakeIDGen{})
	rr, _ := get(t, h, loadURL("alice", "any.jwt.value"))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestRanking_DBError(t *testing.T) {
	saves := &fakeSaveStore{rankingErr: errBoom}
	h := newServer(saves, &fakeJWT{}, fakeIDGen{})
	rr, _ := get(t, h, "/ranking")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}
