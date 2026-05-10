package api

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

func (s *Server) handleRanking(c echo.Context) error {
	limit := 10
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	verifiedOnly := c.QueryParam("verified") == "true"
	rows, err := s.saves.Ranking(c.Request().Context(), limit, verifiedOnly)
	if err != nil {
		s.log.Error("ranking", "err", err)
		return c.String(http.StatusInternalServerError, "internal error")
	}
	if rows == nil {
		rows = []db.RankingRow{}
	}
	return c.JSON(http.StatusOK, rows)
}
