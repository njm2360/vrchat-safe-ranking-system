package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func (s *Server) handleLoad(w http.ResponseWriter, r *http.Request) {
	jwtStr := strings.TrimSpace(r.URL.Query().Get("jwt"))

	if jwtStr == "" {
		writePlain(w, http.StatusUnauthorized, "missing jwt")
		return
	}
	claims, err := s.jwt.Verify(jwtStr)
	if err != nil {
		writePlain(w, http.StatusUnauthorized, "jwt invalid")
		return
	}

	entry, err := s.saves.GetLatestSave(r.Context(), claims.DisplayName)
	if err != nil {
		if errors.Is(err, db.ErrSaveNotFound) {
			writePlain(w, http.StatusNotFound, "")
			return
		}
		s.log.Error("get latest save", "err", err)
		writePlain(w, http.StatusInternalServerError, "internal error")
		return
	}

	sig := auth.SignHex(s.cfg.HMACLoadSecret, auth.LoadSigMessage(entry.Score))
	writeJSON(w, http.StatusOK, fmt.Sprintf(`{"score":%d,"sig":%q}`, entry.Score, sig))
}
