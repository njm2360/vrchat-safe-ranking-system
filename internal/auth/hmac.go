package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func signParts(key []byte, parts [][]byte) []byte {
	mac := hmac.New(sha256.New, key)
	for i, p := range parts {
		if i > 0 {
			mac.Write([]byte{0})
		}
		mac.Write(p)
	}
	return mac.Sum(nil)
}

func SignHex(key []byte, parts ...[]byte) string {
	return hex.EncodeToString(signParts(key, parts))
}

func VerifyHex(key []byte, gotHex string, parts ...[]byte) bool {
	if len(gotHex) != sha256.Size*2 {
		return false
	}
	want, err := hex.DecodeString(gotHex)
	if err != nil {
		return false
	}
	return hmac.Equal(want, signParts(key, parts))
}
