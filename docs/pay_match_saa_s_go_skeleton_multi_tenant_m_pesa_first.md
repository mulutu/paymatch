# PayMatch (Multi-tenant SaaS) — Go Code Skeleton

This is a production-ready skeleton to launch **PayMatch** as a multi-tenant M-Pesa aggregator (with room for more providers later). It bootstraps tenancy, API-key auth, provider abstraction, webhook routing, and STK push flow.

> Go version: **1.22**

---

## 0) Repository tree
```
paymatch/
  .env.example
  .gitignore
  Makefile
  README.md
  docker-compose.yml
  go.mod
  cmd/
    api/
      main.go
  internal/
    config/
      config.go
    core/
      models.go
      reconcile/
        reconciler.go
    crypto/
      crypto.go
    http/
      router.go
      middleware/
        auth.go
        context.go
      handlers/
        health.go
        stk.go
        webhooks.go
    provider/
      provider.go
      mpesa/
        mpesa.go
        webhook.go
    rate/
      limiter.go
    store/
      postgres/
        db.go
        repo.go
        migrations/
          001_init.sql
          002_usage.sql
```

---

## 1) go.mod
```go
module paymatch

go 1.22

require (
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/go-chi/chi/v5 v5.1.0
	github.com/jackc/pgx/v5 v5.6.0
	github.com/redis/go-redis/v9 v9.5.3
	github.com/rs/zerolog v1.33.0
	github.com/spf13/viper v1.19.0
)
```

---

## 2) docker-compose.yml (Postgres + Redis for local dev)
```yaml
version: '3.9'
services:
  db:
    image: postgres:16
    environment:
      POSTGRES_USER: paymatch
      POSTGRES_PASSWORD: paymatch
      POSTGRES_DB: paymatch
    ports: ["5432:5432"]
    volumes:
      - pgdata:/var/lib/postgresql/data
  redis:
    image: redis:7
    ports: ["6379:6379"]
volumes:
  pgdata:
```

---

## 3) .env.example
```env
APP_ENV=sandbox
APP_PORT=8080
APP_BASE_URL=http://localhost:8080
CALLBACK_BASE_URL=http://localhost:8080
DB_DSN=postgres://paymatch:paymatch@localhost:5432/paymatch?sslmode=disable
REDIS_ADDR=localhost:6379
AES_256_KEY_BASE64=REPLACE_WITH_32_BYTE_KEY_BASE64
RATE_LIMIT_PER_MIN=300
TZ=Africa/Nairobi
```

---

## 4) Makefile
```make
run:
	go run ./cmd/api

tidy:
	go mod tidy

migrate:
	psql "$$DB_DSN" -f internal/store/postgres/migrations/001_init.sql || true
	psql "$$DB_DSN" -f internal/store/postgres/migrations/002_usage.sql || true
```

---

## 5) cmd/api/main.go
```go
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"paymatch/internal/config"
	"paymatch/internal/http"
	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"

	"github.com/rs/zerolog/log"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	// Init DB
	pool := postgres.MustOpen(ctx, cfg.DB.DSN)
	defer pool.Close()
	repo := postgres.NewRepo(pool, cfg)

	// Init providers registry (only M-Pesa for MVP)
	mp := mpesa.New(cfg, repo)

	// Router
	r := httpx.NewRouter(cfg, repo, mp)

	srv := &http.Server{
		Addr:         ":" + cfg.App.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Info().Msgf("PayMatch API listening on :%s", cfg.App.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Info().Msg("server stopped")
}
```

---

