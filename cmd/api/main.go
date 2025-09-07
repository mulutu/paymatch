package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"paymatch/internal/config"
	"paymatch/internal/services/data"
	"paymatch/internal/services/event"
	"paymatch/internal/services/payment"
	"paymatch/internal/services/tenant"
	httpx "paymatch/internal/http"
	"paymatch/internal/provider"
	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"

	"github.com/rs/zerolog/log"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Info().Msg("starting PayMatch API server")

	// Initialize database
	pool := postgres.MustOpen(ctx, cfg.DB.DSN)
	defer pool.Close()
	
	// Create repository implementations
	paymentRepo := postgres.NewPaymentRepository(pool)
	eventRepo := postgres.NewEventRepository(pool)
	tenantRepo := postgres.NewTenantRepository(pool)
	credentialRepo := postgres.NewCredentialRepository(pool)
	unitOfWork := postgres.NewUnitOfWork(pool)
	
	// Create services with dependency injection
	paymentService := payment.NewService(paymentRepo, eventRepo)
	tenantService := tenant.NewService(tenantRepo, credentialRepo, cfg)
	dataService := data.NewService(paymentRepo, eventRepo)

	// Initialize provider registry with pure architecture
	providerRegistry := provider.NewProviderRegistry(cfg, credentialRepo)
	
	// Register available providers
	mpesaProvider := mpesa.New(cfg)
	providerRegistry.RegisterProvider(provider.ProviderMpesa, mpesaProvider)
	
	log.Info().
		Int("provider_count", len(providerRegistry.ListProviders())).
		Msg("provider registry initialized with all available providers")

	// Create event services
	eventProcessor := event.NewProcessor(eventRepo, paymentService, unitOfWork)
	replayService := event.NewReplayService(eventRepo, pool)

	// Start event processing worker with pure architecture
	workerConfig := event.DefaultWorkerConfig()
	eventWorker, err := event.NewEventProcessingSystem(pool, paymentService, workerConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create event processing system")
	}
	go eventWorker.Run(ctx)
	log.Info().Msg("event processing worker started (pure architecture)")

	// Create HTTP router with pure architecture
	routerDeps := httpx.RouterDependencies{
		Config:           cfg,
		TenantService:    tenantService,
		DataService:      dataService,
		EventService:     replayService,
		EventProcessor:   eventProcessor,
		ProviderRegistry: providerRegistry,
	}
	r := httpx.NewRouter(routerDeps)

	srv := &http.Server{
		Addr:         ":" + cfg.App.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		log.Info().
			Str("port", cfg.App.Port).
			Str("environment", cfg.App.Env).
			Msg("PayMatch API server listening")
		
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed to start")
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	
	log.Info().Msg("shutting down server...")
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	}
	
	log.Info().Msg("server stopped gracefully")
}
