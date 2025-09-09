package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"paymatch/internal/domain/credential"
	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/provider"

	"github.com/rs/zerolog/log"
)

// STKPush handles STK push payment requests
func STKPush(registry *provider.Registry, tenantService interface {
	GetTenantCredentials(ctx context.Context, tenantID int64) ([]*credential.ProviderCredential, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			writeErrorResponse(w, "tenant not found", http.StatusUnauthorized)
			return
		}

		// Parse request
		var req provider.STKPushReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErrorResponse(w, "invalid JSON request", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if req.Amount <= 0 {
			writeErrorResponse(w, "amount must be greater than 0", http.StatusBadRequest)
			return
		}
		if req.PhoneNumber == "" {
			writeErrorResponse(w, "phone_number is required", http.StatusBadRequest)
			return
		}
		if req.AccountReference == "" {
			writeErrorResponse(w, "account_reference is required", http.StatusBadRequest)
			return
		}
		if req.CallbackURL == "" {
			writeErrorResponse(w, "callback_url is required", http.StatusBadRequest)
			return
		}

		// Get tenant credentials from credential service
		credentials, err := tenantService.GetTenantCredentials(r.Context(), tenantID)
		if err != nil {
			log.Error().Err(err).Int64("tenant_id", tenantID).Msg("Failed to get tenant credentials")
			writeErrorResponse(w, "failed to get tenant credentials", http.StatusInternalServerError)
			return
		}

		if len(credentials) == 0 {
			writeErrorResponse(w, "no provider credentials found for tenant", http.StatusBadRequest)
			return
		}

		// Use the first credential (for now, in the future this could be provider-specific)
		cred := credentials[0]

		log.Info().
			Int64("tenant_id", tenantID).
			Int64("amount", req.Amount).
			Str("phone_number", req.PhoneNumber).
			Str("provider", string(cred.ProviderType)).
			Msg("STK Push request received")

		// Call the provider registry to perform STK Push
		response, err := registry.STKPush(r.Context(), cred, req)
		if err != nil {
			log.Error().Err(err).
				Int64("tenant_id", tenantID).
				Str("provider", string(cred.ProviderType)).
				Msg("STK Push failed")
			writeErrorResponse(w, "STK Push failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error().Err(err).Msg("Failed to encode STK Push response")
			writeErrorResponse(w, "failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}

// B2C handles business to customer transfer requests
func B2C(registry *provider.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			writeErrorResponse(w, "tenant not found", http.StatusUnauthorized)
			return
		}

		// Parse request
		var req provider.B2CReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErrorResponse(w, "invalid JSON request", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if req.Amount <= 0 {
			writeErrorResponse(w, "amount must be greater than 0", http.StatusBadRequest)
			return
		}
		if req.PhoneNumber == "" {
			writeErrorResponse(w, "phone_number is required", http.StatusBadRequest)
			return
		}
		if req.ResultURL == "" {
			writeErrorResponse(w, "result_url is required", http.StatusBadRequest)
			return
		}
		if req.TimeoutURL == "" {
			writeErrorResponse(w, "timeout_url is required", http.StatusBadRequest)
			return
		}

		// TODO: Get tenant's credential (this requires a service to get credentials by tenant ID)
		// For now, return not implemented
		writeErrorResponse(w, "B2C implementation requires credential lookup service", http.StatusNotImplemented)
		
		// The full implementation would be:
		// 1. Get tenant credentials from credential service
		// 2. Call registry.B2C(ctx, cred, req)
		// 3. Return response

		log.Info().
			Int64("tenant_id", tenantID).
			Int64("amount", req.Amount).
			Str("phone_number", req.PhoneNumber).
			Msg("B2C request received")
	}
}