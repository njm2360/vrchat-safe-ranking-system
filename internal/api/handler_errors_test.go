package api_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

var errBoom = errors.New("boom")

func TestChallenge_DBErrorOnRateCheck(t *testing.T) {
	tickets := &fakeTicketStore{checkErr: errBoom}
	h := newServer(tickets, &fakeSaveStore{}, &fakeJWT{}, fakeIDGen{ID: "x"})
	rr, _ := get(t, h, "/challenge?name=alice")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestChallenge_DBErrorOnInsert(t *testing.T) {
	tickets := &fakeTicketStore{allowed: true, insertErr: errBoom}
	h := newServer(tickets, &fakeSaveStore{}, &fakeJWT{}, fakeIDGen{ID: "x"})
	rr, _ := get(t, h, "/challenge?name=alice")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestSave_DBError(t *testing.T) {
	saves := &fakeSaveStore{saveErr: errBoom}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})
	rr, _ := get(t, h, saveURL(1, "any.jwt.value", ""))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestLoad_DBError(t *testing.T) {
	saves := &fakeSaveStore{latestErr: errBoom}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})
	rr, _ := get(t, h, loadURL("any.jwt.value"))
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestRanking_DBError(t *testing.T) {
	saves := &fakeSaveStore{rankingErr: errBoom}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})
	rr, _ := get(t, h, "/ranking")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}
