package tenant

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"paymatch/internal/config"
	"paymatch/internal/domain/credential"
	"paymatch/internal/domain/tenant"
	"paymatch/internal/store/repositories"
)

// OnboardingRequest represents tenant onboarding data
type OnboardingRequest struct {
	Name            string `json:"name"`
	APIKeyName      string `json:"apiKeyName,omitempty"`
	Provider        string `json:"provider,omitempty"`
	Shortcode       string `json:"shortcode"`
	Environment     string `json:"environment"`
	C2BMode         string `json:"c2bMode"`
	BillRefRequired *bool  `json:"billRefRequired,omitempty"`
	BillRefRegex    string `json:"billRefRegex,omitempty"`
	Passkey         string `json:"passkey"`
	ConsumerKey     string `json:"consumerKey"`
	ConsumerSecret  string `json:"consumerSecret"`
}

// OnboardingResponse represents tenant onboarding result
type OnboardingResponse struct {
	TenantID        int64  `json:"tenantId"`
	APIKey          string `json:"apiKey"`
	APIKeyName      string `json:"apiKeyName"`
	WebhookToken    string `json:"webhookToken"`
	Shortcode       string `json:"shortcode"`
	Environment     string `json:"environment"`
	C2BMode         string `json:"c2bMode"`
	BillRefRequired bool   `json:"billRefRequired"`
	BillRefRegex    string `json:"billRefRegex"`
}

// Service handles tenant management with pure architecture
type Service struct {
	tenantRepo     repositories.TenantRepository
	credentialRepo repositories.CredentialRepository
	cfg            config.Cfg
}

// NewService creates a new tenant service with pure architecture
func NewService(tenantRepo repositories.TenantRepository, credentialRepo repositories.CredentialRepository, cfg config.Cfg) *Service {
	return &Service{
		tenantRepo:     tenantRepo,
		credentialRepo: credentialRepo,
		cfg:            cfg,
	}
}

// OnboardTenant creates a new tenant with provider credentials using domain objects
func (s *Service) OnboardTenant(ctx context.Context, req OnboardingRequest) (*OnboardingResponse, error) {
	// Validate request
	if err := s.ValidateOnboardingRequest(&req); err != nil {
		return nil, err
	}

	// Create tenant domain object
	newTenant, err := tenant.NewTenant(strings.TrimSpace(req.Name))
	if err != nil {
		return nil, &ServiceError{Op: "create_tenant", Err: err}
	}

	// Save tenant
	if err := s.tenantRepo.Save(ctx, newTenant); err != nil {
		return nil, &ServiceError{Op: "save_tenant", Err: err}
	}

	// Generate API key
	apiKey, keyName, err := s.createAPIKey(ctx, newTenant.ID, req.APIKeyName)
	if err != nil {
		return nil, &ServiceError{Op: "create_api_key", Err: err}
	}

	// Create provider credential domain object
	providerCred, err := s.createProviderCredential(ctx, newTenant.ID, req)
	if err != nil {
		return nil, &ServiceError{Op: "create_credentials", Err: err}
	}

	// Save provider credential
	if err := s.credentialRepo.Save(ctx, providerCred); err != nil {
		return nil, &ServiceError{Op: "save_credentials", Err: err}
	}

	return &OnboardingResponse{
		TenantID:        newTenant.ID,
		APIKey:          apiKey,
		APIKeyName:      keyName,
		WebhookToken:    providerCred.WebhookToken,
		Shortcode:       providerCred.Shortcode,
		Environment:     string(providerCred.Environment),
		C2BMode:         string(providerCred.C2BConfiguration.Mode),
		BillRefRequired: providerCred.C2BConfiguration.BillRefRequired,
		BillRefRegex:    providerCred.C2BConfiguration.BillRefRegex,
	}, nil
}

// createAPIKey generates and stores a new API key for the tenant
func (s *Service) createAPIKey(ctx context.Context, tenantID int64, keyName string) (string, string, error) {
	if keyName == "" {
		keyName = "default"
	}

	// Generate random API key
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate API key: %w", err)
	}
	apiKey := "pk_" + hex.EncodeToString(keyBytes)

	// Create hash for storage
	keyHash := s.hashAPIKey(apiKey)

	// Create API key domain object
	apiKeyObj, err := tenant.NewAPIKey(tenantID, keyName, keyHash)
	if err != nil {
		return "", "", err
	}

	// Save API key
	if err := s.tenantRepo.SaveAPIKey(ctx, apiKeyObj); err != nil {
		return "", "", err
	}

	return apiKey, keyName, nil
}

