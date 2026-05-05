package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scoreStr := strings.TrimSpace(q.Get("score"))
	jwtStr := strings.TrimSpace(q.Get("jwt"))
	sigHex := strings.TrimSpace(q.Get("sig"))

	score, err := strconv.ParseInt(scoreStr, 10, 64)
	if err != nil {
		writePlain(w, http.StatusBadRequest, "invalid score")
		return
	}
	if sigHex == "" {
		writePlain(w, http.StatusBadRequest, "missing sig")
		return
	}
	if !auth.VerifyHex(s.cfg.HMACSaveSecret, auth.SaveSigMessage(score), sigHex) {
		writePlain(w, http.StatusUnauthorized, "invalid sig")
		return
	}
	if jwtStr == "" {
		writePlain(w, http.StatusUnauthorized, "missing jwt")
		return
	}
	claims, err := s.jwt.Verify(jwtStr)
	if err != nil {
		writePlain(w, http.StatusUnauthorized, "jwt invalid")
		return
	}
	blacklisted, err := s.saves.IsJTIBlacklisted(r.Context(), claims.JTI)
	if err != nil {
		s.log.Error("jti blacklist check", "err", err)
		writePlain(w, http.StatusInternalServerError, "internal error")
		return
	}
	if blacklisted {
		writePlain(w, http.StatusUnauthorized, "jwt revoked")
		return
	}

	if _, err := s.saves.Save(r.Context(), claims.DisplayName, score, claims.JTI); err != nil {
		s.log.Error("save", "err", err)
		writePlain(w, http.StatusInternalServerError, "internal error")
		return
	}
	writePlain(w, http.StatusOK, "OK ranked")
}
