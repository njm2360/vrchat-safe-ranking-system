package api

import "unicode"

const displayNameMaxBytes = 64

func validDisplayName(name string) bool {
	if name == "" || len(name) > displayNameMaxBytes {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