// createProviderCredential creates encrypted provider credentials
func (s *Service) createProviderCredential(ctx context.Context, tenantID int64, req OnboardingRequest) (*credential.ProviderCredential, error) {
	// Parse provider type
	providerType := credential.ProviderType(req.Provider)
	
	// Parse environment
	env := credential.Environment(req.Environment)
	
	// Create C2B configuration
	c2bConfig := credential.C2BConfig{
		Mode:            credential.C2BMode(req.C2BMode),
		BillRefRequired: req.BillRefRequired != nil && *req.BillRefRequired,
		BillRefRegex:    req.BillRefRegex,
	}

	// Generate webhook token
	webhookToken, err := s.generateWebhookToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate webhook token: %w", err)
	}

	// Create provider credential domain object
	providerCred, err := credential.NewProviderCredential(
		tenantID,
		req.Provider,
		providerType,
		req.Shortcode,
		env,
		webhookToken,
		c2bConfig,
	)
	if err != nil {
		return nil, err
	}

	return providerCred, nil
}

// generateWebhookToken generates a secure webhook token
func (s *Service) generateWebhookToken() (string, error) {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	return "wh_" + hex.EncodeToString(tokenBytes), nil
}

// hashAPIKey creates a SHA256 hash of the API key for secure storage
func (s *Service) hashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

// ValidateOnboardingRequest validates the onboarding request
func (s *Service) ValidateOnboardingRequest(req *OnboardingRequest) error {
	// Validate tenant name
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return &ValidationError{Field: "name", Message: "tenant name is required"}
	}

	// Normalize and validate provider
	req.Provider = strings.TrimSpace(req.Provider)
	if req.Provider == "" {
		req.Provider = "mpesa_daraja"
	}

	// Validate environment
	req.Environment = strings.ToLower(strings.TrimSpace(req.Environment))
	if req.Environment != "sandbox" && req.Environment != "production" {
		return &ValidationError{Field: "environment", Message: "must be sandbox or production"}
	}

	// Validate C2B mode
	req.C2BMode = strings.ToLower(strings.TrimSpace(req.C2BMode))
	if req.C2BMode != "paybill" && req.C2BMode != "buygoods" {
		return &ValidationError{Field: "c2bMode", Message: "must be paybill or buygoods"}
	}

	// Validate shortcode
	if strings.TrimSpace(req.Shortcode) == "" {
		return &ValidationError{Field: "shortcode", Message: "shortcode is required"}
	}

	// Validate credentials
	if strings.TrimSpace(req.Passkey) == "" {
		return &ValidationError{Field: "passkey", Message: "passkey is required"}
	}
	if strings.TrimSpace(req.ConsumerKey) == "" {
		return &ValidationError{Field: "consumerKey", Message: "consumer key is required"}
	}
	if strings.TrimSpace(req.ConsumerSecret) == "" {
		return &ValidationError{Field: "consumerSecret", Message: "consumer secret is required"}
	}

	// Validate bill reference regex if provided
	if req.BillRefRegex != "" {
		if _, err := regexp.Compile(req.BillRefRegex); err != nil {
			return &ValidationError{Field: "billRefRegex", Message: "invalid regex pattern"}
		}
	}

	return nil
}

// GetTenantByAPIKey retrieves tenant by API key hash using domain objects
func (s *Service) GetTenantByAPIKey(ctx context.Context, apiKey string) (*tenant.Tenant, error) {
	keyHash := s.hashAPIKey(apiKey)
	return s.tenantRepo.FindByAPIKeyHash(ctx, keyHash)
}

// GetTenantCredentials retrieves provider credentials for a tenant
func (s *Service) GetTenantCredentials(ctx context.Context, tenantID int64) ([]*credential.ProviderCredential, error) {
	return s.credentialRepo.FindByTenantID(ctx, tenantID)
}

// GetCredentialByShortcode retrieves a credential by shortcode (for webhooks)
func (s *Service) GetCredentialByShortcode(ctx context.Context, shortcode string) (*credential.ProviderCredential, error) {
	return s.credentialRepo.FindByShortcode(ctx, shortcode)
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error [%s]: %s", e.Field, e.Message)
}

// ServiceError represents a service operation error
type ServiceError struct {
	Op  string
	Err error
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("tenant service [%s]: %v", e.Op, e.Err)
}

func (e *ServiceError) Unwrap() error {
	return e.Err
}