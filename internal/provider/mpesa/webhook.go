package mpesa

import (
	"encoding/json"
	"fmt"

	"paymatch/internal/provider"
)

// WebhookService handles M-Pesa webhook parsing and validation
type WebhookService struct{}

// NewWebhookService creates a new webhook service
func NewWebhookService() *WebhookService {
	return &WebhookService{}
}

// Parse parses M-Pesa webhook payload and converts to standard Event
func (w *WebhookService) Parse(body []byte, headers map[string]string) (provider.Event, error) {
	// Try STK callback first
	if event, err := w.parseSTKCallback(body); err == nil {
		return event, nil
	}

	// Try C2B confirmation
	if event, err := w.parseC2BCallback(body); err == nil {
		return event, nil
	}

	// Try B2C result
	if event, err := w.parseB2CResult(body); err == nil {
		return event, nil
	}

	return provider.Event{}, &provider.ProviderError{
		Code:    "unrecognized_webhook",
		Message: "unrecognized M-Pesa webhook payload format",
	}
}

// parseSTKCallback parses STK Push callback
func (w *WebhookService) parseSTKCallback(body []byte) (provider.Event, error) {
	var stkCallback struct {
		Body struct {
			StkCallback struct {
				MerchantRequestID string `json:"MerchantRequestID"`
				CheckoutRequestID string `json:"CheckoutRequestID"`
				ResultCode        int    `json:"ResultCode"`
				ResultDesc        string `json:"ResultDesc"`
				CallbackMetadata  struct {
					Item []struct {
						Name  string      `json:"Name"`
						Value interface{} `json:"Value"`
					} `json:"Item"`
				} `json:"CallbackMetadata,omitempty"`
			} `json:"stkCallback"`
		} `json:"Body"`
	}

	if err := json.Unmarshal(body, &stkCallback); err != nil {
		return provider.Event{}, err
	}

	callback := stkCallback.Body.StkCallback
	if callback.CheckoutRequestID == "" {
		return provider.Event{}, fmt.Errorf("not an STK callback")
	}

	// Extract metadata
	var amount int
	var msisdn, transactionID, accountRef string

	if callback.CallbackMetadata.Item != nil {
		for _, item := range callback.CallbackMetadata.Item {
			switch item.Name {
			case "Amount":
				if f, ok := item.Value.(float64); ok {
					amount = int(f)
				}
			case "MpesaReceiptNumber":
				if s, ok := item.Value.(string); ok {
					transactionID = s
				}
			case "PhoneNumber":
				if f, ok := item.Value.(float64); ok {
					msisdn = fmt.Sprintf("%.0f", f)
				} else if s, ok := item.Value.(string); ok {
					msisdn = s
				}
			case "AccountReference":
				if s, ok := item.Value.(string); ok {
					accountRef = s
				}
			}
		}
	}

	// Determine status
	status := provider.StatusFailed
	if callback.ResultCode == 0 {
		status = provider.StatusCompleted
	}

	return provider.Event{
		Type:                provider.EventSTK,
		ExternalID:          callback.CheckoutRequestID,
		Amount:              int64(amount),
		MSISDN:              msisdn,
		InvoiceRef:          accountRef,
		TransactionID:       transactionID,
		Status:              status,
		ResponseDescription: callback.ResultDesc,
		RawJSON:             body,
	}, nil
}

