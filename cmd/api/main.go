package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/api"
	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/config"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/idgen"
	"github.com/njm2360/vrchat-ranking-system/internal/oauth"
	"github.com/njm2360/vrchat-ranking-system/internal/registration"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Error("open db", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	issuer := auth.NewJWTIssuer(cfg.JWTSecret)
	regSvc := registration.New(database, issuer)
	var provider oauth.Provider
	if cfg.OAuthMode == config.OAuthModeMock {
		log.Warn("OAUTH_MODE=mock: real Discord login is bypassed; do NOT use in production")
		provider = oauth.NewFakeEcho()
	} else {
		provider = oauth.NewDiscord(oauth.DiscordConfig{
			ClientID:     cfg.DiscordClientID,
			ClientSecret: cfg.DiscordClientSecret,
			RedirectURL:  cfg.OAuthRedirectURL,
		})
	}

	apiCfg := api.Config{
		HMACSaveSecret: cfg.HMACSaveSecret,
		HMACLoadSecret: cfg.HMACLoadSecret,
		HMACAuthSecret: cfg.HMACAuthSecret,
		OAuthStateTTL:  cfg.OAuthStateTTL,
		SessionTTL:     cfg.SessionTTL,
		MockOAuth:      cfg.OAuthMode == config.OAuthModeMock,
		CookieSecure:   strings.HasPrefix(cfg.BaseURL, "https://"),
	}
	srv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           api.New(apiCfg, database, database, issuer, idgen.Real{}, provider, regSvc, log).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := database.DeleteExpiredOAuthStates(ctx); err != nil {
					log.Error("oauth state cleanup", "err", err)
				} else if n > 0 {
					log.Info("oauth state cleanup", "deleted", n)
				}
				if n, err := database.DeleteExpiredAuthSessions(ctx); err != nil {
					log.Error("auth session cleanup", "err", err)
				} else if n > 0 {
					log.Info("auth session cleanup", "deleted", n)
				}
			}
		}
	}()

	go func() {
		log.Info("api listening", "addr", cfg.APIAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}
