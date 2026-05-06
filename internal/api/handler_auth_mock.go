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

// handleAuthMockLogin stands in for the Discord authorize page in mock
// mode. It immediately redirects the browser to /auth/callback, smuggling
// the (discord_id, username) pair through the OAuth `code` field as
// "<id>|<username>" — oauth.Fake (EchoCode=true) decodes this on the
// other side. Only registered when Config.MockOAuth is true.
func (s *Server) handleAuthMockLogin(c echo.Context) error {
	state := strings.TrimSpace(c.QueryParam("state"))
	discordID := strings.TrimSpace(c.QueryParam("discord_id"))
	username := strings.TrimSpace(c.QueryParam("username"))
	if state == "" || !validMockDiscordID(discordID) || !validMockUsername(username) {
		return c.String(http.StatusBadRequest, "mock-login: missing or invalid state/discord_id/username")
	}
	cb := "/auth/callback?" + url.Values{
		"code":  {discordID + "|" + username},
		"state": {state},
	}.Encode()
	return c.Redirect(http.StatusFound, cb)
}
