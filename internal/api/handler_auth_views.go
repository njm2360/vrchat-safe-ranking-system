package api

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
)

//go:embed templates
var templateFS embed.FS

var (
	tplToken     *template.Template
	tplMessage   *template.Template
	tplPortal    *template.Template
	tplMockLogin *template.Template
)

func init() {
	funcs := template.FuncMap{
		"exec_msg": func(code msgCode, data any) (template.HTML, error) {
			return execNamed(tplMessage, fmt.Sprintf("msg_%s", code), data)
		},
		"exec_tok_heading": func(action tokenAction, data any) (template.HTML, error) {
			return execNamed(tplToken, fmt.Sprintf("tok_heading_%s", action), data)
		},
	}
	parse := func(page string, extra ...string) *template.Template {
		files := append([]string{"templates/layout.html", "templates/" + page}, extra...)
		return template.Must(template.New("layout.html").Funcs(funcs).ParseFS(templateFS, files...))
	}
	tplPortal = parse("portal.html", "templates/shared.html")
	tplMessage = parse("message.html", "templates/shared.html")
	tplToken = parse("token.html")
	tplMockLogin = parse("mock_login.html")
}

func execNamed(t *template.Template, name string, data any) (template.HTML, error) {
	if t.Lookup(name) == nil {
		return "", fmt.Errorf("template %q not defined", name)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

type portalAction struct {
	Action      string // "register" or "unregister" — used to build the form action URL
	ButtonText  string
	Description string
	Primary     bool
}

type portalView struct {
	DiscordUsername  string // empty if the IdP didn't return a username
	ProposedName     string
	CurrentName      string // empty if not registered
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
func (s *Server) renderPortal(c echo.Context, authed *oauth.User, proposedName string) error {
	ctx := c.Request().Context()

	current, err := s.authDB.GetUserByDiscordID(ctx, authed.ID)
	if err != nil && !errors.Is(err, db.ErrUserNotFound) {
		s.log.Error("portal: lookup user", "err", err)
		return s.renderMessageCode(c, msgServerError)
	}

	nameIsBanned := false
	if banned, err := s.authDB.IsDisplayNameBanned(ctx, proposedName); err != nil {
		s.log.Error("portal: display name ban check", "err", err)
		return s.renderMessageCode(c, msgServerError)
	} else if banned {
		nameIsBanned = true
	}

	nameTakenByOther := false
	if existing, err := s.authDB.GetUserByDisplayName(ctx, proposedName); err == nil && existing.DiscordID != authed.ID {
		nameTakenByOther = true
	} else if err != nil && !errors.Is(err, db.ErrUserNotFound) {
		s.log.Error("portal: lookup name", "err", err)
		return s.renderMessageCode(c, msgServerError)
	}

	view := portalView{
		DiscordUsername: authed.Username,
		ProposedName:    proposedName,
	}

	activeUser := current != nil
	if activeUser {
		view.CurrentName = current.DisplayName
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
			ButtonText:  "トークン再発行",
			Description: "新しいトークンを発行します。現在のトークンは無効化されます。",
		}
	default:
		view.RegisterAction = portalAction{
			Action:      "register",
			ButtonText:  "ユーザー名変更",
			Description: "ユーザー名を変更します。現在のトークンは無効化されます。",
		}
	}
	if activeUser && !nameTakenByOther {
		view.UnregisterAction = portalAction{
			Action:      "unregister",
			ButtonText:  "登録解除",
			Description: "Discord連携を解除し、現在のトークンを無効化します。",
		}
	}

	var buf bytes.Buffer
	if err := tplPortal.Execute(&buf, view); err != nil {
		return err
	}
	return c.HTMLBlob(http.StatusOK, buf.Bytes())
}

func (s *Server) renderMessageCode(c echo.Context, code msgCode, displayName ...string) error {
	data := struct {
		Code        msgCode
		DisplayName string
	}{Code: code}
	if len(displayName) > 0 {
		data.DisplayName = displayName[0]
	}
	var buf bytes.Buffer
	if err := tplMessage.Execute(&buf, data); err != nil {
		return err
	}
	return c.HTMLBlob(code.status(), buf.Bytes())
}

func (s *Server) renderToken(c echo.Context, action tokenAction, displayName, jwt string) error {
	var buf bytes.Buffer
	data := struct {
		Action      tokenAction
		DisplayName string
		JWT         string
	}{action, displayName, jwt}
	if err := tplToken.Execute(&buf, data); err != nil {
		return err
	}
	return c.HTMLBlob(http.StatusOK, buf.Bytes())
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
