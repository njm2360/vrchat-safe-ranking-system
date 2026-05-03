package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SignHex returns HMAC-SHA256(key, msg) as a lowercase hex string.
func SignHex(key, msg []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyHex compares the expected hex signature against the computed signature
// in constant time. Returns false on any error or mismatch.
func VerifyHex(key, msg []byte, gotHex string) bool {
	want, err := hex.DecodeString(gotHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return hmac.Equal(want, mac.Sum(nil))
}
