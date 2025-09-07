package middlewarex

import (
	"net/http"
	"strings"

	"paymatch/internal/config"
	"paymatch/internal/services/tenant"
)

func APIKeyAuth(tenantService *tenant.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			key := strings.TrimPrefix(auth, "Bearer ")

			ten, err := tenantService.GetTenantByAPIKey(r.Context(), key)
			if err != nil {
				http.Error(w, "invalid key", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithTenantID(r.Context(), ten.ID)))
		})
	}
}

// AdminAuth middleware for protecting admin endpoints
func AdminAuth(cfg config.Cfg) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			adminToken := r.Header.Get("X-Admin-Token")
			if adminToken == "" {
				http.Error(w, "admin token required", http.StatusUnauthorized)
				return
			}

			// Compare with configured admin token
			if cfg.Sec.AdminToken == "" {
				http.Error(w, "admin access disabled", http.StatusForbidden)
				return
			}

			if adminToken != cfg.Sec.AdminToken {
				http.Error(w, "invalid admin token", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
