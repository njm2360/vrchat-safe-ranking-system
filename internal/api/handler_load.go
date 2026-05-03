package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func (s *Server) handleLoad(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	userID := strings.TrimSpace(q.Get("user_id"))
	sigHex := strings.TrimSpace(q.Get("sig"))

	if !validDisplayName(userID) {
		writePlain(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	if sigHex == "" {
		writePlain(w, http.StatusBadRequest, "missing sig")
		return
	}
	if !auth.VerifyHex(s.cfg.HMACLoadSecret, auth.LoadSigMessage(userID), sigHex) {
		writePlain(w, http.StatusUnauthorized, "invalid sig")
		return
	}

	entry, err := s.saves.GetLatestSave(r.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrSaveNotFound) {
			writePlain(w, http.StatusNotFound, "")
			return
		}
		s.log.Error("get latest save", "err", err)
		writePlain(w, http.StatusInternalServerError, "internal error")
		return
	}
	writePlain(w, http.StatusOK, strconv.FormatInt(entry.Score, 10))
}
