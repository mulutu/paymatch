package handlers

import (
	"encoding/json"
	"net/http"

	"paymatch/internal/config"
	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"
)

type c2bReq struct {
	Shortcode    string `json:"shortcode,omitempty"`
	ResponseType string `json:"responseType,omitempty"` // default "Completed"
	// If not provided, defaults to CALLBACK_BASE_URL + "/hooks/mpesa"
	ConfirmURL  string `json:"confirmUrl,omitempty"`
	ValidateURL string `json:"validateUrl,omitempty"`
}

func RegisterC2B(repo *postgres.Repo, mp *mpesa.Provider, cfg config.Cfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			http.Error(w, "tenant not found", http.StatusUnauthorized)
			return
		}

		var in c2bReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		cred, err := repo.ResolveCredential(r.Context(), tenantID, in.Shortcode)
		if err != nil {
			http.Error(w, "credential not found", http.StatusNotFound)
			return
		}

		responseType := in.ResponseType
		if responseType == "" {
			responseType = "Completed"
		}
		confirm := in.ConfirmURL
		validate := in.ValidateURL
		if confirm == "" {
			confirm = cfg.CBBaseURL() + "/hooks/mpesa"
		}
		if validate == "" {
			validate = cfg.CBBaseURL() + "/hooks/mpesa"
		}

		if err := mp.RegisterC2B(r.Context(), cred, confirm, validate, responseType); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
