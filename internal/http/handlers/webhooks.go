package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"paymatch/internal/domain/event"
	"paymatch/internal/provider"
	eventservice "paymatch/internal/services/event"
	"paymatch/internal/services/tenant"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// WebhookByShortcode handles provider webhooks with pure architecture
func WebhookByShortcode(
	tenantSvc *tenant.Service,
	eventProcessor *eventservice.Processor,
	providerRegistry *provider.Registry,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shortcode := chi.URLParam(r, "shortcode")
		if shortcode == "" {
			writeErrorResponse(w, "shortcode is required", http.StatusBadRequest)
			return
		}

		// Read webhook payload
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErrorResponse(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		// Get headers for webhook validation
		headers := make(map[string]string)
		for name, values := range r.Header {
			if len(values) > 0 {
				headers[name] = values[0]
			}
		}

		// Find provider credential by shortcode
		credential, err := tenantSvc.GetCredentialByShortcode(r.Context(), shortcode)
		if err != nil {
			log.Error().Err(err).Str("shortcode", shortcode).Msg("credential not found")
			writeErrorResponse(w, "invalid shortcode", http.StatusNotFound)
			return
		}

		// Get provider for this credential
		provider, err := providerRegistry.GetProviderForCredential(r.Context(), credential)
		if err != nil {
			log.Error().Err(err).Str("shortcode", shortcode).Msg("provider not found")
			writeErrorResponse(w, "provider not available", http.StatusInternalServerError)
			return
		}

		// Validate webhook signature
		if err := provider.ValidateWebhook(body, headers, credential.WebhookToken); err != nil {
			log.Warn().Err(err).Str("shortcode", shortcode).Msg("webhook validation failed")
			writeErrorResponse(w, "invalid webhook signature", http.StatusUnauthorized)
			return
		}

		// Parse webhook payload
		providerEvent, err := provider.ParseWebhook(body, headers)
		if err != nil {
			log.Error().Err(err).Str("shortcode", shortcode).Msg("failed to parse webhook")
			writeErrorResponse(w, "invalid webhook payload", http.StatusBadRequest)
			return
		}

		// Convert to domain event
		domainEvent, err := createDomainEvent(credential.TenantID, credential.ID, providerEvent, body)
		if err != nil {
			log.Error().Err(err).Str("shortcode", shortcode).Msg("failed to create domain event")
			writeErrorResponse(w, "failed to process event", http.StatusInternalServerError)
			return
		}

		// Process the event asynchronously (this adds it to the processing queue)
		if err := eventProcessor.ProcessEvent(r.Context(), domainEvent); err != nil {
			log.Error().Err(err).Str("shortcode", shortcode).Int64("event_id", domainEvent.ID).Msg("failed to process event")
			writeErrorResponse(w, "failed to process event", http.StatusInternalServerError)
			return
		}

		log.Info().
			Str("shortcode", shortcode).
			Int64("tenant_id", credential.TenantID).
			Str("event_type", string(domainEvent.Type)).
			Str("external_id", domainEvent.ExternalID).
			Msg("webhook processed successfully")

		// Return success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "success",
			"message": "webhook processed",
		})
	}
}

// createDomainEvent converts a provider event to a domain event
func createDomainEvent(tenantID, credentialID int64, providerEvent provider.Event, rawJSON []byte) (*event.Event, error) {
	// Create domain event with validation
	domainEvent, err := event.NewEvent(
		tenantID,
		credentialID,
		event.Type(providerEvent.Type),
		providerEvent.ExternalID,
		rawJSON,
	)
	if err != nil {
		return nil, err
	}

	// Set additional fields from provider event
	domainEvent.Amount = providerEvent.Amount
	domainEvent.MSISDN = providerEvent.MSISDN
	domainEvent.InvoiceRef = providerEvent.InvoiceRef
	domainEvent.TransactionID = providerEvent.TransactionID
	domainEvent.Status = providerEvent.Status
	domainEvent.ResponseDescription = providerEvent.ResponseDescription

	return domainEvent, nil
}

// writeErrorResponse writes a JSON error response
func writeErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   true,
		"message": message,
	})
}