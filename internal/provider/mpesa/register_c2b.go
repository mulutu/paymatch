package mpesa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"paymatch/internal/store/postgres"
)

type RegisterC2BReq struct {
	ShortCode       string `json:"ShortCode"`
	ResponseType    string `json:"ResponseType"` // "Completed" or "Cancelled"
	ConfirmationURL string `json:"ConfirmationURL"`
	ValidationURL   string `json:"ValidationURL"`
}

func (p *Provider) RegisterC2B(ctx context.Context, cred postgres.ProviderCredential, confirmURL, validateURL, responseType string) error {
	token, err := p.token(ctx, cred)
	if err != nil {
		return err
	}

	payload := RegisterC2BReq{
		ShortCode:       cred.Shortcode,
		ResponseType:    responseType,
		ConfirmationURL: confirmURL,
		ValidationURL:   validateURL,
	}
	b, _ := json.Marshal(payload)

	url := baseURL(cred.Environment) + "/mpesa/c2b/v1/registerurl"
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("register c2b failed: %s", res.Status)
	}
	return nil
}
