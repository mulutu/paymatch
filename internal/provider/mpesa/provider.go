package mpesa

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"paymatch/internal/config"
	"paymatch/internal/domain/credential"
	"paymatch/internal/provider"
	"paymatch/internal/provider/base"

	"github.com/rs/zerolog/log"
)

// Provider implements the M-Pesa Daraja API provider
type Provider struct {
	cfg        config.Cfg
	httpClient *base.HTTPClient
	validator  *base.RequestValidator
	tokenCache map[string]*accessToken
}

// accessToken represents cached M-Pesa access token
type accessToken struct {
	Token     string
	ExpiresAt time.Time
}

// New creates a new M-Pesa provider instance
func New(cfg config.Cfg) provider.Provider {
	httpClient := base.NewHTTPClient("mpesa", 30)                // 30 second timeout
	validator := base.NewRequestValidator("KE", "KES", 1, 70000) // Kenya limits

	return &Provider{
		cfg:        cfg,
		httpClient: httpClient,
		validator:  validator,
		tokenCache: make(map[string]*accessToken),
	}
}

// Name returns the provider name
func (p *Provider) Name() string {
	return "M-Pesa (Safaricom Daraja)"
}

// SupportedOperations returns operations supported by M-Pesa
func (p *Provider) SupportedOperations() []provider.OperationType {
	return []provider.OperationType{
		provider.OpSTKPush,
		provider.OpC2B,
		provider.OpB2C,
		provider.OpBalance,
		provider.OpStatus,
		// M-Pesa doesn't support bulk transfers natively
	}
}

// RequiredCredentialFields returns required credential fields for M-Pesa
func (p *Provider) RequiredCredentialFields() []provider.CredentialField {
	return []provider.CredentialField{
		{
			Name:        "shortcode",
			DisplayName: "Business Shortcode",
			Type:        "text",
			Required:    true,
		},
		{
			Name:        "consumer_key",
			DisplayName: "Consumer Key",
			Type:        "password",
			Required:    true,
		},
		{
			Name:        "consumer_secret",
			DisplayName: "Consumer Secret",
			Type:        "password",
			Required:    true,
		},
		{
			Name:        "passkey",
			DisplayName: "LipaNaMpesa Passkey",
			Type:        "password",
			Required:    true,
		},
		{
			Name:        "environment",
			DisplayName: "Environment",
			Type:        "select",
			Required:    true,
			Options:     []string{"sandbox", "production"},
		},
		{
			Name:        "webhook_token",
			DisplayName: "Webhook Token",
			Type:        "text",
			Required:    true,
		},
	}
}

// STKPush initiates STK push payment
func (p *Provider) STKPush(ctx context.Context, cred *credential.ProviderCredential, req provider.STKPushReq) (*provider.STKPushResp, error) {
	// Validate request
	if err := p.validator.ValidateSTKPushReq(&req); err != nil {
		return nil, err
	}

	// Get access token
	token, err := p.getAccessToken(ctx, cred)
	if err != nil {
		return nil, &provider.ProviderError{
			Code:    "auth_failed",
			Message: fmt.Sprintf("failed to get access token: %v", err),
		}
	}

	// Generate password and timestamp
	timestamp := time.Now().Format("20060102150405")
	password := base64.StdEncoding.EncodeToString([]byte(cred.Shortcode + cred.GetDecryptedField("passkey", p.cfg.Sec.AESKey) + timestamp))

	// Generate transaction ID
	transactionID, err := p.generateTransactionID()
	if err != nil {
		return nil, &provider.ProviderError{
			Code:    "transaction_id_generation_failed",
			Message: fmt.Sprintf("failed to generate transaction ID: %v", err),
		}
	}

	// Build request payload
	payload := map[string]interface{}{
		"BusinessShortCode": cred.Shortcode,
		"Password":          password,
		"Timestamp":         timestamp,
		"TransactionType":   "CustomerPayBillOnline",
		"Amount":            req.Amount,
		"PartyA":            req.PhoneNumber,
		"PartyB":            cred.Shortcode,
		"PhoneNumber":       req.PhoneNumber,
		"CallBackURL":       req.CallbackURL,
		"AccountReference":  req.AccountReference,
		"TransactionDesc":   req.Description,
	}

	// Make request
	baseURL := p.getBaseURL(string(cred.Environment))
	url := baseURL + "/mpesa/stkpush/v1/processrequest"

	responseBody, err := p.makeAuthenticatedRequest(ctx, "POST", url, token, payload)
	if err != nil {
		return nil, err
	}

	// Parse response
	var response struct {
		MerchantRequestID   string `json:"MerchantRequestID"`
		CheckoutRequestID   string `json:"CheckoutRequestID"`
		ResponseCode        string `json:"ResponseCode"`
		ResponseDescription string `json:"ResponseDescription"`
		CustomerMessage     string `json:"CustomerMessage"`
		ErrorCode           string `json:"errorCode"`
		ErrorMessage        string `json:"errorMessage"`
	}

	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, &provider.ProviderError{
			Code:    "response_parse_failed",
			Message: fmt.Sprintf("failed to parse STK response: %v", err),
		}
	}

	// Check for errors
	if response.ErrorCode != "" {
		return nil, &provider.ProviderError{
			Code:    response.ErrorCode,
			Message: response.ErrorMessage,
		}
	}

	if response.ResponseCode != "0" {
		return nil, &provider.ProviderError{
			Code:    "stk_failed",
			Message: response.ResponseDescription,
		}
	}

	p.logOperation("stk_push", map[string]interface{}{
		"checkout_request_id": response.CheckoutRequestID,
		"amount":              req.Amount,
		"phone_number":        req.PhoneNumber,
		"shortcode":           cred.Shortcode,
	})

	return &provider.STKPushResp{
		ExternalID:        response.CheckoutRequestID,
		Status:            provider.StatusPending,
		Message:           response.CustomerMessage,
		TransactionID:     transactionID,
		ProviderReference: response.MerchantRequestID,
	}, nil
}

