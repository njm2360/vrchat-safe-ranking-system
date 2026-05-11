package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

func loadURL(displayName, jwt string) string {
	q := url.Values{}
	if displayName != "" {
		q.Set("display_name", displayName)
		q.Set("sig", auth.SignHex(testLoadSecret, []byte(displayName)))
	}
	if jwt != "" {
		q.Set("jwt", jwt)
	}
	return "/load?" + q.Encode()
}

func TestLoad_Success(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Data: &savedata.Data{Score: 1234, GeneratedAt: time.Unix(9999, 0).UTC()}}}
	jwtV := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwtV, fakeIDGen{})

	rr, body := get(t, h, loadURL("alice", "any.jwt.value"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q, want 200", rr.Code, body)
	}

	var resp struct {
		Data json.RawMessage `json:"data"`
		Sig  string          `json:"sig"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v (body=%q)", err, body)
	}
	if string(resp.Data) != `{"score":1234,"generated_at":"1970-01-01T02:46:39Z"}` {
		t.Errorf("data = %q, want canonical JSON", string(resp.Data))
	}
	if !auth.VerifyHex(testLoadSecret, resp.Sig, resp.Data) {
		t.Errorf("response sig does not verify against load secret over data bytes")
	}
}

func TestLoad_Anonymous_Unregistered_OK(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Data: &savedata.Data{Score: 1234, GeneratedAt: time.Unix(9999, 0).UTC()}}}
	authDB := &fakeAuthStore{jtiOwner: true, dnRegistered: false}
	h := newServerFull(saves, authDB, &fakeJWT{}, fakeIDGen{}, nil, nil)

	rr, body := get(t, h, loadURL("ghost", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q, want 200", rr.Code, body)
	}
	var resp struct {
		Data json.RawMessage `json:"data"`
		Sig  string          `json:"sig"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !auth.VerifyHex(testLoadSecret, resp.Sig, resp.Data) {
		t.Error("response sig does not verify")
	}
}

func TestLoad_Anonymous_RegisteredName_Rejected(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Data: &savedata.Data{Score: 1}}}
	authDB := &fakeAuthStore{jtiOwner: true, dnRegistered: true}
	h := newServerFull(saves, authDB, &fakeJWT{}, fakeIDGen{}, nil, nil)

	rr, body := get(t, h, loadURL("alice", ""))
	if rr.Code != http.StatusUnauthorized || body != "jwt required for this display_name" {
		t.Errorf("status=%d body=%q, want 401 'jwt required for this display_name'", rr.Code, body)
	}
}

func TestLoad_InvalidJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Data: &savedata.Data{Score: 1}}}
	jwtV := &fakeJWT{err: errors.New("bad")}
	h := newServer(saves, jwtV, fakeIDGen{})

	rr, _ := get(t, h, loadURL("alice", "bad.token"))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestLoad_BlacklistedJWT_Rejected(t *testing.T) {
	saves := &fakeSaveStore{
		latestRet: &db.SaveEntry{Data: &savedata.Data{Score: 1}},
	}
	authDB := &fakeAuthStore{jtiOwner: true, jtiBlacklisted: true}
	jwtV := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "jti-revoked"}}
	h := newServerFull(saves, authDB, jwtV, fakeIDGen{}, nil, nil)

	rr, body := get(t, h, loadURL("alice", "any.jwt.value"))
	if rr.Code != http.StatusUnauthorized || body != "jwt revoked" {
		t.Errorf("status=%d body=%q, want 401 'jwt revoked'", rr.Code, body)
	}
}

func TestLoad_NotFound(t *testing.T) {
	saves := &fakeSaveStore{latestErr: db.ErrSaveNotFound}
	jwtV := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwtV, fakeIDGen{})

	rr, _ := get(t, h, loadURL("alice", "any.jwt.value"))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestLoad_MissingDisplayName_Rejected(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Data: &savedata.Data{Score: 1}}}
	jwtV := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwtV, fakeIDGen{})

	rr, _ := get(t, h, "/load?jwt=any.jwt.value")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestLoad_RejectsBadSig(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Data: &savedata.Data{Score: 1}}}
	jwtV := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwtV, fakeIDGen{})

	q := url.Values{}
	q.Set("display_name", "alice")
	q.Set("sig", "deadbeef")
	q.Set("jwt", "any.jwt.value")
	rr, body := get(t, h, "/load?"+q.Encode())
	if rr.Code != http.StatusBadRequest || body != "invalid sig" {
		t.Errorf("status=%d body=%q, want 400 'invalid sig'", rr.Code, body)
	}
}

func TestLoad_DisplayNameMismatch_Rejected(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Data: &savedata.Data{Score: 1}}}
	jwtV := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	h := newServer(saves, jwtV, fakeIDGen{})

	// JWT says "alice" but display_name param (and sig) says "bob" — stale JWT after rename
	rr, body := get(t, h, loadURL("bob", "any.jwt.value"))
	if rr.Code != http.StatusUnauthorized || body != "display_name mismatch" {
		t.Errorf("status=%d body=%q, want 401 'display_name mismatch'", rr.Code, body)
	}
}
