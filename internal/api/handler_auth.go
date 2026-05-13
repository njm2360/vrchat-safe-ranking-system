package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
)

const portalSessionCookieName = "vsrs_portal_session"

func (s *Server) setPortalSessionCookie(c echo.Context, token string) {
	c.SetCookie(&http.Cookie{
		Name:     portalSessionCookieName,
		Value:    token,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
}

func (s *Server) clearPortalSessionCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     portalSessionCookieName,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func newRandomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Server) handleAuthStart(c echo.Context) error {
	proposedName := strings.TrimSpace(c.QueryParam("display_name"))
	if !validDisplayName(proposedName) {
		return s.renderMessageCode(c, msgBadRequest)
	}

	sigHex := strings.TrimSpace(c.QueryParam("sig"))
	if sigHex == "" {
		return s.renderMessageCode(c, msgBadRequest)
	}
	_, usedPrev, ok := s.cfg.AuthKeys.Verify(sigHex, []byte(proposedName))
	if !ok {
		return s.renderMessageCode(c, msgBadRequest)
	}
	if usedPrev {
		s.log.Warn("rotation: previous key accepted", "endpoint", "auth/start", "display_name", proposedName)
	}

	mockDiscordID, mockUsername := "", ""
	if s.cfg.MockOAuth {
		mockDiscordID = strings.TrimSpace(c.QueryParam("fake_discord_id"))
		if mockDiscordID == "" {
			id, err := newMockDiscordID()
			if err != nil {
				s.log.Error("mock discord_id", "err", err)
				return s.renderMessageCode(c, msgServerError)
			}
			mockDiscordID = id
		}
		if !validMockDiscordID(mockDiscordID) {
			return s.renderMessageCode(c, msgBadRequest)
		}
		mockUsername = strings.TrimSpace(c.QueryParam("fake_username"))
		if mockUsername == "" {
			mockUsername = proposedName
		}
		if !validMockUsername(mockUsername) {
			return s.renderMessageCode(c, msgBadRequest)
		}
	}

	nameBanned, err := s.authDB.IsDisplayNameBanned(c.Request().Context(), proposedName)
	if err != nil {
		s.log.Error("display name ban check", "err", err)
		return s.renderMessageCode(c, msgServerError)
	}
	if nameBanned {
		return s.renderMessageCodeWithName(c, msgNameBanned, proposedName)
	}

	state, err := newRandomToken()
	if err != nil {
		s.log.Error("state token", "err", err)
		return s.renderMessageCode(c, msgServerError)
	}
	if err := s.authDB.InsertOAuthState(c.Request().Context(), state, proposedName, s.cfg.OAuthStateTTL); err != nil {
		s.log.Error("insert oauth state", "err", err)
		return s.renderMessageCode(c, msgServerError)
	}

	if s.cfg.MockOAuth {
		mockURL := "/auth/mock-login?" + url.Values{
			"state":      {state},
			"discord_id": {mockDiscordID},
			"username":   {mockUsername},
		}.Encode()
		return c.Redirect(http.StatusFound, mockURL)
	}
	return c.Redirect(http.StatusFound, s.provider.AuthURL(state))
}

func (s *Server) handleAuthCallback(c echo.Context) error {
	ctx := c.Request().Context()
	state := strings.TrimSpace(c.QueryParam("state"))
	code := strings.TrimSpace(c.QueryParam("code"))
	if state == "" || code == "" {
		return s.renderMessageCode(c, msgSessionInvalid)
	}

	stateRow, err := s.authDB.ConsumeOAuthState(ctx, state)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrOAuthStateNotFound):
			return s.renderMessageCode(c, msgSessionInvalid)
		case errors.Is(err, db.ErrOAuthStateExpired):
			return s.renderMessageCode(c, msgSessionExpired)
		default:
			s.log.Error("consume oauth state", "err", err)
			return s.renderMessageCode(c, msgServerError)
		}
	}

	user, err := s.provider.Exchange(ctx, code)
	if err != nil {
		s.log.Error("oauth exchange", "err", err)
		if errors.Is(err, oauth.ErrRateLimited) {
			return s.renderMessageCode(c, msgRateLimited)
		}
		return s.renderMessageCode(c, msgOAuthFailed)
	}

	banned, err := s.authDB.IsDiscordIDBanned(ctx, user.ID)
	if err != nil {
		s.log.Error("ban check", "err", err)
		return s.renderMessageCode(c, msgServerError)
	}
	if banned {
		return s.renderMessageCode(c, msgDiscordBanned)
	}

	sessionToken, err := newRandomToken()
	if err != nil {
		s.log.Error("session token", "err", err)
		return s.renderMessageCode(c, msgServerError)
	}
	if err := s.authDB.InsertAuthSession(ctx, sessionToken, user.ID, user.Username, stateRow.ProposedName, s.cfg.SessionTTL); err != nil {
		s.log.Error("insert auth session", "err", err)
		return s.renderMessageCode(c, msgServerError)
	}

	s.setPortalSessionCookie(c, sessionToken)
	return c.Redirect(http.StatusSeeOther, "/auth/portal")
}

