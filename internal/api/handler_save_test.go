package api_test

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

func saveURL(name string, score int64, jwt, sigOverride string) string {
	q := url.Values{}
	q.Set("user_id", name)
	q.Set("score", strconv.FormatInt(score, 10))
	if jwt != "" {
		q.Set("jwt", jwt)
	}
	if sigOverride != "" {
		q.Set("sig", sigOverride)
	} else {
		q.Set("sig", auth.SignHex([]byte("save-secret"), auth.SaveSigMessage(name, score)))
	}
	return "/save?" + q.Encode()
}

func TestSave_LocalOnly_NoJWT(t *testing.T) {
	saves := &fakeSaveStore{saveID: 1}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, body := get(t, h, saveURL("alice", 100, "", ""))
	if rr.Code != http.StatusOK || body != "OK saved" {
		t.Errorf("status=%d body=%q, want 200 'OK saved'", rr.Code, body)
	}
	if len(saves.saveCalls) != 1 || saves.saveCalls[0].JTI != "" {
		t.Errorf("save call = %+v", saves.saveCalls)
	}
}

func TestSave_RankedWithValidJWT(t *testing.T) {
	saves := &fakeSaveStore{saveID: 1}
	jwt := &fakeJWT{claims: &auth.Claims{DiscordID: "d", DisplayName: "alice", JTI: "jti-1"}}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, body := get(t, h, saveURL("alice", 1234, "any.jwt.value", ""))
	if rr.Code != http.StatusOK || body != "OK ranked" {
		t.Errorf("status=%d body=%q, want 200 'OK ranked'", rr.Code, body)
	}
	if len(saves.saveCalls) != 1 || saves.saveCalls[0].JTI != "jti-1" {
		t.Errorf("save call = %+v", saves.saveCalls)
	}
}

func TestSave_JWTNameMismatch_SavedNotRanked(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "bob", JTI: "j"}}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, body := get(t, h, saveURL("alice", 1, "tok", ""))
	if rr.Code != http.StatusOK || body != "OK saved (jwt name mismatch)" {
		t.Errorf("status=%d body=%q", rr.Code, body)
	}
	if saves.saveCalls[0].JTI != "" {
		t.Errorf("expected jti empty on mismatch, got %q", saves.saveCalls[0].JTI)
	}
}

func TestSave_InvalidJWT_SavedNotRanked(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{err: errors.New("bad")}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, body := get(t, h, saveURL("alice", 1, "tok", ""))
	if rr.Code != http.StatusOK || body != "OK saved (jwt invalid)" {
		t.Errorf("status=%d body=%q", rr.Code, body)
	}
}

func TestSave_RejectsBadHMAC(t *testing.T) {
	saves := &fakeSaveStore{}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, _ := get(t, h, saveURL("alice", 100, "", "deadbeef"))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when sig is invalid")
	}
}

func TestSave_RejectsMissingSig(t *testing.T) {
	h := newServer(&fakeTicketStore{}, &fakeSaveStore{}, &fakeJWT{}, fakeIDGen{})
	rr, _ := get(t, h, "/save?user_id=alice&score=1")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSave_RejectsInvalidScore(t *testing.T) {
	h := newServer(&fakeTicketStore{}, &fakeSaveStore{}, &fakeJWT{}, fakeIDGen{})
	rr, _ := get(t, h, "/save?user_id=alice&score=notanumber&sig=00")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
