package api

import (
	"net/http"
	"strings"
	"unicode"
)

const maxDisplayNameLen = 64

func validDisplayName(name string) bool {
	if name == "" || len(name) > maxDisplayNameLen {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (s *Server) handleChallenge(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if !validDisplayName(name) {
		writePlain(w, http.StatusBadRequest, "invalid name")
		return
	}

	ctx := r.Context()
	_, allowed, err := s.tickets.CheckChallengeRate(ctx, name, s.cfg.ChallengeRateTTL)
	if err != nil {
		s.log.Error("rate check", "err", err)
		writePlain(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !allowed {
		writePlain(w, http.StatusTooManyRequests, "rate limited")
		return
	}

	id := s.idgen.NewUUID()
	if err := s.tickets.InsertTicket(ctx, id, name, s.cfg.TicketTTL); err != nil {
		s.log.Error("insert ticket", "err", err)
		writePlain(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := s.tickets.UpsertChallengeRate(ctx, name); err != nil {
		s.log.Error("upsert rate", "err", err)
	}

	writePlain(w, http.StatusOK, id)
}
