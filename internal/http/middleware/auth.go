package middlewarex

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"paymatch/internal/store/postgres"
)

func APIKeyAuth(repo *postgres.Repo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "missing bearer", http.StatusUnauthorized)
				return
			}
			key := strings.TrimPrefix(auth, "Bearer ")
			h := sha256.Sum256([]byte(key))
			hx := hex.EncodeToString(h[:])

			ten, err := repo.LookupTenantByAPIKeyHash(r.Context(), hx)
			if err != nil {
				http.Error(w, "invalid key", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(WithTenantID(r.Context(), ten.ID)))
		})
	}
}
