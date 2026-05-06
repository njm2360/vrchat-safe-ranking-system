package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

const claimsKey = "claims"

func (s *Server) requireJWT(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		jwtStr := strings.TrimSpace(c.QueryParam("jwt"))
		if jwtStr == "" {
			return c.String(http.StatusBadRequest, "missing jwt")
		}
		claims, err := s.jwt.Verify(jwtStr)
		if err != nil {
			return c.String(http.StatusUnauthorized, "jwt invalid")
		}
		blacklisted, err := s.authDB.IsJTIBlacklisted(c.Request().Context(), claims.JTI)
		if err != nil {
			s.log.Error("jti blacklist check", "err", err)
			return c.String(http.StatusInternalServerError, "internal error")
		}
		if blacklisted {
			return c.String(http.StatusUnauthorized, "jwt revoked")
		}
		c.Set(claimsKey, claims)
		return next(c)
	}
}

func claimsFromEcho(c echo.Context) *auth.Claims {
	v, _ := c.Get(claimsKey).(*auth.Claims)
	return v
}
