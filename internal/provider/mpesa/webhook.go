package mpesa

import (
	"encoding/json"
	"fmt"
	"paymatch/internal/provider"
)

// ParseWebhook converts Daraja callback payloads (STK/C2B) into generic provider.Event
func (p *Provider) ParseWebhook(body []byte) (provider.Event, error) {
	// Try STK callback shape first
	var stk struct {
		Body struct {
			StkCallback struct {
				CheckoutRequestID string `json:"CheckoutRequestID"`
				ResultCode        int    `json:"ResultCode"`
				ResultDesc        string `json:"ResultDesc"`
				CallbackMetadata  struct {
					Item []struct {
						Name  string `json:"Name"`
						Value any    `json:"Value"`
					} `json:"Item"`
				} `json:"CallbackMetadata"`
			} `json:"stkCallback"`
		} `json:"Body"`
	}
	if err := json.Unmarshal(body, &stk); err == nil && stk.Body.StkCallback.CheckoutRequestID != "" {
		// Extract minimal fields
		var amount int
		var msisdn, ref string
		for _, it := range stk.Body.StkCallback.CallbackMetadata.Item {
			switch it.Name {
			case "Amount":
				if f, ok := it.Value.(float64); ok {
					amount = int(f)
				}
			case "MpesaReceiptNumber": // ignored here
			case "PhoneNumber":
				if f, ok := it.Value.(float64); ok {
					msisdn = fmtInt(f)
				} else if s, ok := it.Value.(string); ok {
					msisdn = s
				}
			case "AccountReference":
				if s, ok := it.Value.(string); ok {
					ref = s
				}
			}
		}
		return provider.Event{Type: provider.EventSTK, ExternalID: stk.Body.StkCallback.CheckoutRequestID, Amount: amount, MSISDN: msisdn, InvoiceRef: ref, RawJSON: body}, nil
	}

	// Try C2B confirmation shape
	var c2b map[string]any
	if err := json.Unmarshal(body, &c2b); err == nil {
		if tx, ok := c2b["TransID"].(string); ok && tx != "" {
			amt := 0
			if f, ok := c2b["TransAmount"].(float64); ok {
				amt = int(f)
			}
			ms := ""
			if s, ok := c2b["MSISDN"].(string); ok {
				ms = s
			}
			ref := ""
			if s, ok := c2b["BillRefNumber"].(string); ok {
				ref = s
			}
			return provider.Event{Type: provider.EventC2B, ExternalID: tx, Amount: amt, MSISDN: ms, InvoiceRef: ref, RawJSON: body}, nil
		}
	}
	return provider.Event{}, fmt.Errorf("unrecognized webhook shape")
}

func fmtInt(f float64) string { return fmt.Sprintf("%.0f", f) }