## 6) internal/config/config.go
```go
package config

import (
	"encoding/base64"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type AppCfg struct{ Env, Port, BaseURL, CallbackBaseURL string }

type DBCfg struct{ DSN string }

type RedisCfg struct{ Addr string }

type SecurityCfg struct{ AESKey []byte; RateLimitPerMin int }

type Cfg struct{
	App   AppCfg
	DB    DBCfg
	Redis RedisCfg
	Sec   SecurityCfg
}

func Load() Cfg {
	viper.AutomaticEnv()
	viper.SetDefault("APP_ENV", "sandbox")
	viper.SetDefault("APP_PORT", "8080")
	viper.SetDefault("RATE_LIMIT_PER_MIN", 300)
	viper.SetDefault("TZ", "Africa/Nairobi")

	// Ensure TZ
	if tz := viper.GetString("TZ"); tz != "" { os.Setenv("TZ", tz) }

	keyB64 := viper.GetString("AES_256_KEY_BASE64")
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != 32 {
		log.Warn().Msg("AES_256_KEY_BASE64 invalid or missing; using zero key (dev only)")
		key = make([]byte, 32)
	}

	cfg := Cfg{
		App: AppCfg{
			Env: viper.GetString("APP_ENV"),
			Port: viper.GetString("APP_PORT"),
			BaseURL: viper.GetString("APP_BASE_URL"),
			CallbackBaseURL: viper.GetString("CALLBACK_BASE_URL"),
		},
		DB:    DBCfg{DSN: viper.GetString("DB_DSN")},
		Redis: RedisCfg{Addr: viper.GetString("REDIS_ADDR")},
		Sec:   SecurityCfg{AESKey: key, RateLimitPerMin: viper.GetInt("RATE_LIMIT_PER_MIN")},
	}

	_ = time.Local // TZ already set via env
	return cfg
}
```

---

## 7) internal/http/router.go
```go
package httpx

import (
	"net/http"

	"paymatch/internal/config"
	"paymatch/internal/http/handlers"
	"paymatch/internal/http/middleware"
	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(cfg config.Cfg, repo *postgres.Repo, mp *mpesa.Provider) http.Handler {
	r := chi.NewRouter()
	r.Use(middlewarex.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health
	r.Get("/healthz", handlers.Health)

	// Public webhooks (no tenant auth). Two routing modes.
	r.Post("/hooks/mpesa/{shortcode}", handlers.MpesaWebhookByShortcode(repo, mp))
	r.Post("/hooks/mpesa", handlers.MpesaWebhookByToken(repo, mp))

	// Tenant APIs (API-key auth)
	r.Group(func(pr chi.Router) {
		pr.Use(middlewarex.APIKeyAuth(repo))
		pr.Post("/v1/payments/stk", handlers.CreateSTK(repo, mp))
		// add more tenant endpoints here
	})

	return r
}
```

---

## 8) internal/http/middleware/context.go
```go
package middlewarex

import "context"

type ctxKey string

const (
	ctxTenantID ctxKey = "tenant_id"
)

func WithTenantID(ctx context.Context, tenantID int64) context.Context {
	return context.WithValue(ctx, ctxTenantID, tenantID)
}

func TenantID(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(ctxTenantID).(int64)
	return v, ok
}
```

---

## 9) internal/http/middleware/auth.go
```go
package middlewarex

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"paymatch/internal/store/postgres"
)

func APIKeyAuth(repo *postgres.Repo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "missing bearer", http.StatusUnauthorized); return
			}
			key := strings.TrimPrefix(auth, "Bearer ")
			h := sha256.Sum256([]byte(key))
			hx := hex.EncodeToString(h[:])

			ten, err := repo.LookupTenantByAPIKeyHash(r.Context(), hx)
			if err != nil { http.Error(w, "invalid key", http.StatusUnauthorized); return }

			next.ServeHTTP(w, r.WithContext(WithTenantID(r.Context(), ten.ID)))
		})
	}
}
```

---

## 10) internal/http/handlers/health.go
```go
package handlers

import (
	"net/http"
)

func Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("ok"))
}
```

---

## 11) internal/http/handlers/stk.go
```go
package handlers

import (
	"encoding/json"
	"net/http"

	"paymatch/internal/http/middleware"
	"paymatch/internal/provider"
	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"
)

type stkReq struct {
	Amount     int    `json:"amount"`
	Phone      string `json:"phone"`
	AccountRef string `json:"accountRef"`
	Description string `json:"description"`
	Shortcode  string `json:"shortcode,omitempty"`
}

type stkResp struct {
	CheckoutRequestID string `json:"checkoutRequestId"`
	CustomerMessage   string `json:"customerMessage"`
}

func CreateSTK(repo *postgres.Repo, mp *mpesa.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok { http.Error(w, "tenant not found", 401); return }

		var in stkReq
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad json", 400); return
		}

		cred, err := repo.ResolveCredential(r.Context(), tenantID, in.Shortcode)
		if err != nil { http.Error(w, "credential not found", 404); return }

		out, err := mp.STKPush(r.Context(), cred, provider.STKPushReq{
			Amount: in.Amount, Phone: in.Phone, AccountRef: in.AccountRef, Description: in.Description,
		})
		if err != nil { http.Error(w, "stk failed", 502); return }

		json.NewEncoder(w).Encode(stkResp{CheckoutRequestID: out.CheckoutRequestID, CustomerMessage: out.CustomerMessage})
	}
}
```

