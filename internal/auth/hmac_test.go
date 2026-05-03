package auth_test

import (
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

func TestHMACSignVerifyRoundtrip(t *testing.T) {
	key := []byte("save-secret")
	msg := []byte("alice|1234")
	sig := auth.SignHex(key, msg)
	if !auth.VerifyHex(key, msg, sig) {
		t.Fatal("VerifyHex rejected its own signature")
	}
}

func TestHMACVerifyRejectsTampering(t *testing.T) {
	key := []byte("k")
	cases := []struct {
		name string
		msg  []byte
		sig  string
	}{
		{"tampered msg", []byte("alice|1235"), auth.SignHex([]byte("k"), []byte("alice|1234"))},
		{"wrong key", []byte("alice|1234"), auth.SignHex([]byte("other"), []byte("alice|1234"))},
		{"empty sig", []byte("alice|1234"), ""},
		{"non-hex sig", []byte("alice|1234"), "zzzz"},
		{"truncated sig", []byte("alice|1234"), auth.SignHex([]byte("k"), []byte("alice|1234"))[:10]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if auth.VerifyHex(key, tc.msg, tc.sig) {
				t.Errorf("Verify accepted bad input")
			}
		})
	}
}

func TestSaveSigMessageFormat(t *testing.T) {
	got := string(auth.SaveSigMessage("alice", 1234))
	if got != "alice|1234" {
		t.Errorf("SaveSigMessage = %q, want %q", got, "alice|1234")
	}
}

func TestLoadSigMessageFormat(t *testing.T) {
	got := string(auth.LoadSigMessage("alice"))
	if got != "alice" {
		t.Errorf("LoadSigMessage = %q, want %q", got, "alice")
	}
}
