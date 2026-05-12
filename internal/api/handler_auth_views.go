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

type portalView struct {
	DiscordUsername string // empty if the IdP didn't return a username
	ProposedName    string
	CurrentName     string // empty if not registered
	NameBanned      bool   // proposedName is banned by an administrator
	NameConflict    bool   // proposedName held by another discord_id
	ShowRegister    bool
	ShowReissue     bool
	ShowRename      bool
	ShowUnregister  bool
}

func (v portalView) Registered() bool { return v.CurrentName != "" }

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

	userRegistered := current != nil
	if userRegistered {
		view.CurrentName = current.DisplayName
	}

	switch {
	case nameIsBanned:
		view.NameBanned = true
	case nameTakenByOther:
		view.NameConflict = true
	case !userRegistered:
		view.ShowRegister = true
	case current.DisplayName == proposedName:
		view.ShowReissue = true
	default:
		view.ShowRename = true
	}
	if userRegistered && !nameTakenByOther {
		view.ShowUnregister = true
	}

	var buf bytes.Buffer
	if err := tplPortal.Execute(&buf, view); err != nil {
		return err
	}
	return c.HTMLBlob(http.StatusOK, buf.Bytes())
}

func (s *Server) renderMessageCode(c echo.Context, code msgCode) error {
	return s.renderMessage(c, code, "")
}

func (s *Server) renderMessageCodeWithName(c echo.Context, code msgCode, displayName string) error {
	return s.renderMessage(c, code, displayName)
}

func (s *Server) renderMessage(c echo.Context, code msgCode, displayName string) error {
	var buf bytes.Buffer
	if err := tplMessage.Execute(&buf, struct {
		Code        msgCode
		DisplayName string
	}{code, displayName}); err != nil {
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