---

## 12) internal/http/handlers/webhooks.go
```go
package handlers

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"paymatch/internal/provider/mpesa"
	"paymatch/internal/store/postgres"
)

func MpesaWebhookByShortcode(repo *postgres.Repo, mp *mpesa.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shortcode := chi.URLParam(r, "shortcode")
		cred, tenant, err := repo.FindCredentialByShortcode(r.Context(), shortcode)
		if err != nil { http.Error(w, "unknown shortcode", 404); return }
		body, _ := io.ReadAll(r.Body)
		evt, err := mp.ParseWebhook(body)
		if err != nil { http.Error(w, "bad payload", 400); return }

		if err := repo.SaveEvent(r.Context(), tenant.ID, cred.ID, evt); err != nil {
			w.WriteHeader(200); w.Write([]byte(`{"status":"duplicate"}`)); return
		}
		w.WriteHeader(200); w.Write([]byte(`{"status":"ok"}`))
	}
}

func MpesaWebhookByToken(repo *postgres.Repo, mp *mpesa.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-PM-Webhook-Token")
		cred, tenant, err := repo.FindCredentialByWebhookToken(r.Context(), tok)
		if err != nil { http.Error(w, "unknown token", 404); return }
		body, _ := io.ReadAll(r.Body)
		evt, err := mp.ParseWebhook(body)
		if err != nil { http.Error(w, "bad payload", 400); return }
		if err := repo.SaveEvent(r.Context(), tenant.ID, cred.ID, evt); err != nil {
			w.WriteHeader(200); w.Write([]byte(`{"status":"duplicate"}`)); return
		}
		w.WriteHeader(200); w.Write([]byte(`{"status":"ok"}`))
	}
}
```

---

## 13) internal/provider/provider.go
```go
package provider

type STKPushReq struct {
	Amount      int
	Phone       string
	AccountRef  string
	Description string
}

type STKPushResp struct {
	CheckoutRequestID string
	CustomerMessage   string
}

type EventType string

const (
	EventSTK EventType = "stk"
	EventC2B EventType = "c2b"
)

type Event struct {
	Type       EventType
	ExternalID string
	Amount     int
	MSISDN     string
	InvoiceRef string
	RawJSON    []byte
}
```

---

