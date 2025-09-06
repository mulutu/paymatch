package handlers

import (
	"io"
	"net/http"

	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"

	"github.com/go-chi/chi/v5"
)

func MpesaWebhookByShortcode(repo *postgres.Repo, mp *mpesa.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shortcode := chi.URLParam(r, "shortcode")
		cred, tenant, err := repo.FindCredentialByShortcode(r.Context(), shortcode)
		if err != nil {
			http.Error(w, "unknown shortcode", 404)
			return
		}

		body, _ := io.ReadAll(r.Body)
		evt, err := mp.ParseWebhook(body)
		if err != nil {
			http.Error(w, "bad payload", 400)
			return
		}

		eid, err := repo.SaveEvent(r.Context(), tenant.ID, cred.ID, evt)
		if err != nil {
			http.Error(w, "save failed", 500)
			return
		}

		_ = repo.EnqueueEvent(r.Context(), tenant.ID, eid)

		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}
}

func MpesaWebhookByToken(repo *postgres.Repo, mp *mpesa.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-PM-Webhook-Token")
		cred, tenant, err := repo.FindCredentialByWebhookToken(r.Context(), tok)
		if err != nil {
			http.Error(w, "unknown token", 404)
			return
		}

		body, _ := io.ReadAll(r.Body)
		evt, err := mp.ParseWebhook(body)
		if err != nil {
			http.Error(w, "bad payload", 400)
			return
		}

		eid, err := repo.SaveEvent(r.Context(), tenant.ID, cred.ID, evt)
		if err != nil {
			http.Error(w, "save failed", 500)
			return
		}

		_ = repo.EnqueueEvent(r.Context(), tenant.ID, eid)

		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}
}
