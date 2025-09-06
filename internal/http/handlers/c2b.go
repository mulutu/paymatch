// internal/http/handlers/c2b.go
package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"paymatch/internal/provider"
	"paymatch/internal/store/postgres"
)

type c2bCallback struct {
	TransactionType   string `json:"TransactionType"` // "Pay Bill", "Buy Goods" etc.
	TransID           string `json:"TransID"`
	TransTime         string `json:"TransTime"`
	TransAmount       string `json:"TransAmount"`       // e.g. "100.00"
	BusinessShortCode string `json:"BusinessShortCode"` // use to resolve tenant/cred
	BillRefNumber     string `json:"BillRefNumber"`     // may be empty for buygoods
	InvoiceNumber     string `json:"InvoiceNumber"`
	MSISDN            string `json:"MSISDN"`
	FirstName         string `json:"FirstName"`
	MiddleName        string `json:"MiddleName"`
	LastName          string `json:"LastName"`
}

type c2bValidationResponse struct {
	ResultCode        int    `json:"ResultCode"` // 0 accept; non-zero reject
	ResultDesc        string `json:"ResultDesc"`
	ThirdPartyTransID string `json:"ThirdPartyTransID,omitempty"`
}

// VALIDATION: apply tenant rules before M-Pesa confirms the transaction.
func MpesaC2BValidation(repo *postgres.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cb c2bCallback
		if err := json.NewDecoder(r.Body).Decode(&cb); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		cred, _, err := repo.FindCredentialByShortcode(r.Context(), strings.TrimSpace(cb.BusinessShortCode))
		if err != nil {
			json.NewEncoder(w).Encode(c2bValidationResponse{ResultCode: 1, ResultDesc: "Unknown ShortCode"})
			return
		}

		// Enforce by C2B mode
		if cred.C2BMode == "paybill" {
			// PayBill requires a BillRef (typically invoice/order ID)
			if cred.BillRefRequired && strings.TrimSpace(cb.BillRefNumber) == "" {
				json.NewEncoder(w).Encode(c2bValidationResponse{ResultCode: 1, ResultDesc: "BillRef required"})
				return
			}
			if rx := strings.TrimSpace(cred.BillRefRegex); rx != "" {
				if ok, _ := regexp.MatchString(rx, cb.BillRefNumber); !ok {
					json.NewEncoder(w).Encode(c2bValidationResponse{ResultCode: 1, ResultDesc: "BillRef invalid"})
					return
				}
			}
		} else {
			// buygoods: ignore BillRef (optional). You could still reject on amount caps etc.
		}

		json.NewEncoder(w).Encode(c2bValidationResponse{ResultCode: 0, ResultDesc: "Accepted"})
	}
}

// CONFIRMATION: persist the payment/event; only ACK if event is durably saved.
func MpesaC2BConfirmation(repo *postgres.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var cb c2bCallback
		if err := json.NewDecoder(r.Body).Decode(&cb); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		cred, tenant, err := repo.FindCredentialByShortcode(r.Context(), strings.TrimSpace(cb.BusinessShortCode))
		if err != nil {
			http.Error(w, "unknown shortcode", http.StatusNotFound)
			return
		}

		// parse amount (string -> int KES)
		amt := 0
		if t := strings.TrimSpace(cb.TransAmount); t != "" {
			if f, err := strconv.ParseFloat(t, 64); err == nil {
				amt = int(f + 0.5)
			}
		}

		// BillRef may be empty for buygoods; that's OK.
		invoice := strings.TrimSpace(cb.BillRefNumber)

		// 1) Save raw event FIRST (idempotent). If this fails, DO NOT ACK.
		raw, _ := json.Marshal(cb)
		eid, err := repo.SaveEvent(r.Context(), tenant.ID, cred.ID, provider.Event{
			Type:       provider.EventC2B,
			ExternalID: strings.TrimSpace(cb.TransID),
			Amount:     amt,
			MSISDN:     strings.TrimSpace(cb.MSISDN),
			InvoiceRef: invoice,
			RawJSON:    raw,
		})
		if err != nil {
			// Return non-2xx so Safaricom retries; we haven't captured the event.
			http.Error(w, "temporary failure", http.StatusInternalServerError)
			return
		}

		// 2) Best-effort enqueue for replay/retry processing.
		if err := repo.EnqueueEvent(r.Context(), tenant.ID, eid); err != nil {
			// Not fatal: event is saved; you can replay/backfill later.
			// log.Warn().Err(err).Msg("enqueue failed")
		}

		// 3) Best-effort immediate payment upsert (optional; worker will also reconcile).
		if err := repo.UpsertPaymentByExternalID(
			r.Context(),
			tenant.ID,
			cred.ID,
			strings.TrimSpace(cb.TransID),
			invoice,
			amt,
			strings.TrimSpace(cb.MSISDN),
		); err != nil {
			// Not fatal; event is saved and will be retried by the queue worker.
			// log.Warn().Err(err).Msg("upsert payment failed (c2b)")
		}

		// 4) Now it’s safe to ACK the provider.
		w.Header().Set("Content-Type", "application/json")
		// Daraja C2B confirmation typically accepts 200 with any body; this is fine.
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ResultDesc":"Received"}`))
	}
}
