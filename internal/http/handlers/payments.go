package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/store/postgres"
)

func ListPayments(repo *postgres.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			http.Error(w, "tenant not found", http.StatusUnauthorized)
			return
		}
		limit := 50
		offset := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		rows, err := repo.ListPayments(r.Context(), tenantID, limit, offset)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": rows})
	}
}

func ListEvents(repo *postgres.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			http.Error(w, "tenant not found", http.StatusUnauthorized)
			return
		}
		limit := 50
		offset := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		items, err := repo.ListEvents(r.Context(), tenantID, limit, offset)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": items})
	}
}
