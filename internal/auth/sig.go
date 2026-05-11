package auth

import (
	"crypto/subtle"
	"encoding/hex"

	"github.com/dchest/siphash"
)

const SigKeySize = 16

const sigTagSize = 8

func signParts(key []byte, parts [][]byte) []byte {
	h := siphash.New(key)
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write(p)
	}
	return h.Sum(nil)
}

func SignHex(key []byte, parts ...[]byte) string {
	return hex.EncodeToString(signParts(key, parts))
}

func VerifyHex(key []byte, gotHex string, parts ...[]byte) bool {
	if len(gotHex) != sigTagSize*2 {
		return false
	}
	want, err := hex.DecodeString(gotHex)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(want, signParts(key, parts)) == 1
}
