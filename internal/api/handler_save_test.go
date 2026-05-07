package api_test

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

// saveURL builds /save?data=...&display_name=...&jwt=...&sig=...; sigOverride bypasses the
// computed sig (for negative tests).
func saveURL(score, generatedAt int64, displayName, jwt, sigOverride string) string {
	body, err := savedata.Marshal(&savedata.Data{Score: score, GeneratedAt: generatedAt})
	if err != nil {
		panic(err)
	}
	q := url.Values{}
	q.Set("data", string(body))
	if displayName != "" {
		q.Set("display_name", displayName)
	}
	if jwt != "" {
		q.Set("jwt", jwt)
	}
	if sigOverride != "" {
		q.Set("sig", sigOverride)
	} else {
		q.Set("sig", auth.SignHex([]byte("save-secret"), body, []byte(displayName)))
	}
	return "/save?" + q.Encode()
}

func TestSave_Ranked(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{claims: &auth.Claims{DiscordID: "d", DisplayName: "alice", JTI: "jti-1"}}
	h := newServer(saves, jwt, fakeIDGen{})

	rr, body := get(t, h, saveURL(1234, 1000, "alice", "any.jwt.value", ""))
	if rr.Code != http.StatusOK || body != "success" {
		t.Errorf("status=%d body=%q, want 200 'success'", rr.Code, body)
	}
	if len(saves.saveCalls) != 1 || saves.saveCalls[0].JTI != "jti-1" || saves.saveCalls[0].DisplayName != "alice" {
		t.Errorf("save call = %+v", saves.saveCalls)
	}
	if saves.saveCalls[0].Data == nil || saves.saveCalls[0].Data.Score != 1234 {
		t.Errorf("save call data = %+v, want Score=1234", saves.saveCalls[0].Data)
	}
}

func TestSave_MissingJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{}
	h := newServer(saves, &fakeJWT{}, fakeIDGen{})

	rr, _ := get(t, h, saveURL(100, 0, "alice", "", ""))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when jwt is missing")
	}
}

func TestSave_InvalidJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{err: errors.New("bad")}
	h := newServer(saves, jwt, fakeIDGen{})

	rr, _ := get(t, h, saveURL(1, 0, "alice", "tok", ""))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when jwt is invalid")
	}
}

func TestSave_BlacklistedJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{}
	authDB := &fakeAuthStore{jtiBlacklisted: true}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "jti-revoked"}}
	h := newServerFull(saves, authDB, jwt, fakeIDGen{}, nil, nil)

	rr, body := get(t, h, saveURL(100, 0, "alice", "any.jwt.value", ""))
	if rr.Code != http.StatusUnauthorized || body != "jwt revoked" {
		t.Errorf("status=%d body=%q, want 401 'jwt revoked'", rr.Code, body)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when jti is blacklisted")
	}
}

func TestSave_RejectsBadHMAC(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwt, fakeIDGen{})

	rr, body := get(t, h, saveURL(100, 0, "alice", "any.jwt.value", "deadbeef"))
	if rr.Code != http.StatusBadRequest || body != "invalid sig" {
		t.Errorf("status=%d body=%q, want 400 'invalid sig'", rr.Code, body)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when sig is invalid")
	}
}

func TestSave_RejectsMissingSig(t *testing.T) {
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeSaveStore{}, jwt, fakeIDGen{})
	rr, _ := get(t, h, "/save?data=%7B%22score%22%3A1%7D&display_name=alice&jwt=any.jwt.value")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSave_RejectsMissingData(t *testing.T) {
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeSaveStore{}, jwt, fakeIDGen{})
	rr, _ := get(t, h, "/save?sig=00&display_name=alice&jwt=any.jwt.value")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSave_RejectsMissingDisplayName(t *testing.T) {
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeSaveStore{}, jwt, fakeIDGen{})
	rr, _ := get(t, h, "/save?data=%7B%22score%22%3A1%7D&sig=00&jwt=any.jwt.value")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSave_RejectsInvalidJSON(t *testing.T) {
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(&fakeSaveStore{}, jwt, fakeIDGen{})

	bad := "not json"
	q := url.Values{}
	q.Set("data", bad)
	q.Set("display_name", "alice")
	q.Set("jwt", "tok")
	q.Set("sig", auth.SignHex([]byte("save-secret"), []byte(bad), []byte("alice")))
	rr, _ := get(t, h, "/save?"+q.Encode())
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSave_DisplayNameMismatch_Rejected(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwt, fakeIDGen{})

	// JWT says "alice" but display_name param (and sig) says "bob" — stale JWT after rename
	rr, body := get(t, h, saveURL(100, 0, "bob", "any.jwt.value", ""))
	if rr.Code != http.StatusUnauthorized || body != "display_name mismatch" {
		t.Errorf("status=%d body=%q, want 401 'display_name mismatch'", rr.Code, body)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when display_name mismatches JWT")
	}
}

func TestSave_DuplicateSave_Returns409(t *testing.T) {
	saves := &fakeSaveStore{saveErr: db.ErrDuplicateSave}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwt, fakeIDGen{})

	rr, body := get(t, h, saveURL(100, 1000, "alice", "any.jwt.value", ""))
	if rr.Code != http.StatusConflict || body != "duplicate save" {
		t.Errorf("status=%d body=%q, want 409 'duplicate save'", rr.Code, body)
	}
}

func TestSave_MissingGeneratedAt_Returns400(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwt, fakeIDGen{})

	rr, body := get(t, h, saveURL(100, 0, "alice", "any.jwt.value", ""))
	if rr.Code != http.StatusBadRequest || body != "missing generated_at" {
		t.Errorf("status=%d body=%q, want 400 'missing generated_at'", rr.Code, body)
	}
	if len(saves.saveCalls) != 0 {
		t.Error("Save should not be called when generated_at is missing")
	}
}
