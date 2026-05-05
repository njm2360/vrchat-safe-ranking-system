// Package api implements the HTTP endpoints called from VRChat Udon clients.
//
// The Server depends on small interfaces (defined here, in the consumer
// package) so tests can substitute fakes for the database, JWT verifier and
// ID generator.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
)

// TicketStore is the subset of *db.DB the challenge handler needs.
type TicketStore interface {
	InsertTicket(ctx context.Context, uuid, displayName string, ttl time.Duration) error
	CheckChallengeRate(ctx context.Context, displayName string, window time.Duration) (last time.Time, allowed bool, err error)
	UpsertChallengeRate(ctx context.Context, displayName string) error
}

// SaveStore is the subset of *db.DB the save/load/ranking handlers need.
type SaveStore interface {
	Save(ctx context.Context, displayName string, score int64, jti string) (historyID int64, err error)
	GetLatestSave(ctx context.Context, displayName string) (*db.SaveEntry, error)
	Ranking(ctx context.Context, limit int) ([]db.RankingRow, error)
	IsJTIBlacklisted(ctx context.Context, jti string) (bool, error)
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
	HMACSaveSecret   []byte
	HMACLoadSecret   []byte
	TicketTTL        time.Duration
	ChallengeRateTTL time.Duration
}

type Server struct {
	cfg     Config
	tickets TicketStore
	saves   SaveStore
	jwt     JWTVerifier
	idgen   IDGen
	log     *slog.Logger
}

func New(cfg Config, tickets TicketStore, saves SaveStore, jwt JWTVerifier, idgen IDGen, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{cfg: cfg, tickets: tickets, saves: saves, jwt: jwt, idgen: idgen, log: log}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /challenge", s.handleChallenge)
	mux.HandleFunc("GET /save", s.handleSave)
	mux.HandleFunc("GET /load", s.handleLoad)
	mux.HandleFunc("GET /ranking", s.handleRanking)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	return s.logMiddleware(mux)
}

func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		s.log.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", rw.status,
			"elapsed_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(body))
}