## 14) internal/provider/mpesa/mpesa.go
```go
package mpesa

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"paymatch/internal/config"
	"paymatch/internal/provider"
	"paymatch/internal/store/postgres"
)

type Provider struct {
	cfg  config.Cfg
	repo *postgres.Repo
	http *http.Client
}

func New(cfg config.Cfg, repo *postgres.Repo) *Provider {
	return &Provider{cfg: cfg, repo: repo, http: &http.Client{Timeout: 15 * time.Second}}
}

func baseURL(env string) string {
	if env == "production" { return "https://api.safaricom.co.ke" }
	return "https://sandbox.safaricom.co.ke"
}

func (p *Provider) token(ctx context.Context, cred postgres.ProviderCredential) (string, error) {
	url := baseURL(cred.Environment) + "/oauth/v1/generate?grant_type=client_credentials"
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	ck, cs, err := p.repo.DecryptConsumer(ctx, cred)
	if err != nil { return "", err }
	req.SetBasicAuth(ck, cs)
	res, err := p.http.Do(req)
	if err != nil { return "", err }
	defer res.Body.Close()
	if res.StatusCode != 200 { return "", fmt.Errorf("auth failed: %s", res.Status) }
	var t struct{ AccessToken string `json:"access_token"`; ExpiresIn string `json:"expires_in"` }
	if err := json.NewDecoder(res.Body).Decode(&t); err != nil { return "", err }
	return t.AccessToken, nil
}

func (p *Provider) STKPush(ctx context.Context, cred postgres.ProviderCredential, r provider.STKPushReq) (*provider.STKPushResp, error) {
	token, err := p.token(ctx, cred)
	if err != nil { return nil, err }
	// Timestamp in EAT
	ts := time.Now().In(time.FixedZone("EAT", 3*3600)).Format("20060102150405")
	passkey, err := p.repo.DecryptPasskey(ctx, cred)
	if err != nil { return nil, err }
	pwd := base64.StdEncoding.EncodeToString([]byte(cred.Shortcode + passkey + ts))
	payload := map[string]any{
		"BusinessShortCode": cred.Shortcode,
		"Password": pwd,
		"Timestamp": ts,
		"TransactionType": "CustomerPayBillOnline",
		"Amount": r.Amount,
		"PartyA": r.Phone,
		"PartyB": cred.Shortcode,
		"PhoneNumber": r.Phone,
		"CallBackURL": p.cfg.App.CallbackBaseURL + "/hooks/mpesa", // token route preferred
		"AccountReference": r.AccountRef,
		"TransactionDesc": r.Description,
	}
	b, _ := json.Marshal(payload)
	url := baseURL(cred.Environment) + "/mpesa/stkpush/v1/processrequest"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := p.http.Do(req)
	if err != nil { return nil, err }
	defer res.Body.Close()
	if res.StatusCode != 200 { return nil, fmt.Errorf("stk failed: %s", res.Status) }
	var out struct{
		CheckoutRequestID string `json:"CheckoutRequestID"`
		CustomerMessage   string `json:"CustomerMessage"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil { return nil, err }
	return &provider.STKPushResp{CheckoutRequestID: out.CheckoutRequestID, CustomerMessage: out.CustomerMessage}, nil
}
```

---

## 15) internal/provider/mpesa/webhook.go
```go
package mpesa

import (
	"encoding/json"
	"paymatch/internal/provider"
)

// ParseWebhook converts Daraja callback payloads (STK/C2B) into generic provider.Event
func (p *Provider) ParseWebhook(body []byte) (provider.Event, error) {
	// Try STK callback shape first
	var stk struct{
		Body struct{
			StkCallback struct{
				CheckoutRequestID string `json:"CheckoutRequestID"`
				ResultCode int `json:"ResultCode"`
				ResultDesc string `json:"ResultDesc"`
				CallbackMetadata struct{ Item []struct{ Name string `json:"Name"`; Value any `json:"Value"` } `json:"Item"` } `json:"CallbackMetadata"`
			} `json:"stkCallback"`
		} `json:"Body"`
	}
	if err := json.Unmarshal(body, &stk); err == nil && stk.Body.StkCallback.CheckoutRequestID != "" {
		// Extract minimal fields
		var amount int; var msisdn, ref string
		for _, it := range stk.Body.StkCallback.CallbackMetadata.Item {
			switch it.Name {
			case "Amount": if f, ok := it.Value.(float64); ok { amount = int(f) }
			case "MpesaReceiptNumber": // ignored here
			case "PhoneNumber": if f, ok := it.Value.(float64); ok { msisdn = fmtInt(f) } else if s, ok := it.Value.(string); ok { msisdn = s }
			case "AccountReference": if s, ok := it.Value.(string); ok { ref = s }
			}
		}
		return provider.Event{Type: provider.EventSTK, ExternalID: stk.Body.StkCallback.CheckoutRequestID, Amount: amount, MSISDN: msisdn, InvoiceRef: ref, RawJSON: body}, nil
	}

	// Try C2B confirmation shape
	var c2b map[string]any
	if err := json.Unmarshal(body, &c2b); err == nil {
		if tx, ok := c2b["TransID"].(string); ok && tx != "" {
			amt := 0
			if f, ok := c2b["TransAmount"].(float64); ok { amt = int(f) }
			ms := ""; if s, ok := c2b["MSISDN"].(string); ok { ms = s }
			ref := ""; if s, ok := c2b["BillRefNumber"].(string); ok { ref = s }
			return provider.Event{Type: provider.EventC2B, ExternalID: tx, Amount: amt, MSISDN: ms, InvoiceRef: ref, RawJSON: body}, nil
		}
	}
	return provider.Event{}, fmt.Errorf("unrecognized webhook shape")
}