func (s *Server) handleAuthPortalView(c echo.Context) error {
	ctx := c.Request().Context()
	token := readPortalSessionCookie(c)
	if token == "" {
		return s.renderMessageCode(c, msgSessionInvalid)
	}
	session, err := s.authDB.GetAuthSession(ctx, token)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrAuthSessionNotFound):
			s.clearPortalSessionCookie(c)
			return s.renderMessageCode(c, msgSessionInvalid)
		case errors.Is(err, db.ErrAuthSessionExpired):
			s.clearPortalSessionCookie(c)
			return s.renderMessageCode(c, msgSessionExpired)
		default:
			s.log.Error("get auth session", "err", err)
			return s.renderMessageCode(c, msgServerError)
		}
	}
	authed := &oauth.User{ID: session.DiscordID, Username: session.DiscordUsername}
	return s.renderPortal(c, authed, session.ProposedName)
}

func readPortalSessionCookie(c echo.Context) string {
	ck, err := c.Cookie(portalSessionCookieName)
	if err != nil || ck == nil {
		return ""
	}
	return strings.TrimSpace(ck.Value)
}

func (s *Server) consumePortalSession(c echo.Context) (*db.AuthSession, error) {
	token := readPortalSessionCookie(c)
	if token == "" {
		_ = s.renderMessageCode(c, msgSessionInvalid)
		return nil, db.ErrAuthSessionNotFound
	}
	session, err := s.authDB.ConsumeAuthSession(c.Request().Context(), token)
	// The cookie is single-use: clear it regardless of consume outcome so
	// the browser can't keep replaying a stale token.
	s.clearPortalSessionCookie(c)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrAuthSessionNotFound):
			_ = s.renderMessageCode(c, msgSessionInvalid)
		case errors.Is(err, db.ErrAuthSessionExpired):
			_ = s.renderMessageCode(c, msgSessionExpired)
		default:
			s.log.Error("consume auth session", "err", err)
			_ = s.renderMessageCode(c, msgServerError)
		}
		return nil, err // レスポンス書き込み済み。err は常に non-nil
	}
	return session, nil
}

func (s *Server) handleAuthRegister(c echo.Context) error {
	ctx := c.Request().Context()
	session, err := s.consumePortalSession(c)
	if err != nil {
		return err
	}
	if banned, err := s.authDB.IsDiscordIDBanned(ctx, session.DiscordID); err != nil {
		s.log.Error("register: ban check", "err", err)
		return s.renderMessageCode(c, msgServerError)
	} else if banned {
		return s.renderMessageCode(c, msgDiscordBanned)
	}
	return s.commitRegister(c, session.DiscordID, session.ProposedName)
}

func (s *Server) handleAuthUnregister(c echo.Context) error {
	ctx := c.Request().Context()
	session, err := s.consumePortalSession(c)
	if err != nil {
		return err
	}
	if banned, err := s.authDB.IsDiscordIDBanned(ctx, session.DiscordID); err != nil {
		s.log.Error("unregister: ban check", "err", err)
		return s.renderMessageCode(c, msgServerError)
	} else if banned {
		return s.renderMessageCode(c, msgDiscordBanned)
	}
	return s.commitUnregister(c, session.DiscordID, session.ProposedName)
}

func (s *Server) commitRegister(c echo.Context, discordID, displayName string) error {
	res, err := s.regSvc.Register(c.Request().Context(), discordID, displayName)
	if err != nil {
		switch {
		case errors.Is(err, registration.ErrBanned):
			return s.renderMessageCode(c, msgDiscordBanned)
		case errors.Is(err, registration.ErrDisplayNameBanned):
			return s.renderMessageCodeWithName(c, msgNameBanned, displayName)
		case errors.Is(err, registration.ErrDisplayNameTaken):
			return s.renderMessageCodeWithName(c, msgNameTaken, displayName)
		default:
			s.log.Error("register", "err", err)
			return s.renderMessageCode(c, msgRegisterFailed)
		}
	}
	action := tokenActionRegister
	if res.IsRenewal {
		if res.PrevDisplayName != res.DisplayName {
			action = tokenActionRename
		} else {
			action = tokenActionRenewal
		}
	}
	return s.renderToken(c, action, res.DisplayName, res.JWT)
}

func (s *Server) commitUnregister(c echo.Context, discordID, displayName string) error {
	if err := s.authDB.Unregister(c.Request().Context(), discordID); err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			return s.renderMessageCodeWithName(c, msgNotRegistered, displayName)
		}
		s.log.Error("unregister", "err", err)
		return s.renderMessageCode(c, msgUnregisterFailed)
	}
	return s.renderMessageCode(c, msgUnregistered)
}
