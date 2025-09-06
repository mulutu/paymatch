package httpx

import (
	"net/http"

	"paymatch/internal/config"
	"paymatch/internal/http/handlers"
	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func NewRouter(cfg config.Cfg, repo *postgres.Repo, mp *mpesa.Provider) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	// Health
	r.Get("/healthz", handlers.Health)

	// Admin onboarding (guarded by X-Admin-Token)
	r.Post("/v1/admin/tenants/onboard", handlers.OnboardTenant(repo, cfg))

	// --- Public webhooks (no tenant auth) ---
	// STK + generic M-Pesa webhook routing
	r.Post("/hooks/mpesa/{shortcode}", handlers.MpesaWebhookByShortcode(repo, mp))
	r.Post("/hooks/mpesa", handlers.MpesaWebhookByToken(repo, mp))

	// C2B specific (Daraja C2B validation & confirmation)
	r.Post("/hooks/paywatch/c2b/validation", handlers.MpesaC2BValidation(repo))     // <-- call it
	r.Post("/hooks/paywatch/c2b/confirmation", handlers.MpesaC2BConfirmation(repo)) // <-- call it

	// --- Tenant APIs (API-key auth) ---
	r.Group(func(pr chi.Router) {
		pr.Use(middlewarex.APIKeyAuth(repo))

		// STK push
		pr.Post("/v1/payments/stk", handlers.CreateSTK(repo, mp))

		// Debug / verification APIs
		pr.Get("/v1/payments", handlers.ListPayments(repo))
		pr.Get("/v1/events", handlers.ListEvents(repo))

		// Optional: register C2B URLs on Daraja
		pr.Post("/v1/mpesa/register-c2b", handlers.RegisterC2B(repo, mp, cfg))
	})

	// Tenant APIs (Bearer <API-KEY>)
	r.Group(func(pr chi.Router) {
		pr.Use(middlewarex.APIKeyAuth(repo))
		pr.Post("/v1/payments/stk", handlers.CreateSTK(repo, mp))
		// ... your tenant endpoints
	})

	return r
}
