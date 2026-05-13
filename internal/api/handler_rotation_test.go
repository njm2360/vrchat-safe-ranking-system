package api_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

var (
	rotCurrSave = []byte("save-curr-16byte")
	rotPrevSave = []byte("save-prev-16byte")
	rotCurrLoad = []byte("load-curr-16byte")
	rotPrevLoad = []byte("load-prev-16byte")
	rotCurrAuth = []byte("auth-curr-16byte")
	rotPrevAuth = []byte("auth-prev-16byte")
)

func rotationKeys() (saveKS, loadKS, authKS auth.KeySet) {
	return auth.KeySet{Current: rotCurrSave, Previous: rotPrevSave},
		auth.KeySet{Current: rotCurrLoad, Previous: rotPrevLoad},
		auth.KeySet{Current: rotCurrAuth, Previous: rotPrevAuth}
}

func TestSave_AcceptsPreviousKeyDuringRotation(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{claims: &auth.Claims{DiscordID: "d", DisplayName: "alice", JTI: "jti-1"}}
	saveKS, loadKS, authKS := rotationKeys()
	h := newServerWithKeys(saves, jwt, saveKS, loadKS, authKS)

	body, err := savedata.Marshal(&savedata.Data{Score: 1, GeneratedAt: time.Now().Add(-time.Minute).UTC()})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	q := url.Values{}
	q.Set("data", string(body))
	q.Set("display_name", "alice")
	q.Set("jwt", "any.jwt.value")
	// Sign with the PREVIOUS key — a not-yet-updated Udon client.
	q.Set("sig", auth.SignHex(rotPrevSave, body, []byte("alice")))

	rr, respBody := get(t, h, "/save?"+q.Encode())
	if rr.Code != http.StatusOK || respBody != "success" {
		t.Fatalf("rotation save with previous key: status=%d body=%q, want 200 'success'", rr.Code, respBody)
	}
}

func TestSave_RejectsKeyOutsideKeySet(t *testing.T) {
	saves := &fakeSaveStore{}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	saveKS, loadKS, authKS := rotationKeys()
	h := newServerWithKeys(saves, jwt, saveKS, loadKS, authKS)

	body, _ := savedata.Marshal(&savedata.Data{Score: 1, GeneratedAt: time.Now().Add(-time.Minute).UTC()})
	q := url.Values{}
	q.Set("data", string(body))
	q.Set("display_name", "alice")
	q.Set("jwt", "any.jwt.value")
	q.Set("sig", auth.SignHex([]byte("attk-key-16bytes"), body, []byte("alice")))

	rr, respBody := get(t, h, "/save?"+q.Encode())
	if rr.Code != http.StatusBadRequest || respBody != "invalid sig" {
		t.Errorf("status=%d body=%q, want 400 'invalid sig'", rr.Code, respBody)
	}
}

func TestLoad_EchoesKeyVersionUsedByClient(t *testing.T) {
	saves := &fakeSaveStore{latestRet: &db.SaveEntry{Data: &savedata.Data{Score: 1, GeneratedAt: time.Unix(0, 0).UTC()}}}
	jwt := &fakeJWT{claims: &auth.Claims{DisplayName: "alice", JTI: "j"}}
	saveKS, loadKS, authKS := rotationKeys()
	h := newServerWithKeys(saves, jwt, saveKS, loadKS, authKS)

	cases := []struct {
		name      string
		signKey   []byte
		verifyKey []byte
		otherKey  []byte
	}{
		{"client on current key", rotCurrLoad, rotCurrLoad, rotPrevLoad},
		{"client on previous key", rotPrevLoad, rotPrevLoad, rotCurrLoad},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := url.Values{}
			q.Set("display_name", "alice")
			q.Set("jwt", "any.jwt.value")
			q.Set("sig", auth.SignHex(tc.signKey, []byte("alice")))

			rr, respBody := get(t, h, "/load?"+q.Encode())
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%q, want 200", rr.Code, respBody)
			}
			var resp struct {
				Data json.RawMessage `json:"data"`
				Sig  string          `json:"sig"`
			}
			if err := json.Unmarshal([]byte(respBody), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !auth.VerifyHex(tc.verifyKey, resp.Sig, resp.Data) {
				t.Errorf("response sig does not verify against the key the client used to sign the request")
			}
			// Cross-check: response must NOT verify with the other key — i.e.
			// the server isn't blindly using current and the echo is real.
			if auth.VerifyHex(tc.otherKey, resp.Sig, resp.Data) {
				t.Errorf("response sig unexpectedly verifies with the other key (echo broken)")
			}
		})
	}
}

func TestAuthStart_AcceptsPreviousKeyDuringRotation(t *testing.T) {
	saves := &fakeSaveStore{}
	saveKS, loadKS, authKS := rotationKeys()
	h := newServerWithKeys(saves, &fakeJWT{}, saveKS, loadKS, authKS)

	q := url.Values{}
	q.Set("display_name", "alice")
	q.Set("sig", auth.SignHex(rotPrevAuth, []byte("alice")))

	rr, _ := get(t, h, "/auth/start?"+q.Encode())
	// Provider isn't wired in this harness; we only assert sig check passed
	// (anything except a 400 from the sig branch).
	if rr.Code == http.StatusBadRequest {
		t.Errorf("status=%d, signature with previous key should be accepted (not 400)", rr.Code)
	}
}
