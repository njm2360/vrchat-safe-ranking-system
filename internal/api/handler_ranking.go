package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func (s *Server) handleRanking(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := s.saves.Ranking(r.Context(), limit)
	if err != nil {
		s.log.Error("ranking", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if rows == nil {
		rows = []db.RankingRow{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(rows)
}