// parseC2BCallback parses C2B confirmation callback
func (w *WebhookService) parseC2BCallback(body []byte) (provider.Event, error) {
	var c2bCallback map[string]interface{}

	if err := json.Unmarshal(body, &c2bCallback); err != nil {
		return provider.Event{}, err
	}

	// Check for C2B fields
	transID, hasTransID := c2bCallback["TransID"].(string)
	if !hasTransID || transID == "" {
		return provider.Event{}, fmt.Errorf("not a C2B callback")
	}

	// Extract fields
	amount := 0
	if f, ok := c2bCallback["TransAmount"].(float64); ok {
		amount = int(f)
	}

	msisdn := ""
	if s, ok := c2bCallback["MSISDN"].(string); ok {
		msisdn = s
	}

	billRefNumber := ""
	if s, ok := c2bCallback["BillRefNumber"].(string); ok {
		billRefNumber = s
	}

	firstName := ""
	if s, ok := c2bCallback["FirstName"].(string); ok {
		firstName = s
	}

	return provider.Event{
		Type:                provider.EventC2B,
		ExternalID:          transID,
		Amount:              int64(amount),
		MSISDN:              msisdn,
		InvoiceRef:          billRefNumber,
		TransactionID:       transID,
		Status:              provider.StatusCompleted, // C2B callbacks are always successful
		ResponseDescription: fmt.Sprintf("C2B payment from %s", firstName),
		RawJSON:             body,
	}, nil
}

// parseB2CResult parses B2C result callback
func (w *WebhookService) parseB2CResult(body []byte) (provider.Event, error) {
	var b2cResult struct {
		Result struct {
			ResultType               int    `json:"ResultType"`
			ResultCode               int    `json:"ResultCode"`
			ResultDesc               string `json:"ResultDesc"`
			OriginatorConversationID string `json:"OriginatorConversationID"`
			ConversationID           string `json:"ConversationID"`
			TransactionID            string `json:"TransactionID"`
			ResultParameters         struct {
				ResultParameter []struct {
					Key   string      `json:"Key"`
					Value interface{} `json:"Value"`
				} `json:"ResultParameter"`
			} `json:"ResultParameters,omitempty"`
		} `json:"Result"`
	}

	if err := json.Unmarshal(body, &b2cResult); err != nil {
		return provider.Event{}, err
	}

	result := b2cResult.Result
	if result.ConversationID == "" {
		return provider.Event{}, fmt.Errorf("not a B2C result")
	}

	// Extract parameters
	var amount int
	var msisdn, transactionID string

	if result.ResultParameters.ResultParameter != nil {
		for _, param := range result.ResultParameters.ResultParameter {
			switch param.Key {
			case "TransactionAmount":
				if f, ok := param.Value.(float64); ok {
					amount = int(f)
				}
			case "TransactionReceipt":
				if s, ok := param.Value.(string); ok {
					transactionID = s
				}
			case "ReceiverPartyPublicName":
				if s, ok := param.Value.(string); ok {
					msisdn = s
				}
			}
		}
	}

	// Determine status
	status := provider.StatusFailed
	if result.ResultCode == 0 {
		status = provider.StatusCompleted
	}

	return provider.Event{
		Type:                provider.EventB2C,
		ExternalID:          result.ConversationID,
		Amount:              int64(amount),
		MSISDN:              msisdn,
		InvoiceRef:          result.OriginatorConversationID,
		TransactionID:       transactionID,
		Status:              status,
		ResponseDescription: result.ResultDesc,
		RawJSON:             body,
	}, nil
}

// Validate validates M-Pesa webhook authenticity
func (w *WebhookService) Validate(body []byte, headers map[string]string, webhookToken string) error {
	// M-Pesa doesn't provide signature validation in their current implementation
	// This is a placeholder for future enhancement or custom validation
	
	// For now, we can validate that required headers are present
	if webhookToken != "" {
		// Custom validation logic could be added here
		// For example, checking a custom header that matches the webhook token
		if token := headers["X-Webhook-Token"]; token != webhookToken {
			// Only validate if the header is present - some M-Pesa callbacks don't include custom headers
			if token != "" {
				return &provider.ProviderError{
					Code:    "invalid_webhook_token",
					Message: "webhook token validation failed",
				}
			}
		}
	}

	// Basic payload validation - ensure it's valid JSON
	var temp interface{}
	if err := json.Unmarshal(body, &temp); err != nil {
		return &provider.ProviderError{
			Code:    "invalid_webhook_payload",
			Message: "webhook payload is not valid JSON",
		}
	}

	return nil
}