// B2C initiates business to customer transfer
func (p *Provider) B2C(ctx context.Context, cred *credential.ProviderCredential, req provider.B2CReq) (*provider.B2CResp, error) {
	// Validate request
	if err := p.validator.ValidateB2CReq(&req); err != nil {
		return nil, err
	}

	// Get access token
	token, err := p.getAccessToken(ctx, cred)
	if err != nil {
		return nil, &provider.ProviderError{
			Code:    "auth_failed",
			Message: fmt.Sprintf("failed to get access token: %v", err),
		}
	}

	// Generate originator conversation ID
	originatorID, err := p.generateConversationID()
	if err != nil {
		return nil, &provider.ProviderError{
			Code:    "conversation_id_generation_failed",
			Message: fmt.Sprintf("failed to generate conversation ID: %v", err),
		}
	}

	// Build request payload
	payload := map[string]interface{}{
		"InitiatorName":              "testapi", // This should be configurable
		"SecurityCredential":         p.getSecurityCredential(cred),
		"CommandID":                  "BusinessPayment",
		"Amount":                     req.Amount,
		"PartyA":                     cred.Shortcode,
		"PartyB":                     req.PhoneNumber,
		"Remarks":                    req.Description,
		"QueueTimeOutURL":            req.TimeoutURL,
		"ResultURL":                  req.ResultURL,
		"Occasion":                   req.Occasion,
		"OriginatorConversationID":   originatorID,
	}

	// Make request
	baseURL := p.getBaseURL(string(cred.Environment))
	url := baseURL + "/mpesa/b2c/v1/paymentrequest"

	responseBody, err := p.makeAuthenticatedRequest(ctx, "POST", url, token, payload)
	if err != nil {
		return nil, err
	}

	// Parse response
	var response struct {
		ConversationID           string `json:"ConversationID"`
		OriginatorConversationID string `json:"OriginatorConversationID"`
		ResponseCode             string `json:"ResponseCode"`
		ResponseDescription      string `json:"ResponseDescription"`
		ErrorCode                string `json:"errorCode"`
		ErrorMessage             string `json:"errorMessage"`
	}

	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, &provider.ProviderError{
			Code:    "response_parse_failed",
			Message: fmt.Sprintf("failed to parse B2C response: %v", err),
		}
	}

	// Check for errors
	if response.ErrorCode != "" {
		return nil, &provider.ProviderError{
			Code:    response.ErrorCode,
			Message: response.ErrorMessage,
		}
	}

	if response.ResponseCode != "0" {
		return nil, &provider.ProviderError{
			Code:    "b2c_failed",
			Message: response.ResponseDescription,
		}
	}

	p.logOperation("b2c_transfer", map[string]interface{}{
		"conversation_id": response.ConversationID,
		"amount":          req.Amount,
		"phone_number":    req.PhoneNumber,
		"shortcode":       cred.Shortcode,
	})

	return &provider.B2CResp{
		ExternalID:        response.ConversationID,
		Status:            provider.StatusPending,
		Message:           response.ResponseDescription,
		ProviderReference: response.OriginatorConversationID,
	}, nil
}

