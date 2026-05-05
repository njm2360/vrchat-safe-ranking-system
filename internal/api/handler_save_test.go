package api_test

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

func saveURL(score int64, jwt, sigOverride string) string {
	q := url.Values{}
	q.Set("score", strconv.FormatInt(score, 10))
	if jwt != "" {
		q.Set("jwt", jwt)
	}
	if sigOverride != "" {
		q.Set("sig", sigOverride)
	} else {
		q.Set("sig", auth.SignHex([]byte("save-secret"), auth.SaveSigMessage(score)))
	}
	return "/save?" + q.Encode()
}

func TestSave_Ranked(t *testing.T) {
	saves := &fakeSaveStore{saveID: 1}
	jwt := &fakeJWT{claims: &auth.Claims{DiscordID: "d", DisplayName: "alice", JTI: "jti-1"}}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, body := get(t, h, saveURL(1234, "any.jwt.value", ""))
	if rr.Code != http.StatusOK || body != "success" {
		t.Errorf("status=%d body=%q, want 200 'success'", rr.Code, body)
	}
	if len(saves.saveCalls) != 1 || saves.saveCalls[0].JTI != "jti-1" || saves.saveCalls[0].DisplayName != "alice" {
		t.Errorf("save call = %+v", saves.saveCalls)
	}
}

func TestSave_MissingJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, _ := get(t, h, saveURL(100, "", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when jwt is missing")
	}
}

func TestSave_InvalidJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{err: errors.New("bad")}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, _ := get(t, h, saveURL(1, "tok", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when jwt is invalid")
	}
}

func TestSave_BlacklistedJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{jtiBlacklisted: true}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "jti-revoked"}}
	h := newServer(&fakeTicketStore{}, saves, jwt, fakeIDGen{})

	rr, body := get(t, h, saveURL(100, "any.jwt.value", ""))
	if rr.Code != http.StatusUnauthorized || body != "jwt revoked" {
		t.Errorf("status=%d body=%q, want 401 'jwt revoked'", rr.Code, body)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when jti is blacklisted")
	}
}

func TestSave_RejectsBadHMAC(t *testing.T) {
	saves := &fakeSaveStore{}
	h := newServer(&fakeTicketStore{}, saves, &fakeJWT{}, fakeIDGen{})

	rr, _ := get(t, h, saveURL(100, "", "deadbeef"))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when sig is invalid")
	}
}

func TestSave_RejectsMissingSig(t *testing.T) {
	h := newServer(&fakeTicketStore{}, &fakeSaveStore{}, &fakeJWT{}, fakeIDGen{})
	rr, _ := get(t, h, "/save?score=1")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSave_RejectsInvalidScore(t *testing.T) {
	h := newServer(&fakeTicketStore{}, &fakeSaveStore{}, &fakeJWT{}, fakeIDGen{})
	rr, _ := get(t, h, "/save?score=notanumber&sig=00")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
