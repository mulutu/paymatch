package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"paymatch/internal/config"
	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/services/event"
	"paymatch/internal/services/tenant"
)

// OnboardTenant handles tenant onboarding using the tenant service
func OnboardTenant(tenantService *tenant.Service, cfg config.Cfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Admin guard
		adminToken := r.Header.Get("X-Admin-Token")
		if cfg.Sec.AdminToken == "" || adminToken != cfg.Sec.AdminToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse request
		var req tenant.OnboardingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Use tenant service to handle onboarding
		response, err := tenantService.OnboardTenant(r.Context(), req)
		if err != nil {
			// Handle different error types
			switch e := err.(type) {
			case *tenant.ValidationError:
				http.Error(w, e.Error(), http.StatusBadRequest)
			case *tenant.ServiceError:
				http.Error(w, "onboarding failed: "+e.Error(), http.StatusInternalServerError)
			default:
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// ReplayEvents handles event replay requests using the events service
func ReplayEvents(eventsService *event.ReplayService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			http.Error(w, "tenant not found", http.StatusUnauthorized)
			return
		}

		// Parse request
		var requestData struct {
			EventIDs []int64 `json:"eventIds,omitempty"`
			SinceISO string  `json:"since,omitempty"` // RFC3339
			UntilISO string  `json:"until,omitempty"` // RFC3339
			Max      int     `json:"max,omitempty"`   // default 200, max 1000
		}
		
		if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Convert to service request
		req := event.ReplayRequest{
			EventIDs: requestData.EventIDs,
			Max:      requestData.Max,
		}

		// Parse time fields
		if requestData.SinceISO != "" {
			if t, err := time.Parse(time.RFC3339, requestData.SinceISO); err == nil {
				req.Since = &t
			}
		}
		if requestData.UntilISO != "" {
			if t, err := time.Parse(time.RFC3339, requestData.UntilISO); err == nil {
				req.Until = &t
			}
		}

		// Use events service to handle replay
		response, err := eventsService.ReplayEvents(r.Context(), tenantID, req)
		if err != nil {
			http.Error(w, "replay failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Return response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}