// BulkTransfer is not supported by M-Pesa natively
func (p *Provider) BulkTransfer(ctx context.Context, cred *credential.ProviderCredential, req provider.BulkTransferReq) (*provider.BulkTransferResp, error) {
	return nil, &provider.ProviderError{
		Code:    "operation_not_supported",
		Message: "M-Pesa does not support bulk transfers natively",
	}
}

// CheckBalance checks account balance
func (p *Provider) CheckBalance(ctx context.Context, cred *credential.ProviderCredential) (*provider.BalanceResp, error) {
	// Get access token
	token, err := p.getAccessToken(ctx, cred)
	if err != nil {
		return nil, &provider.ProviderError{
			Code:    "auth_failed",
			Message: fmt.Sprintf("failed to get access token: %v", err),
		}
	}

	// Build request payload
	payload := map[string]interface{}{
		"Initiator":              "testapi", // This should be configurable
		"SecurityCredential":     p.getSecurityCredential(cred),
		"CommandID":              "AccountBalance",
		"PartyA":                 cred.Shortcode,
		"IdentifierType":         "4",
		"Remarks":                "Balance inquiry",
		"QueueTimeOutURL":        p.cfg.App.BaseURL + "/webhooks/" + cred.Shortcode + "/timeout",
		"ResultURL":              p.cfg.App.BaseURL + "/webhooks/" + cred.Shortcode + "/result",
	}

	// Make request
	baseURL := p.getBaseURL(string(cred.Environment))
	url := baseURL + "/mpesa/accountbalance/v1/query"

	responseBody, err := p.makeAuthenticatedRequest(ctx, "POST", url, token, payload)
	if err != nil {
		return nil, err
	}

	// Parse response
	var response struct {
		ConversationID           string `json:"ConversationID"`
		OriginatorConversationID string `json:"OriginatorConversationID"`
		ResponseCode             string `json:"ResponseCode"`
		ResponseDescription      string `json:"ResponseDescription"`
		ErrorCode                string `json:"errorCode"`
		ErrorMessage             string `json:"errorMessage"`
	}

	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, &provider.ProviderError{
			Code:    "response_parse_failed",
			Message: fmt.Sprintf("failed to parse balance response: %v", err),
		}
	}

	// Check for errors
	if response.ErrorCode != "" {
		return nil, &provider.ProviderError{
			Code:    response.ErrorCode,
			Message: response.ErrorMessage,
		}
	}

	if response.ResponseCode != "0" {
		return nil, &provider.ProviderError{
			Code:    "balance_failed",
			Message: response.ResponseDescription,
		}
	}

	p.logOperation("balance_inquiry", map[string]interface{}{
		"conversation_id": response.ConversationID,
		"shortcode":       cred.Shortcode,
	})

	return &provider.BalanceResp{
		ExternalID: response.ConversationID,
		Status:     provider.StatusPending,
		Message:    response.ResponseDescription,
	}, nil
}

// GetTransactionStatus checks transaction status
func (p *Provider) GetTransactionStatus(ctx context.Context, cred *credential.ProviderCredential, externalID string) (*provider.StatusResp, error) {
	// Get access token
	token, err := p.getAccessToken(ctx, cred)
	if err != nil {
		return nil, &provider.ProviderError{
			Code:    "auth_failed",
			Message: fmt.Sprintf("failed to get access token: %v", err),
		}
	}

	// Build request payload
	payload := map[string]interface{}{
		"Initiator":              "testapi", // This should be configurable
		"SecurityCredential":     p.getSecurityCredential(cred),
		"CommandID":              "TransactionStatusQuery",
		"TransactionID":          externalID,
		"PartyA":                 cred.Shortcode,
		"IdentifierType":         "4",
		"Remarks":                "Transaction status query",
		"QueueTimeOutURL":        p.cfg.App.BaseURL + "/webhooks/" + cred.Shortcode + "/timeout",
		"ResultURL":              p.cfg.App.BaseURL + "/webhooks/" + cred.Shortcode + "/result",
		"Occasion":               "Status Check",
	}

	// Make request
	baseURL := p.getBaseURL(string(cred.Environment))
	url := baseURL + "/mpesa/transactionstatus/v1/query"

	responseBody, err := p.makeAuthenticatedRequest(ctx, "POST", url, token, payload)
	if err != nil {
		return nil, err
	}

	// Parse response
	var response struct {
		ConversationID           string `json:"ConversationID"`
		OriginatorConversationID string `json:"OriginatorConversationID"`
		ResponseCode             string `json:"ResponseCode"`
		ResponseDescription      string `json:"ResponseDescription"`
		ErrorCode                string `json:"errorCode"`
		ErrorMessage             string `json:"errorMessage"`
	}

	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, &provider.ProviderError{
			Code:    "response_parse_failed",
			Message: fmt.Sprintf("failed to parse status response: %v", err),
		}
	}

	// Check for errors
	if response.ErrorCode != "" {
		return nil, &provider.ProviderError{
			Code:    response.ErrorCode,
			Message: response.ErrorMessage,
		}
	}

	if response.ResponseCode != "0" {
		return nil, &provider.ProviderError{
			Code:    "status_failed",
			Message: response.ResponseDescription,
		}
	}

	p.logOperation("status_query", map[string]interface{}{
		"conversation_id": response.ConversationID,
		"external_id":     externalID,
		"shortcode":       cred.Shortcode,
	})

	return &provider.StatusResp{
		ExternalID:    externalID,
		Status:        provider.StatusPending,
		Message:       response.ResponseDescription,
		ConversationID: response.ConversationID,
	}, nil
}

