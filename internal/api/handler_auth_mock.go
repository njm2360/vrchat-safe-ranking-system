package api

import (
	"crypto/rand"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
)

// validMockDiscordID gates which strings the mock flow accepts as a
// stand-in Discord ID. Real Discord user IDs are numeric snowflakes, so
// we mirror that and allow only ASCII digits.
func validMockDiscordID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// newMockDiscordID generates an 18-digit decimal string in the same shape
// as a real Discord snowflake. Used when /auth/start is called without an
// explicit fake_discord_id in mock mode.
func newMockDiscordID() (string, error) {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	out := make([]byte, 18)
	out[0] = '1' + b[0]%9 // leading digit 1-9 (avoid leading zero)
	for i := 1; i < 18; i++ {
		out[i] = '0' + b[i]%10
	}
	return string(out), nil
}

// validMockUsername accepts any value that is a valid VRChat DisplayName
// AND does not contain the '|' character (used as the (id, username)
// separator inside the OAuth `code` field in mock mode).
func validMockUsername(name string) bool {
	if !validDisplayName(name) {
		return false
	}
	return !strings.ContainsRune(name, '|')
}

// handleAuthMockLogin stands in for the Discord authorize page in mock mode.
// It renders an interactive form so the user can confirm or change the Discord
// ID before proceeding. Only registered when Config.MockOAuth is true.
func (s *Server) handleAuthMockLogin(c echo.Context) error {
	state := strings.TrimSpace(c.QueryParam("state"))
	if state == "" {
		return c.String(http.StatusBadRequest, "mock-login: missing state")
	}
	return s.renderMockLogin(c,
		state,
		strings.TrimSpace(c.QueryParam("discord_id")),
		strings.TrimSpace(c.QueryParam("username")),
	)
}

// handleAuthMockLoginPost processes the mock login form submission. It encodes
// the (discord_id, username) pair into the OAuth code field and redirects to
// /auth/callback — matching what a real Discord authorize redirect would do.
func (s *Server) handleAuthMockLoginPost(c echo.Context) error {
	state := strings.TrimSpace(c.FormValue("state"))
	discordID := strings.TrimSpace(c.FormValue("discord_id"))
	username := strings.TrimSpace(c.FormValue("username"))
	if state == "" || !validMockDiscordID(discordID) || !validMockUsername(username) {
		return c.String(http.StatusBadRequest, "mock-login: missing or invalid state/discord_id/username")
	}
	cb := "/auth/callback?" + url.Values{
		"code":  {discordID + "|" + username},
		"state": {state},
	}.Encode()
	return c.Redirect(http.StatusFound, cb)
}