func fmtInt(f float64) string { return fmt.Sprintf("%.0f", f) }
```

---

## 16) internal/store/postgres/db.go
```go
package postgres

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

func MustOpen(ctx context.Context, dsn string) *pgxpool.Pool {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil { log.Fatal().Err(err).Msg("db connect fail") }
	if err := pool.Ping(ctx); err != nil { log.Fatal().Err(err).Msg("db ping fail") }
	return pool
}
```

---

## 17) internal/store/postgres/repo.go
```go
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"paymatch/internal/config"
	"paymatch/internal/crypto"
	"paymatch/internal/provider"
)

type Repo struct { db *pgxpool.Pool; cfg config.Cfg }

func NewRepo(db *pgxpool.Pool, cfg config.Cfg) *Repo { return &Repo{db: db, cfg: cfg} }

// TENANTS & KEYS

type Tenant struct { ID int64; Name string; Status string }

type TenantAPIKey struct { ID int64; TenantID int64; KeyHash string }

type ProviderCredential struct {
	ID int64; TenantID int64; Provider string; Shortcode string; Environment string; WebhookToken string; IsActive bool
	PasskeyEnc string; ConsumerKeyEnc string; ConsumerSecretEnc string
}

func (r *Repo) LookupTenantByAPIKeyHash(ctx context.Context, keyHash string) (Tenant, error) {
	row := r.db.QueryRow(ctx, `SELECT t.id, t.name, t.status FROM tenant_api_keys k JOIN tenants t ON t.id=k.tenant_id WHERE k.key_hash=$1`, keyHash)
	var t Tenant
	if err := row.Scan(&t.ID, &t.Name, &t.Status); err != nil { return Tenant{}, err }
	return t, nil
}

// CREDENTIAL RESOLUTION
func (r *Repo) ResolveCredential(ctx context.Context, tenantID int64, shortcode string) (ProviderCredential, error) {
	if shortcode == "" {
		row := r.db.QueryRow(ctx, `SELECT id, tenant_id, provider, shortcode, environment, webhook_token, is_active, passkey_enc, consumer_key_enc, consumer_secret_enc
			FROM provider_credentials WHERE tenant_id=$1 AND is_active=true ORDER BY id LIMIT 1`, tenantID)
		var c ProviderCredential; if err := row.Scan(&c.ID, &c.TenantID, &c.Provider, &c.Shortcode, &c.Environment, &c.WebhookToken, &c.IsActive, &c.PasskeyEnc, &c.ConsumerKeyEnc, &c.ConsumerSecretEnc); err != nil { return ProviderCredential{}, err }
		return c, nil
	}
	row := r.db.QueryRow(ctx, `SELECT id, tenant_id, provider, shortcode, environment, webhook_token, is_active, passkey_enc, consumer_key_enc, consumer_secret_enc FROM provider_credentials WHERE tenant_id=$1 AND shortcode=$2 AND is_active=true`, tenantID, shortcode)
	var c ProviderCredential; if err := row.Scan(&c.ID, &c.TenantID, &c.Provider, &c.Shortcode, &c.Environment, &c.WebhookToken, &c.IsActive, &c.PasskeyEnc, &c.ConsumerKeyEnc, &c.ConsumerSecretEnc); err != nil { return ProviderCredential{}, err }
	return c, nil
}

func (r *Repo) FindCredentialByShortcode(ctx context.Context, shortcode string) (ProviderCredential, Tenant, error) {
	row := r.db.QueryRow(ctx, `SELECT c.id,c.tenant_id,c.provider,c.shortcode,c.environment,c.webhook_token,c.is_active,c.passkey_enc,c.consumer_key_enc,c.consumer_secret_enc, t.id,t.name,t.status
		FROM provider_credentials c JOIN tenants t ON t.id=c.tenant_id WHERE c.shortcode=$1 AND c.is_active=true`, shortcode)
	var c ProviderCredential; var t Tenant
	if err := row.Scan(&c.ID,&c.TenantID,&c.Provider,&c.Shortcode,&c.Environment,&c.WebhookToken,&c.IsActive,&c.PasskeyEnc,&c.ConsumerKeyEnc,&c.ConsumerSecretEnc,&t.ID,&t.Name,&t.Status); err != nil { return ProviderCredential{}, Tenant{}, err }
	return c, t, nil
}