// ParseWebhook parses M-Pesa webhook payload
func (p *Provider) ParseWebhook(body []byte, headers map[string]string) (provider.Event, error) {
	webhookService := NewWebhookService()
	return webhookService.Parse(body, headers)
}

// ValidateWebhook validates webhook authenticity
func (p *Provider) ValidateWebhook(body []byte, headers map[string]string, webhookToken string) error {
	webhookService := NewWebhookService()
	return webhookService.Validate(body, headers, webhookToken)
}

// getBaseURL returns the appropriate base URL for the environment
func (p *Provider) getBaseURL(environment string) string {
	if environment == "production" {
		return "https://api.safaricom.co.ke"
	}
	return "https://sandbox.safaricom.co.ke"
}

// logOperation logs provider operations for debugging
func (p *Provider) logOperation(operation string, details map[string]interface{}) {
	log.Info().
		Str("provider", "mpesa").
		Str("operation", operation).
		Fields(details).
		Msg("M-Pesa operation")
}

// getAccessToken retrieves or generates an access token for M-Pesa API
func (p *Provider) getAccessToken(ctx context.Context, cred *credential.ProviderCredential) (string, error) {
	// Check cache first
	cacheKey := cred.Shortcode + "_" + string(cred.Environment)
	if token, exists := p.tokenCache[cacheKey]; exists && token.ExpiresAt.After(time.Now().Add(5*time.Minute)) {
		return token.Token, nil
	}

	// Get credentials
	consumerKey := cred.GetDecryptedField("consumer_key", p.cfg.Sec.AESKey)
	consumerSecret := cred.GetDecryptedField("consumer_secret", p.cfg.Sec.AESKey)
	
	if consumerKey == "" || consumerSecret == "" {
		return "", fmt.Errorf("missing consumer key or secret")
	}

	// Create basic auth
	auth := base64.StdEncoding.EncodeToString([]byte(consumerKey + ":" + consumerSecret))

	// Build request
	baseURL := p.getBaseURL(string(cred.Environment))
	url := baseURL + "/oauth/v1/generate?grant_type=client_credentials"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")

	// Make request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth failed with status %d", resp.StatusCode)
	}

	// Parse response
	var authResponse struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		return "", fmt.Errorf("failed to parse auth response: %w", err)
	}

	// Parse expiry
	expiresIn, err := strconv.Atoi(authResponse.ExpiresIn)
	if err != nil {
		expiresIn = 3600 // Default to 1 hour
	}

	// Cache token
	p.tokenCache[cacheKey] = &accessToken{
		Token:     authResponse.AccessToken,
		ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Second),
	}

	return authResponse.AccessToken, nil
}

// makeAuthenticatedRequest makes an authenticated request to M-Pesa API
func (p *Provider) makeAuthenticatedRequest(ctx context.Context, method, url, token string, payload interface{}) ([]byte, error) {
	var body []byte
	var err error

	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, &provider.ProviderError{
			Code:    "request_failed",
			Message: fmt.Sprintf("request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &provider.ProviderError{
			Code:    "api_error",
			Message: fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(responseBody)),
		}
	}

	return responseBody, nil
}

// generateTransactionID generates a unique transaction ID
func (p *Provider) generateTransactionID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("PM_%d_%x", time.Now().Unix(), bytes), nil
}

// generateConversationID generates a unique conversation ID for B2C
func (p *Provider) generateConversationID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("AG_%d_%x", time.Now().Unix(), bytes), nil
}

// getSecurityCredential returns the security credential (placeholder)
func (p *Provider) getSecurityCredential(cred *credential.ProviderCredential) string {
	// In production, this should be the encrypted initiator password
	// For now, return a placeholder
	return "encrypted_security_credential"
}
