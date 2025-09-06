package mpesa

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
	if env == "production" {
		return "https://api.safaricom.co.ke"
	}
	return "https://sandbox.safaricom.co.ke"
}

func (p *Provider) token(ctx context.Context, cred postgres.ProviderCredential) (string, error) {
	url := baseURL(cred.Environment) + "/oauth/v1/generate?grant_type=client_credentials"
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	ck, cs, err := p.repo.DecryptConsumer(ctx, cred)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(ck, cs)
	res, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("auth failed: %s; body=%s", res.Status, string(b))
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
	}
	if err := json.NewDecoder(res.Body).Decode(&t); err != nil {
		return "", err
	}
	return t.AccessToken, nil
}

func (p *Provider) STKPush(ctx context.Context, cred postgres.ProviderCredential, r provider.STKPushReq) (*provider.STKPushResp, error) {
	token, err := p.token(ctx, cred)
	if err != nil {
		return nil, err
	}

	// Timestamp in EAT
	ts := time.Now().In(time.FixedZone("EAT", 3*3600)).Format("20060102150405")

	passkey, err := p.repo.DecryptPasskey(ctx, cred)
	if err != nil {
		return nil, err
	}

	pwd := base64.StdEncoding.EncodeToString([]byte(cred.Shortcode + passkey + ts))

	payload := map[string]any{
		"BusinessShortCode": cred.Shortcode,
		"Password":          pwd,
		"Timestamp":         ts,
		"TransactionType":   "CustomerPayBillOnline", // for 174379 in sandbox
		"Amount":            r.Amount,
		"PartyA":            r.Phone,
		"PartyB":            cred.Shortcode,
		"PhoneNumber":       r.Phone,
		// Use shortcode webhook route (no token header required by Daraja)
		"CallBackURL":      p.cfg.App.CallbackBaseURL + "/hooks/mpesa/" + cred.Shortcode,
		"AccountReference": r.AccountRef,
		"TransactionDesc":  r.Description,
	}

	b, _ := json.Marshal(payload)
	url := baseURL(cred.Environment) + "/mpesa/stkpush/v1/processrequest"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		// >>> show the REAL reason from Safaricom
		b2, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("stk failed: %s; body=%s", res.Status, string(b2))
	}

	var out struct {
		CheckoutRequestID string `json:"CheckoutRequestID"`
		CustomerMessage   string `json:"CustomerMessage"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &provider.STKPushResp{
		CheckoutRequestID: out.CheckoutRequestID,
		CustomerMessage:   out.CustomerMessage,
	}, nil
}
