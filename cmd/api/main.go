package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"paymatch/internal/config"
	httpx "paymatch/internal/http"
	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"

	"paymatch/internal/core/reconcile"

	"github.com/rs/zerolog/log"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Init DB
	pool := postgres.MustOpen(ctx, cfg.DB.DSN)
	defer pool.Close()
	repo := postgres.NewRepo(pool, cfg)

	// Init providers registry (only M-Pesa for MVP)
	mp := mpesa.New(cfg, repo)

	// ✅ Start reconciliation worker
	worker := reconcile.NewWorker(repo)
	go worker.Run(ctx)

	// Router
	r := httpx.NewRouter(cfg, repo, mp)

	srv := &http.Server{
		Addr:         ":" + cfg.App.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Info().Msgf("PayMatch API listening on :%s", cfg.App.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	cancel()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	_ = srv.Shutdown(ctx2)
	log.Info().Msg("server stopped")
}
