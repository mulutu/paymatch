package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"paymatch/internal/config"
	"paymatch/internal/crypto"
	"paymatch/internal/store/postgres"
)

type onboardTenantReq struct {
	Name            string `json:"name"`                      // Tenant name
	APIKeyName      string `json:"apiKeyName,omitempty"`      // Optional label for the API key
	Provider        string `json:"provider,omitempty"`        // default: "mpesa_daraja"
	Shortcode       string `json:"shortcode"`                 // Paybill/Till number
	Environment     string `json:"environment"`               // "sandbox" | "production"
	C2BMode         string `json:"c2bMode"`                   // "paybill" | "buygoods"
	BillRefRequired *bool  `json:"billRefRequired,omitempty"` // defaults by mode
	BillRefRegex    string `json:"billRefRegex,omitempty"`    // optional
	Passkey         string `json:"passkey"`                   // Daraja passkey
	ConsumerKey     string `json:"consumerKey"`               // Daraja consumer key
	ConsumerSecret  string `json:"consumerSecret"`            // Daraja consumer secret
}

type onboardTenantResp struct {
	TenantID        int64  `json:"tenantId"`
	APIKey          string `json:"apiKey"` // plaintext (show once!)
	APIKeyName      string `json:"apiKeyName"`
	WebhookToken    string `json:"webhookToken"`
	Shortcode       string `json:"shortcode"`
	Environment     string `json:"environment"`
	C2BMode         string `json:"c2bMode"`
	BillRefRequired bool   `json:"billRefRequired"`
	BillRefRegex    string `json:"billRefRegex"`
}

func OnboardTenant(repo *postgres.Repo, cfg config.Cfg) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// --- Admin guard ---
		admin := r.Header.Get("X-Admin-Token")
		if cfg.Sec.AdminToken == "" || admin != cfg.Sec.AdminToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// --- Parse & validate input ---
		var in onboardTenantReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		in.Provider = strings.TrimSpace(in.Provider)
		if in.Provider == "" {
			in.Provider = "mpesa_daraja"
		}

		in.Environment = strings.ToLower(strings.TrimSpace(in.Environment))
		if in.Environment != "sandbox" && in.Environment != "production" {
			http.Error(w, "environment must be sandbox|production", http.StatusBadRequest)
			return
		}

		in.C2BMode = strings.ToLower(strings.TrimSpace(in.C2BMode))
		if in.C2BMode != "paybill" && in.C2BMode != "buygoods" {
			http.Error(w, "c2bMode must be paybill|buygoods", http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Shortcode) == "" ||
			strings.TrimSpace(in.Passkey) == "" || strings.TrimSpace(in.ConsumerKey) == "" || strings.TrimSpace(in.ConsumerSecret) == "" {
			http.Error(w, "name, shortcode, passkey, consumerKey, consumerSecret are required", http.StatusBadRequest)
			return
		}

		// Default bill_ref_required by mode if not supplied
		billRefRequired := true
		if in.C2BMode == "buygoods" {
			billRefRequired = false
		}
		if in.BillRefRequired != nil {
			billRefRequired = *in.BillRefRequired
		}

		// Validate regex if provided
		if strings.TrimSpace(in.BillRefRegex) != "" {
			if _, err := regexp.Compile(in.BillRefRegex); err != nil {
				http.Error(w, "invalid billRefRegex", http.StatusBadRequest)
				return
			}
		}

		// --- Create tenant ---
		tenant, err := repo.CreateTenant(r.Context(), strings.TrimSpace(in.Name))
		if err != nil {
			http.Error(w, "create tenant failed: "+err.Error(), http.StatusBadRequest)
			return
		}

		// --- Generate API key (plaintext) & store hash ---
		apiKey := mustRandHex(32) // 32 bytes -> 64 hex chars
		apiKeyHash := sha256Hex(apiKey)
		keyName := in.APIKeyName
		if keyName == "" {
			keyName = "default"
		}
		if _, err := repo.InsertAPIKey(r.Context(), tenant.ID, keyName, apiKeyHash); err != nil {
			http.Error(w, "store api key failed: "+err.Error(), http.StatusBadRequest)
			return
		}

		// --- Encrypt Daraja secrets with AES_256_KEY_BASE64 ---
		passEnc, err := crypto.EncryptString(cfg.Sec.AESKey, in.Passkey)
		if err != nil {
			http.Error(w, "encrypt passkey failed", http.StatusInternalServerError)
			return
		}
		ckEnc, err := crypto.EncryptString(cfg.Sec.AESKey, in.ConsumerKey)
		if err != nil {
			http.Error(w, "encrypt consumerKey failed", http.StatusInternalServerError)
			return
		}
		csEnc, err := crypto.EncryptString(cfg.Sec.AESKey, in.ConsumerSecret)
		if err != nil {
			http.Error(w, "encrypt consumerSecret failed", http.StatusInternalServerError)
			return
		}

		// --- Webhook token for token-based webhook route ---
		webhookTok := mustRandHex(24)

		// --- Insert provider credential (with C2B rules) ---
		cred, err := repo.InsertProviderCredential(r.Context(), postgres.ProviderCredential{
			TenantID:          tenant.ID,
			Provider:          in.Provider,
			Shortcode:         strings.TrimSpace(in.Shortcode),
			Environment:       in.Environment,
			WebhookToken:      webhookTok,
			IsActive:          true,
			PasskeyEnc:        passEnc,
			ConsumerKeyEnc:    ckEnc,
			ConsumerSecretEnc: csEnc,
			C2BMode:           in.C2BMode,
			BillRefRequired:   billRefRequired,
			BillRefRegex:      strings.TrimSpace(in.BillRefRegex),
		})
		if err != nil {
			http.Error(w, "save credential failed: "+err.Error(), http.StatusBadRequest)
			return
		}

		// --- Response (return plaintext API key ONCE) ---
		out := onboardTenantResp{
			TenantID:        tenant.ID,
			APIKey:          apiKey, // show once!
			APIKeyName:      keyName,
			WebhookToken:    cred.WebhookToken,
			Shortcode:       cred.Shortcode,
			Environment:     cred.Environment,
			C2BMode:         cred.C2BMode,
			BillRefRequired: cred.BillRefRequired,
			BillRefRegex:    cred.BillRefRegex,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// Helpers
func mustRandHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
