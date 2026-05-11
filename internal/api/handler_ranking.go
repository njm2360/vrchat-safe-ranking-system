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
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			return c.String(http.StatusBadRequest, "bad request")
		}
		limit = n
	}
	verifiedOnly := false
	switch c.QueryParam("verified") {
	case "", "false":
	case "true":
		verifiedOnly = true
	default:
		return c.String(http.StatusBadRequest, "bad request")
	}
	rows, err := s.rankingCache.get(c.Request().Context(), limit, verifiedOnly)
	if err != nil {
		s.log.Error("ranking", "err", err)
		return c.String(http.StatusInternalServerError, "internal error")
	}
	if rows == nil {
		rows = []db.RankingRow{}
	}
	return c.JSON(http.StatusOK, rows)
}
