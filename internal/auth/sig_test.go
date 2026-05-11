package auth_test

import (
	"testing"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

func TestSignVerifyRoundtrip(t *testing.T) {
	key := []byte("save-secret-16by")
	msg := []byte("alice|1234")
	sig := auth.SignHex(key, msg)
	if !auth.VerifyHex(key, sig, msg) {
		t.Fatal("VerifyHex rejected its own signature")
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	key := []byte("save-secret-16by")
	other := []byte("other-secret-16b")
	cases := []struct {
		name string
		msg  []byte
		sig  string
	}{
		{"tampered msg", []byte("alice|1235"), auth.SignHex(key, []byte("alice|1234"))},
		{"wrong key", []byte("alice|1234"), auth.SignHex(other, []byte("alice|1234"))},
		{"empty sig", []byte("alice|1234"), ""},
		{"non-hex sig", []byte("alice|1234"), "zzzz"},
		{"truncated sig", []byte("alice|1234"), auth.SignHex(key, []byte("alice|1234"))[:10]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if auth.VerifyHex(key, tc.sig, tc.msg) {
				t.Errorf("Verify accepted bad input")
			}
		})
	}
}

// Reference vector from the SipHash-2-4 paper
// (Aumasson & Bernstein, "SipHash: a fast short-input PRF"):
// k = 00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f,
// m = (empty) → output bytes 31 0e 0e dd 47 db 6f 72.
// Pinning this lets the Udon-side SipHash port verify against the same input.
func TestSignHexRefVector_EmptyMessage(t *testing.T) {
	key := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}
	got := auth.SignHex(key)
	want := "310e0edd47db6f72"
	if got != want {
		t.Errorf("ref vector mismatch: got %s want %s", got, want)
	}
}

// Multi-part NUL separator: SignHex(k, "a", "b") must equal a direct
// SipHash over "a\x00b". This locks the wire format the Udon client
// has to reproduce.
func TestSignHexMultipartSeparator(t *testing.T) {
	key := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}
	multi := auth.SignHex(key, []byte("a"), []byte("b"))
	joined := auth.SignHex(key, []byte("a\x00b"))
	if multi != joined {
		t.Errorf("multi-part sig %s != joined-with-NUL sig %s", multi, joined)
	}
}