func (r *Repo) FindCredentialByWebhookToken(ctx context.Context, token string) (ProviderCredential, Tenant, error) {
	row := r.db.QueryRow(ctx, `SELECT c.id,c.tenant_id,c.provider,c.shortcode,c.environment,c.webhook_token,c.is_active,c.passkey_enc,c.consumer_key_enc,c.consumer_secret_enc, t.id,t.name,t.status
		FROM provider_credentials c JOIN tenants t ON t.id=c.tenant_id WHERE c.webhook_token=$1 AND c.is_active=true`, token)
	var c ProviderCredential; var t Tenant
	if err := row.Scan(&c.ID,&c.TenantID,&c.Provider,&c.Shortcode,&c.Environment,&c.WebhookToken,&c.IsActive,&c.PasskeyEnc,&c.ConsumerKeyEnc,&c.ConsumerSecretEnc,&t.ID,&t.Name,&t.Status); err != nil { return ProviderCredential{}, Tenant{}, err }
	return c, t, nil
}

// SECRETS DECRYPTION
func (r *Repo) DecryptPasskey(ctx context.Context, c ProviderCredential) (string, error) { return crypto.DecryptString(r.cfg.Sec.AESKey, c.PasskeyEnc) }
func (r *Repo) DecryptConsumer(ctx context.Context, c ProviderCredential) (string, string, error) {
	ck, err := crypto.DecryptString(r.cfg.Sec.AESKey, c.ConsumerKeyEnc); if err != nil { return "", "", err }
	cs, err := crypto.DecryptString(r.cfg.Sec.AESKey, c.ConsumerSecretEnc); if err != nil { return "", "", err }
	return ck, cs, nil
}

// EVENTS (idempotent save)
func (r *Repo) SaveEvent(ctx context.Context, tenantID, credID int64, evt provider.Event) error {
	_, err := r.db.Exec(ctx, `INSERT INTO payment_events (tenant_id, provider_credential_id, event_type, external_id, payload_json)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (tenant_id, event_type, external_id) DO NOTHING`, tenantID, credID, string(evt.Type), evt.ExternalID, evt.RawJSON)
	return err
}

// Helper to pre-hash API keys for seeding
func HashAPIKey(key string) string { h := sha256.Sum256([]byte(key)); return hex.EncodeToString(h[:]) }
```

---

## 18) internal/crypto/crypto.go
```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

func EncryptString(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key); if err != nil { return "", err }
	gcm, err := cipher.NewGCM(block); if err != nil { return "", err }
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return "", err }
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func DecryptString(key []byte, b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64); if err != nil { return "", err }
	block, err := aes.NewCipher(key); if err != nil { return "", err }
	gcm, err := cipher.NewGCM(block); if err != nil { return "", err }
	if len(raw) < gcm.NonceSize() { return "", errors.New("ciphertext too short") }
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil); if err != nil { return "", err }
	return string(pt), nil
}
```

---

## 19) internal/core/models.go
```go
package core

type PaymentStatus string
const (
	PaymentPending PaymentStatus = "pending"
	PaymentMatched PaymentStatus = "matched"
	PaymentFailed  PaymentStatus = "failed"
)
```

---

## 20) internal/core/reconcile/reconciler.go (stub for async workers)
```go
package reconcile

// TODO: add background worker to consume payment_events and upsert payments
// with matching rules (exact invoice + amount, then fuzzy match, then pending_review).
```

---

## 21) internal/rate/limiter.go (placeholder)
```go
package rate

