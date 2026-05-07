package api

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
)

//go:embed templates
var templateFS embed.FS

var (
	tplToken     = template.Must(template.ParseFS(templateFS, "templates/token.html"))
	tplMessage   = template.Must(template.ParseFS(templateFS, "templates/message.html"))
	tplPortal    = template.Must(template.ParseFS(templateFS, "templates/portal.html"))
	tplMockLogin = template.Must(template.ParseFS(templateFS, "templates/mock_login.html"))
)

type portalAction struct {
	Action      string // "register" or "unregister" — submitted as a hidden form field
	ButtonText  string
	Description string
	Primary     bool
}

type portalView struct {
	Token            string // session token, embedded in each action form
	DiscordUsername  string // empty if the IdP didn't return a username
	ProposedName     string
	CurrentName      string // empty if not registered
	CurrentJWT       string // empty if not registered
	NameBanned       bool   // proposedName is banned by an administrator
	NameConflict     bool   // proposedName held by another discord_id
	RegisterAction   portalAction
	UnregisterAction portalAction
}

func (v portalView) Registered() bool { return v.CurrentName != "" }

// ShowProposedName returns true only when a registered user is changing to a
// different name. New registration and token re-issue show the name in the
// status card instead, so the operations card doesn't need to repeat it.
func (v portalView) ShowProposedName() bool {
	return v.Registered() && v.ProposedName != v.CurrentName
}

// renderPortal builds the per-user portal view: shows the authenticated
// Discord identity, the current registration state (display name + active
// JWT if any), and offers context-appropriate action buttons.
func (s *Server) renderPortal(c echo.Context, sessionToken string, authed *oauth.User, proposedName string) error {
	ctx := c.Request().Context()

	current, err := s.authDB.GetUserByDiscordID(ctx, authed.ID)
	if err != nil && !errors.Is(err, db.ErrUserNotFound) {
		s.log.Error("portal: lookup user", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	}

	nameIsBanned := false
	if banned, err := s.authDB.IsDisplayNameBanned(ctx, proposedName); err != nil {
		s.log.Error("portal: display name ban check", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	} else if banned {
		nameIsBanned = true
	}

	nameTakenByOther := false
	if existing, err := s.authDB.GetUserByDisplayName(ctx, proposedName); err == nil && existing.DiscordID != authed.ID {
		nameTakenByOther = true
	} else if err != nil && !errors.Is(err, db.ErrUserNotFound) {
		s.log.Error("portal: lookup name", "err", err)
		return s.renderError(c, http.StatusInternalServerError, "サーバーエラーが発生しました。")
	}

	view := portalView{
		Token:           sessionToken,
		DiscordUsername: authed.Username,
		ProposedName:    proposedName,
	}

	// current_jti が空 = 名前予約済みだが登録解除済み。名前あり行があっても「未登録」扱いにする。
	activeUser := current != nil && current.CurrentJTI != ""
	if activeUser {
		view.CurrentName = current.DisplayName
		jwt, _, err := s.authDB.GetCurrentJWT(ctx, authed.ID)
		if err == nil {
			view.CurrentJWT = jwt
		} else if !errors.Is(err, db.ErrUserNotFound) {
			s.log.Error("portal: lookup jwt", "err", err)
		}
	}

	switch {
	case nameIsBanned:
		view.NameBanned = true
	case nameTakenByOther:
		view.NameConflict = true
	case !activeUser:
		view.RegisterAction = portalAction{
			Action:      "register",
			ButtonText:  "登録",
			Description: "このユーザー名で新規登録します。",
			Primary:     true,
		}
	case current.DisplayName == proposedName:
		view.RegisterAction = portalAction{
			Action:      "register",
			ButtonText:  "トークンを再発行",
			Description: "新しいトークンを発行します。現在のトークンは無効化されます。",
		}
	default:
		view.RegisterAction = portalAction{
			Action:      "register",
			ButtonText:  "ユーザー名を変更",
			Description: "ユーザー名を変更します。現在のトークンは無効化されます。",
		}
	}
	if activeUser && !nameTakenByOther {
		view.UnregisterAction = portalAction{
			Action:      "unregister",
			ButtonText:  "登録解除",
			Description: "ランキングから削除し、現在のトークンを無効化します。",
		}
	}

	var buf bytes.Buffer
	if err := tplPortal.Execute(&buf, view); err != nil {
		return err
	}
	return c.HTMLBlob(http.StatusOK, buf.Bytes())
}

func (s *Server) renderToken(c echo.Context, heading, displayName, jwt string) error {
	var buf bytes.Buffer
	if err := tplToken.Execute(&buf, struct{ Heading, DisplayName, JWT string }{heading, displayName, jwt}); err != nil {
		return err
	}
	return c.HTMLBlob(http.StatusOK, buf.Bytes())
}

func (s *Server) renderMessage(c echo.Context, heading, body string) error {
	var buf bytes.Buffer
	if err := tplMessage.Execute(&buf, struct{ Heading, Body string }{heading, body}); err != nil {
		return err
	}
	return c.HTMLBlob(http.StatusOK, buf.Bytes())
}

func (s *Server) renderError(c echo.Context, status int, body string) error {
	var buf bytes.Buffer
	if err := tplMessage.Execute(&buf, struct{ Heading, Body string }{"エラー", body}); err != nil {
		return err
	}
	return c.HTMLBlob(status, buf.Bytes())
}

type mockLoginView struct {
	State     string
	DiscordID string
	Username  string
}

func (s *Server) renderMockLogin(c echo.Context, state, discordID, username string) error {
	var buf bytes.Buffer
	if err := tplMockLogin.Execute(&buf, mockLoginView{state, discordID, username}); err != nil {
		return err
	}
	return c.HTMLBlob(http.StatusOK, buf.Bytes())
}
