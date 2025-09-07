package httpx

import (
	"encoding/json"
	"net/http"

	"paymatch/internal/config"
	"paymatch/internal/http/handlers"
	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/provider"
	"paymatch/internal/services/data"
	"paymatch/internal/services/event"
	"paymatch/internal/services/tenant"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// RouterDependencies holds all dependencies for the HTTP router
type RouterDependencies struct {
	Config           config.Cfg
	TenantService    *tenant.Service
	DataService      *data.Service
	EventService     *event.ReplayService
	EventProcessor   *event.Processor
	ProviderRegistry *provider.Registry
}

// NewRouter creates the HTTP router with pure architecture services
func NewRouter(deps RouterDependencies) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	// Health check (public)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":       "ok",
			"architecture": "pure",
			"message":      "PayMatch API running with clean architecture",
		})
	})

	// Admin routes (protected by admin auth)
	r.Route("/admin", func(r chi.Router) {
		r.Use(middlewarex.AdminAuth(deps.Config))
		
		// Tenant onboarding
		r.Post("/onboard", handlers.OnboardTenant(deps.TenantService, deps.Config))
		
		// Event replay for debugging/recovery
		r.Post("/events/replay", handlers.ReplayEvents(deps.EventService))
	})

	// API routes (protected by API key auth)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middlewarex.APIKeyAuth(deps.TenantService))
		
		// Data listing endpoints
		r.Get("/payments", handlers.ListPayments(deps.DataService))
		r.Get("/events", handlers.ListEvents(deps.DataService))
		
		// Provider payment operations (if registry is available)
		if deps.ProviderRegistry != nil {
			r.Post("/payments/stk", handlers.STKPush(deps.ProviderRegistry))
			r.Post("/payments/b2c", handlers.B2C(deps.ProviderRegistry))
		}
	})

	// Webhook endpoints (public, but validated by provider)
	r.Route("/webhooks", func(r chi.Router) {
		// Webhook by shortcode - provider-specific
		r.Post("/{shortcode}", handlers.WebhookByShortcode(
			deps.TenantService,
			deps.EventProcessor,
			deps.ProviderRegistry,
		))
	})

	return r
}