package auth

import "strconv"

// SaveSigMessage builds the canonical message that must be signed for /save.
// Format: "<user_id>|<score>"
//
// This format is the contract between server, vrcclient, and any future Udon
// implementation. Do not change without updating all sides and the README.
func SaveSigMessage(userID string, score int64) []byte {
	return []byte(userID + "|" + strconv.FormatInt(score, 10))
}

// LoadSigMessage builds the canonical message for /load.
// Format: "<user_id>"
func LoadSigMessage(userID string) []byte {
	return []byte(userID)
}
