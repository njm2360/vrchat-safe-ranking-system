package auth

import "strconv"

// SaveSigMessage builds the canonical message signed by the Udon client on /save.
// The HMAC proves the request originated from a client that knows the save secret.
func SaveSigMessage(score int64) []byte {
	return []byte(strconv.FormatInt(score, 10))
}

// LoadSigMessage builds the canonical message signed by the server in the /load response.
// The Udon client verifies this to detect MITM score tampering.
func LoadSigMessage(score int64) []byte {
	return []byte(strconv.FormatInt(score, 10))
}
