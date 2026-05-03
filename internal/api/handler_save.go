package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	userID := strings.TrimSpace(q.Get("user_id"))
	scoreStr := strings.TrimSpace(q.Get("score"))
	jwtStr := strings.TrimSpace(q.Get("jwt"))
	sigHex := strings.TrimSpace(q.Get("sig"))

	if !validDisplayName(userID) {
		writePlain(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	score, err := strconv.ParseInt(scoreStr, 10, 64)
	if err != nil {
		writePlain(w, http.StatusBadRequest, "invalid score")
		return
	}
	if sigHex == "" {
		writePlain(w, http.StatusBadRequest, "missing sig")
		return
	}
	if !auth.VerifyHex(s.cfg.HMACSaveSecret, auth.SaveSigMessage(userID, score), sigHex) {
		writePlain(w, http.StatusUnauthorized, "invalid sig")
		return
	}

	// Anything short of a fully-valid JWT is recorded but excluded from the
	// ranking by leaving jti empty. Banned users and revoked jtis still get
	// to save — they just stay out of the ranking via the Ranking query.
	var jti, status = "", "OK saved"
	if jwtStr != "" {
		claims, err := s.jwt.Verify(jwtStr)
		switch {
		case err != nil:
			status = "OK saved (jwt invalid)"
		case claims.DisplayName != userID:
			status = "OK saved (jwt name mismatch)"
		default:
			jti = claims.JTI
			status = "OK ranked"
		}
	}

	if _, err := s.saves.Save(r.Context(), userID, score, jti); err != nil {
		s.log.Error("save", "err", err)
		writePlain(w, http.StatusInternalServerError, "internal error")
		return
	}
	writePlain(w, http.StatusOK, status)
}
