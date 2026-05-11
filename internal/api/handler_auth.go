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
	proposedName := strings.TrimSpace(c.QueryParam("name"))
	if !validDisplayName(proposedName) {
		return s.renderError(c, http.StatusBadRequest, "名前が指定されていないか、使用できない文字が含まれています。")
	}

	mockDiscordID, mockUsername := "", ""
	if s.cfg.MockOAuth {
		mockDiscordID = strings.TrimSpace(c.QueryParam("fake_discord_id"))
		if mockDiscordID == "" {
			id, err := newMockDiscordID()
			if err != nil {
				s.log.Error("mock discord_id", "err", err)
				return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
			}
			mockDiscordID = id
		}
		if !validMockDiscordID(mockDiscordID) {
			return c.String(http.StatusBadRequest,
				"mock OAuth: fake_discord_id must be numeric (Discord snowflake).")
		}
		mockUsername = strings.TrimSpace(c.QueryParam("fake_username"))
		if mockUsername == "" {
			mockUsername = proposedName
		}
		if !validMockUsername(mockUsername) {
			return c.String(http.StatusBadRequest,
				"mock OAuth: fake_username must be 1-64 chars and contain no '|' or control chars.")
		}
	}

	nameBanned, err := s.authDB.IsDisplayNameBanned(c.Request().Context(), proposedName)
	if err != nil {
		s.log.Error("display name ban check", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	}
	if nameBanned {
		return s.renderError(c, http.StatusForbidden, "このユーザー名は使用できません。")
	}

	state, err := newRandomToken()
	if err != nil {
		s.log.Error("state token", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	}
	if err := s.authDB.InsertOAuthState(c.Request().Context(), state, proposedName, s.cfg.OAuthStateTTL); err != nil {
		s.log.Error("insert oauth state", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
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
		return s.renderError(c, http.StatusBadRequest, "セッションが無効です。最初からやり直してください。")
	}

	stateRow, err := s.authDB.ConsumeOAuthState(ctx, state)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrOAuthStateNotFound):
			return s.renderError(c, http.StatusBadRequest, "セッションが無効です。最初からやり直してください。")
		case errors.Is(err, db.ErrOAuthStateExpired):
			return s.renderError(c, http.StatusBadRequest, "セッションが期限切れです。最初からやり直してください。")
		default:
			s.log.Error("consume oauth state", "err", err)
			return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
		}
	}

	user, err := s.provider.Exchange(ctx, code)
	if err != nil {
		s.log.Error("oauth exchange", "err", err)
		if errors.Is(err, oauth.ErrRateLimited) {
			return s.renderError(c, http.StatusTooManyRequests, "レート制限に達しました。しばらく待ってから再試行してください。")
		}
		return s.renderError(c, http.StatusBadGateway, "Discord認証に失敗しました。")
	}

	banned, err := s.authDB.IsDiscordIDBanned(ctx, user.ID)
	if err != nil {
		s.log.Error("ban check", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	}
	if banned {
		return s.renderError(c, http.StatusForbidden, "このDiscordアカウントは使用できません。")
	}

	sessionToken, err := newRandomToken()
	if err != nil {
		s.log.Error("session token", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	}
	if err := s.authDB.InsertAuthSession(ctx, sessionToken, user.ID, user.Username, stateRow.ProposedName, s.cfg.SessionTTL); err != nil {
		s.log.Error("insert auth session", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	}

	s.setPortalSessionCookie(c, sessionToken)
	return c.Redirect(http.StatusSeeOther, "/auth/portal")
}

func (s *Server) handleAuthPortalView(c echo.Context) error {
	ctx := c.Request().Context()
	token := readPortalSessionCookie(c)
	if token == "" {
		return s.renderError(c, http.StatusBadRequest, "セッションが無効です。最初からやり直してください。")
	}
	session, err := s.authDB.GetAuthSession(ctx, token)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrAuthSessionNotFound):
			s.clearPortalSessionCookie(c)
			return s.renderError(c, http.StatusBadRequest,
				"セッションが無効です。最初からやり直してください。")
		case errors.Is(err, db.ErrAuthSessionExpired):
			s.clearPortalSessionCookie(c)
			return s.renderError(c, http.StatusBadRequest,
				"セッションが期限切れです。最初からやり直してください。")
		default:
			s.log.Error("get auth session", "err", err)
			return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
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
		_ = s.renderError(c, http.StatusBadRequest, "セッションが無効です。最初からやり直してください。")
		return nil, db.ErrAuthSessionNotFound
	}
	session, err := s.authDB.ConsumeAuthSession(c.Request().Context(), token)
	// The cookie is single-use: clear it regardless of consume outcome so
	// the browser can't keep replaying a stale token.
	s.clearPortalSessionCookie(c)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrAuthSessionNotFound):
			_ = s.renderError(c, http.StatusBadRequest,
				"セッションが無効です。最初からやり直してください。")
		case errors.Is(err, db.ErrAuthSessionExpired):
			_ = s.renderError(c, http.StatusBadRequest,
				"セッションが期限切れです。最初からやり直してください。")
		default:
			s.log.Error("consume auth session", "err", err)
			_ = s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
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
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	} else if banned {
		return s.renderError(c, http.StatusForbidden, "このDiscordアカウントは使用できません。")
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
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	} else if banned {
		return s.renderError(c, http.StatusForbidden, "このDiscordアカウントは使用できません。")
	}
	return s.commitUnregister(c, session.DiscordID)
}

func (s *Server) commitRegister(c echo.Context, discordID, displayName string) error {
	res, err := s.regSvc.Register(c.Request().Context(), discordID, displayName)
	if err != nil {
		switch {
		case errors.Is(err, registration.ErrBanned):
			return s.renderError(c, http.StatusForbidden, "このDiscordアカウントは使用できません。")
		case errors.Is(err, registration.ErrDisplayNameBanned):
			return s.renderError(c, http.StatusForbidden, "このユーザー名は使用できません。")
		case errors.Is(err, registration.ErrDisplayNameTaken):
			return s.renderError(c, http.StatusConflict,
				"このVRChatユーザー名は別のDiscordアカウントで登録されています。最初からやり直してください。")
		default:
			s.log.Error("register", "err", err)
			return s.renderError(c, http.StatusInternalServerError, "登録に失敗しました。")
		}
	}
	heading := "登録完了"
	if res.IsRenewal {
		heading = "トークンを再発行しました（旧トークンは無効化されました）"
	}
	return s.renderToken(c, heading, res.DisplayName, res.JWT)
}

func (s *Server) commitUnregister(c echo.Context, discordID string) error {
	if err := s.authDB.Unregister(c.Request().Context(), discordID); err != nil {
		if errors.Is(err, db.ErrUserNotFound) {
			return s.renderError(c, http.StatusNotFound, "このアカウントは登録されていません。")
		}
		s.log.Error("unregister", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "登録解除に失敗しました。")
	}
	return s.renderMessage(c, "登録解除完了", "ランキングから削除されました。")
}
