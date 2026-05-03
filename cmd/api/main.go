package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/njm2360/vrchat-ranking-system/internal/api"
	"github.com/njm2360/vrchat-ranking-system/internal/auth"
	"github.com/njm2360/vrchat-ranking-system/internal/config"
	"github.com/njm2360/vrchat-ranking-system/internal/db"
	"github.com/njm2360/vrchat-ranking-system/internal/idgen"
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

	apiCfg := api.Config{
		HMACSaveSecret:   cfg.HMACSaveSecret,
		HMACLoadSecret:   cfg.HMACLoadSecret,
		TicketTTL:        cfg.TicketTTL,
		ChallengeRateTTL: cfg.ChallengeRateTTL,
	}
	srv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           api.New(apiCfg, database, database, auth.NewJWTIssuer(cfg.JWTSecret), idgen.Real{}, log).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
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
				if n, err := database.DeleteExpiredTickets(ctx); err != nil {
					log.Error("ticket cleanup", "err", err)
				} else if n > 0 {
					log.Info("ticket cleanup", "deleted", n)
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