// TODO: redis-backed token bucket per API key (cfg.Sec.RateLimitPerMin)
```

---

## 22) internal/store/postgres/migrations/001_init.sql
```sql
-- Tenants & keys
CREATE TABLE IF NOT EXISTS tenants (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tenant_api_keys (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  key_hash TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ
);

-- Provider credentials
CREATE TABLE IF NOT EXISTS provider_credentials (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  provider TEXT NOT NULL,                        -- 'mpesa_daraja'
  shortcode TEXT NOT NULL,
  passkey_enc TEXT NOT NULL,
  consumer_key_enc TEXT NOT NULL,
  consumer_secret_enc TEXT NOT NULL,
  environment TEXT NOT NULL,                    -- 'sandbox'|'production'
  webhook_token TEXT UNIQUE NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_provider_credentials_shortcode ON provider_credentials(shortcode);
CREATE INDEX IF NOT EXISTS idx_provider_credentials_tenant ON provider_credentials(tenant_id);

-- Payments & events
CREATE TABLE IF NOT EXISTS payment_events (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  provider_credential_id BIGINT NOT NULL REFERENCES provider_credentials(id),
  event_type TEXT NOT NULL,
  external_id TEXT NOT NULL,
  payload_json JSONB NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  processed_at TIMESTAMPTZ,
  status TEXT,
  UNIQUE (tenant_id, event_type, external_id)
);

CREATE TABLE IF NOT EXISTS payments (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  invoice_no TEXT,
  msisdn_hash TEXT,
  amount INT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'KES',
  status TEXT NOT NULL DEFAULT 'pending',
  method TEXT NOT NULL DEFAULT 'mpesa',
  provider_credential_id BIGINT REFERENCES provider_credentials(id),
  external_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_payments_tenant_invoice ON payments(tenant_id, invoice_no);
CREATE INDEX IF NOT EXISTS idx_payments_tenant_created ON payments(tenant_id, created_at);
```

---

## 23) internal/store/postgres/migrations/002_usage.sql
```sql
CREATE TABLE IF NOT EXISTS usage_counters (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  period_ym TEXT NOT NULL,                       -- e.g. '2025-09'
  api_calls INT NOT NULL DEFAULT 0,
  events_ingested INT NOT NULL DEFAULT 0,
  reconciled_count INT NOT NULL DEFAULT 0,
  UNIQUE (tenant_id, period_ym)
);
```

---

## 24) README.md (quick start)
```md
# PayMatch (SaaS) — Quick start

1. **Start infra**
   ```bash
   docker compose up -d
   ```
2. **Set env**
   ```bash
   cp .env.example .env
   export $(cat .env | xargs)
   ```
3. **Migrate DB**
   ```bash
   make migrate
   ```
4. **Seed a tenant & API key (psql)**
   ```sql
   INSERT INTO tenants(name) VALUES ('DemoCo') RETURNING id; -- note the id
   -- generate an API key string and hash with repo helper (or any sha256 tool)
   INSERT INTO tenant_api_keys(tenant_id, key_hash, name) VALUES (<TENANT_ID>, '<SHA256_HEX>', 'default');
   -- encrypt Daraja creds using AES_256_KEY_BASE64 (use a tiny Go tool or psql function)
   INSERT INTO provider_credentials(tenant_id, provider, shortcode, passkey_enc, consumer_key_enc, consumer_secret_enc, environment, webhook_token)
   VALUES (<TENANT_ID>, 'mpesa_daraja', '174379', '<ENC_PASSKEY>', '<ENC_CONSUMER_KEY>', '<ENC_CONSUMER_SECRET>', 'sandbox', '<RANDOM_TOKEN>');
   ```
5. **Run API**
   ```bash
   make run
   ```
6. **Test STK**
   ```bash
   curl -X POST http://localhost:8080/v1/payments/stk \
     -H "Authorization: Bearer <YOUR_API_KEY>" \
     -H "Content-Type: application/json" \
     -d '{"amount":1,"phone":"2547XXXXXXXX","accountRef":"INV-1001","description":"Test"}'
   ```
7. **Expose webhooks** (ngrok etc.) and set callback URLs in Daraja.
```

---

### Notes
- Secrets are locally encrypted with AES-256 (replace with KMS in prod).
- Webhook routing supports **/{shortcode}** and **token header**.
- Add a background worker to process `payment_events` → `payments` reconciliation.
- All timestamps default to **Africa/Nairobi** via ENV TZ.

