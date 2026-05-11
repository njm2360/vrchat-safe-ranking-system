package api

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/njm2360/vrchat-ranking-system/internal/auth"
)

const claimsKey = "claims"

func securityHeaders(hstsEnabled bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			if hstsEnabled {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			return next(c)
		}
	}
}

func (s *Server) optionalJWT(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		jwtStr := strings.TrimSpace(c.QueryParam("jwt"))
		if jwtStr == "" {
			return next(c)
		}
		claims, err := s.jwt.Verify(jwtStr)
		if err != nil {
			return c.String(http.StatusUnauthorized, "jwt invalid")
		}
		owner, err := s.authDB.IsJTIOwner(c.Request().Context(), claims.JTI, claims.DisplayName)
		if err != nil {
			s.log.Error("jti owner check", "err", err)
			return c.String(http.StatusInternalServerError, "internal error")
		}
		if !owner {
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
