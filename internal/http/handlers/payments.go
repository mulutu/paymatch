package handlers

import (
	"encoding/json"
	"net/http"

	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/provider"

	"github.com/rs/zerolog/log"
)

// STKPush handles STK push payment requests
func STKPush(registry *provider.Registry) http.HandlerFunc {
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

		// TODO: Get tenant's credential (this requires a service to get credentials by tenant ID)
		// For now, return not implemented
		writeErrorResponse(w, "STK Push implementation requires credential lookup service", http.StatusNotImplemented)
		
		// The full implementation would be:
		// 1. Get tenant credentials from credential service
		// 2. Call registry.STKPush(ctx, cred, req)
		// 3. Return response

		log.Info().
			Int64("tenant_id", tenantID).
			Int64("amount", req.Amount).
			Str("phone_number", req.PhoneNumber).
			Msg("STK Push request received")
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