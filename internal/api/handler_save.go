package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

func (s *Server) handleSave(c echo.Context) error {
	dataStr := c.QueryParam("data")
	displayName := c.QueryParam("display_name")
	sigHex := strings.TrimSpace(c.QueryParam("sig"))

	if dataStr == "" {
		return c.String(http.StatusBadRequest, "missing data")
	}
	if displayName == "" {
		return c.String(http.StatusBadRequest, "missing display_name")
	}
	if sigHex == "" {
		return c.String(http.StatusBadRequest, "missing sig")
	}
	if !auth.VerifyHex(s.cfg.HMACSaveSecret, sigHex, []byte(dataStr), []byte(displayName)) {
		return c.String(http.StatusBadRequest, "invalid sig")
	}
	claims := claimsFromEcho(c)
	if displayName != claims.DisplayName {
		return c.String(http.StatusUnauthorized, "display_name mismatch")
	}

	data, err := savedata.Unmarshal([]byte(dataStr))
	if err != nil {
		return c.String(http.StatusBadRequest, "invalid data json")
	}
	if data.GeneratedAt == 0 {
		return c.String(http.StatusBadRequest, "missing generated_at")
	}
	if err := s.saves.Save(c.Request().Context(), claims.DisplayName, data, claims.JTI); err != nil {
		if errors.Is(err, db.ErrDuplicateSave) {
			return c.String(http.StatusConflict, "duplicate save")
		}
		s.log.Error("save", "err", err)
		return c.String(http.StatusInternalServerError, "internal error")
	}
	return c.String(http.StatusOK, "success")
}
