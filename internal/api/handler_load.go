package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

func (s *Server) handleLoad(c echo.Context) error {
	displayName := c.QueryParam("display_name")
	sigHex := strings.TrimSpace(c.QueryParam("sig"))

	if displayName == "" {
		return c.String(http.StatusBadRequest, "missing display_name")
	}
	if !validDisplayName(displayName) {
		return c.String(http.StatusBadRequest, "bad request")
	}
	if sigHex == "" {
		return c.String(http.StatusBadRequest, "missing sig")
	}
	matchedKey, usedPrev, ok := s.cfg.LoadKeys.Verify(sigHex, []byte(displayName))
	if !ok {
		return c.String(http.StatusBadRequest, "invalid sig")
	}
	if usedPrev {
		s.log.Warn("rotation: previous key accepted", "endpoint", "load", "display_name", displayName)
	}

	claims := claimsFromEcho(c)

	if claims == nil {
		registered, err := s.authDB.IsDisplayNameRegistered(c.Request().Context(), displayName)
		if err != nil {
			s.log.Error("is display name registered", "err", err)
			return c.String(http.StatusInternalServerError, "internal error")
		}
		if registered {
			return c.String(http.StatusUnauthorized, "jwt required for this display_name")
		}
	} else {
		if displayName != claims.DisplayName {
			return c.String(http.StatusUnauthorized, "display_name mismatch")
		}
	}

	ctx := c.Request().Context()

	entry, err := s.saves.GetLatestSave(ctx, displayName)
	if err != nil {
		if errors.Is(err, db.ErrSaveNotFound) {
			return c.String(http.StatusNotFound, "")
		}
		s.log.Error("get latest save", "err", err)
		return c.String(http.StatusInternalServerError, "internal error")
	}

	dataBytes, err := savedata.Marshal(entry.Data)
	if err != nil {
		s.log.Error("marshal savedata", "err", err)
		return c.String(http.StatusInternalServerError, "internal error")
	}
	sig := auth.SignHex(matchedKey, dataBytes)

	return c.JSONBlob(http.StatusOK, fmt.Appendf(nil, `{"data":%s,"sig":%q}`, dataBytes, sig))
}
