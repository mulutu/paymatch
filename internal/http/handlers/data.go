package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/services/data"
)

// ListPayments handles payment listing requests using the data service
func ListPayments(dataService *data.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			http.Error(w, "tenant not found", http.StatusUnauthorized)
			return
		}

		// Parse query parameters
		req := parseListRequest(r)

		// Use data service to handle business logic
		response, err := dataService.ListPayments(r.Context(), tenantID, req)
		if err != nil {
			if serviceErr, ok := err.(*data.ServiceError); ok {
				http.Error(w, "failed to list payments: "+serviceErr.Error(), http.StatusInternalServerError)
			} else {
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}

		// Return response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// ListEvents handles event listing requests using the data service
func ListEvents(dataService *data.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			http.Error(w, "tenant not found", http.StatusUnauthorized)
			return
		}

		// Parse query parameters
		req := parseListRequest(r)

		// Use data service to handle business logic
		response, err := dataService.ListEvents(r.Context(), tenantID, req)
		if err != nil {
			if serviceErr, ok := err.(*data.ServiceError); ok {
				http.Error(w, "failed to list events: "+serviceErr.Error(), http.StatusInternalServerError)
			} else {
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
			return
		}

		// Return response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// parseListRequest parses HTTP query parameters into ListRequest
func parseListRequest(r *http.Request) data.ListRequest {
	req := data.ListRequest{}
	
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			req.Limit = n
		}
	}
	
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			req.Offset = n
		}
	}
	
	return req
}