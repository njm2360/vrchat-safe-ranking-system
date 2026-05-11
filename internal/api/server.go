// Package api implements the HTTP endpoints called from VRChat Udon clients
// and the OAuth-based registration web flow.
//
// The Server depends on small interfaces so tests can substitute fakes.
package api

//go:generate oapi-codegen --config oapi-codegen.yaml ../../api/openapi.yaml

import (
	"context"
	"net/http"
	"time"
	"unicode/utf8"

	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
	"github.com/njm2360/vrchat-ranking-system/internal/savedata"
)

// SaveStore is the subset of *db.DB the save/load/ranking handlers need.
type SaveStore interface {
	Save(ctx context.Context, displayName string, data *savedata.Data, jti *string) error
	GetLatestSave(ctx context.Context, displayName string) (*db.SaveEntry, error)
	Ranking(ctx context.Context, limit int, verifiedOnly bool) ([]db.RankingRow, error)
}

// AuthStore is the subset of *db.DB the OAuth handlers need.
type AuthStore interface {
	IsJTIOwner(ctx context.Context, jti, displayName string) (bool, error)
	IsJTIBlacklisted(ctx context.Context, jti string) (bool, error)
	IsDisplayNameBanned(ctx context.Context, displayName string) (bool, error)
	InsertOAuthState(ctx context.Context, state, proposedName string, ttl time.Duration) error
	ConsumeOAuthState(ctx context.Context, state string) (*db.OAuthState, error)
	IsDiscordIDBanned(ctx context.Context, discordID string) (bool, error)
	IsDisplayNameRegistered(ctx context.Context, displayName string) (bool, error)
	GetUserByDiscordID(ctx context.Context, discordID string) (*db.User, error)
	GetUserByDisplayName(ctx context.Context, displayName string) (*db.User, error)
	Unregister(ctx context.Context, discordID string) error
	InsertAuthSession(ctx context.Context, token, discordID, discordUsername, proposedName string, ttl time.Duration) error
	GetAuthSession(ctx context.Context, token string) (*db.AuthSession, error)
	ConsumeAuthSession(ctx context.Context, token string) (*db.AuthSession, error)
}

// JWTVerifier verifies a JWT and returns its claims.
type JWTVerifier interface {
	Verify(token string) (*auth.Claims, error)
}

// IDGen produces unique IDs (UUIDs in production).
type IDGen interface {
	NewUUID() string
}

// Config carries the runtime parameters the handlers consult.
type Config struct {
	SaveSecret    []byte
	LoadSecret    []byte
	AuthSecret    []byte
	OAuthStateTTL  time.Duration
	SessionTTL     time.Duration
	MockOAuth      bool
	CookieSecure   bool
}

type Server struct {
	cfg          Config
	saves        SaveStore
	authDB       AuthStore
	jwt          JWTVerifier
	idgen        IDGen
	provider     oauth.Provider
	regSvc       *registration.Service
	log          *slog.Logger
	rankingCache *rankingCache
}

func New(cfg Config, saves SaveStore, authDB AuthStore, jwt JWTVerifier, idgen IDGen, provider oauth.Provider, regSvc *registration.Service, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		cfg:          cfg,
		saves:        saves,
		authDB:       authDB,
		jwt:          jwt,
		idgen:        idgen,
		provider:     provider,
		regSvc:       regSvc,
		log:          log,
		rankingCache: newRankingCache(saves),
	}
}

func (s *Server) Handler() http.Handler {
	e := echo.New()

	e.HideBanner = true
	e.HidePort = true

	e.HTTPErrorHandler = func(err error, c echo.Context) {
		code := http.StatusInternalServerError
		msg := "internal error"
		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
			if m, ok := he.Message.(string); ok {
				msg = m
			}
		}
		if !c.Response().Committed {
			c.String(code, msg) //nolint:errcheck
		}
	}

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogMethod:    true,
		LogURI:       true,
		LogStatus:    true,
		LogError:     true,
		LogUserAgent: true,
		LogRemoteIP:  true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			path := c.Request().URL.Path
			attrs := []slog.Attr{
				slog.String("method", v.Method),
				slog.String("path", path),
				slog.Int("status", v.Status),
				slog.String("user_agent", clampForLog(v.UserAgent, 256)),
				slog.String("remote_ip", v.RemoteIP),
			}
			if path == "/save" || path == "/load" {
				if dn := c.QueryParam("display_name"); dn != "" {
					attrs = append(attrs, slog.String("display_name", clampForLog(dn, 64)))
				}
			}
			if v.Error != nil {
				attrs = append(attrs, slog.String("error", v.Error.Error()))
			}
			s.log.LogAttrs(c.Request().Context(), slog.LevelInfo, "REQUEST", attrs...)
			return nil
		},
	}))
	e.Use(middleware.Recover())
	e.Use(securityHeaders(s.cfg.CookieSecure))

	e.GET("/save", s.handleSave, s.optionalJWT)
	e.GET("/load", s.handleLoad, s.optionalJWT)
	e.GET("/ranking", s.handleRanking)
	e.GET("/auth/start", s.handleAuthStart)
	e.GET("/auth/callback", s.handleAuthCallback)
	e.GET("/auth/portal", s.handleAuthPortalView)
	e.POST("/auth/register", s.handleAuthRegister)
	e.POST("/auth/unregister", s.handleAuthUnregister)
	if s.cfg.MockOAuth {
		e.GET("/auth/mock-login", s.handleAuthMockLogin)
		e.POST("/auth/mock-login", s.handleAuthMockLoginPost)
	}

	e.GET("/openapi.yaml", handleOpenapiSpec)
	e.GET("/swagger", handleSwaggerUI)

	return e
}

// clampForLog truncates s to at most maxBytes, cutting on a UTF-8 boundary and
// appending "…" when truncated. Used to bound log field sizes for untrusted
// client-supplied values (User-Agent, display_name) so a single request can't
// inflate log volume.
func clampForLog(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
