// internal/http/handlers/stk.go
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/provider"
	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"

	"github.com/rs/zerolog/log"
)

type stkReq struct {
	Amount      int    `json:"amount"`
	Phone       string `json:"phone"`
	AccountRef  string `json:"accountRef"`
	Description string `json:"description"`
	Shortcode   string `json:"shortcode,omitempty"`
}
type stkResp struct {
	CheckoutRequestID string `json:"checkoutRequestId"`
	CustomerMessage   string `json:"customerMessage"`
}

func CreateSTK(repo *postgres.Repo, mp *mpesa.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			http.Error(w, "tenant not found", http.StatusUnauthorized)
			return
		}

		var in stkReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if in.Amount <= 0 || in.Phone == "" || in.AccountRef == "" {
			http.Error(w, "missing amount/phone/accountRef", http.StatusBadRequest)
			return
		}

		// Short, bounded context for provider call
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		cred, err := repo.ResolveCredential(ctx, tenantID, in.Shortcode)
		if err != nil {
			http.Error(w, "credential not found", http.StatusNotFound)
			return
		}

		// 1) Ask Daraja to initiate STK
		out, err := mp.STKPush(ctx, cred, provider.STKPushReq{
			Amount:      in.Amount,
			Phone:       in.Phone,
			AccountRef:  in.AccountRef,
			Description: in.Description,
		})
		if err != nil {
			log.Error().Err(err).
				Int64("tenant_id", tenantID).
				Str("shortcode", cred.Shortcode).
				Str("environment", cred.Environment).
				Str("phone", in.Phone).
				Int("amount", in.Amount).
				Msg("STK push failed")
			http.Error(w, "stk failed", http.StatusBadGateway)
			return
		}

		// 2) Record the pending payment; if THIS fails, DO NOT proceed.
		if err := repo.UpsertPendingPayment(ctx, tenantID, cred.ID, in.AccountRef, in.Amount, out.CheckoutRequestID); err != nil {
			log.Error().Err(err).
				Int64("tenant_id", tenantID).
				Str("invoice", in.AccountRef).
				Str("checkout_request_id", out.CheckoutRequestID).
				Msg("failed to save pending payment")
			// Return a server error so the client treats the whole call as failed.
			http.Error(w, "failed to persist payment", http.StatusInternalServerError)
			return
		}

		// 3) Only if DB write succeeded, return success to client.
		_ = json.NewEncoder(w).Encode(stkResp{
			CheckoutRequestID: out.CheckoutRequestID,
			CustomerMessage:   out.CustomerMessage,
		})
	}
}
