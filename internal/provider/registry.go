package provider

import (
	"context"
	"fmt"
	"sync"

	"paymatch/internal/config"
	"paymatch/internal/domain/credential"
	"paymatch/internal/store/repositories"

	"github.com/rs/zerolog/log"
)

// Registry manages all payment providers
type Registry struct {
	providers      map[ProviderType]Provider
	cfg            config.Cfg
	credentialRepo repositories.CredentialRepository
	mu             sync.RWMutex
}

// NewRegistry creates a new provider registry
func NewRegistry(cfg config.Cfg, credentialRepo repositories.CredentialRepository) *Registry {
	return &Registry{
		providers:      make(map[ProviderType]Provider),
		cfg:            cfg,
		credentialRepo: credentialRepo,
	}
}

// RegisterProvider adds a provider to the registry
func (r *Registry) RegisterProvider(providerType ProviderType, provider Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.providers[providerType] = provider
	log.Info().
		Str("provider", string(providerType)).
		Str("name", provider.Name()).
		Strs("operations", operationTypesToStrings(provider.SupportedOperations())).
		Msg("registered payment provider")
}

// GetProvider returns a provider by type
func (r *Registry) GetProvider(providerType ProviderType) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	provider, ok := r.providers[providerType]
	if !ok {
		return nil, &ProviderError{
			Code:    "provider_not_found",
			Message: fmt.Sprintf("provider %s not registered", providerType),
		}
	}
	return provider, nil
}

// GetProviderForCredential resolves provider from credential
func (r *Registry) GetProviderForCredential(ctx context.Context, cred *credential.ProviderCredential) (Provider, error) {
	// If provider_type is set, use it directly
	if cred.ProviderType != "" {
		return r.GetProvider(ProviderType(cred.ProviderType))
	}
	
	// Fallback to legacy provider field (for backward compatibility)
	if cred.Provider != "" {
		return r.GetProvider(ProviderType(cred.Provider))
	}
	
	return nil, &ProviderError{
		Code:    "provider_not_specified",
		Message: "no provider type specified in credential",
	}
}

// ListProviders returns all registered provider types
func (r *Registry) ListProviders() []ProviderType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	var types []ProviderType
	for t := range r.providers {
		types = append(types, t)
	}
	return types
}

// GetProviderInfo returns detailed information about a provider
func (r *Registry) GetProviderInfo(providerType ProviderType) (*ProviderInfo, error) {
	provider, err := r.GetProvider(providerType)
	if err != nil {
		return nil, err
	}
	
	return &ProviderInfo{
		Type:                 providerType,
		Name:                 provider.Name(),
		SupportedOperations:  provider.SupportedOperations(),
		RequiredCredentials:  provider.RequiredCredentialFields(),
	}, nil
}

// GetAllProviderInfo returns information about all registered providers
func (r *Registry) GetAllProviderInfo() []*ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	var infos []*ProviderInfo
	for providerType, provider := range r.providers {
		infos = append(infos, &ProviderInfo{
			Type:                 providerType,
			Name:                 provider.Name(),
			SupportedOperations:  provider.SupportedOperations(),
			RequiredCredentials:  provider.RequiredCredentialFields(),
		})
	}
	return infos
}

// STKPush executes STK push through the appropriate provider
func (r *Registry) STKPush(ctx context.Context, cred *credential.ProviderCredential, req STKPushReq) (*STKPushResp, error) {
	provider, err := r.GetProviderForCredential(ctx, cred)
	if err != nil {
		return nil, err
	}
	
	// Check if provider supports STK Push
	if !r.supportsOperation(provider, OpSTKPush) {
		return nil, &ProviderError{
			Code:    "operation_not_supported",
			Message: fmt.Sprintf("provider %s does not support STK Push", provider.Name()),
		}
	}
	
	return provider.STKPush(ctx, cred, req)
}

// B2C executes B2C transfer through the appropriate provider
func (r *Registry) B2C(ctx context.Context, cred *credential.ProviderCredential, req B2CReq) (*B2CResp, error) {
	provider, err := r.GetProviderForCredential(ctx, cred)
	if err != nil {
		return nil, err
	}
	
	if !r.supportsOperation(provider, OpB2C) {
		return nil, &ProviderError{
			Code:    "operation_not_supported",
			Message: fmt.Sprintf("provider %s does not support B2C transfers", provider.Name()),
		}
	}
	
	return provider.B2C(ctx, cred, req)
}

// BulkTransfer executes bulk transfer through the appropriate provider
func (r *Registry) BulkTransfer(ctx context.Context, cred *credential.ProviderCredential, req BulkTransferReq) (*BulkTransferResp, error) {
	provider, err := r.GetProviderForCredential(ctx, cred)
	if err != nil {
		return nil, err
	}
	
	if !r.supportsOperation(provider, OpBulkTransfer) {
		return nil, &ProviderError{
			Code:    "operation_not_supported",
			Message: fmt.Sprintf("provider %s does not support bulk transfers", provider.Name()),
		}
	}
	
	return provider.BulkTransfer(ctx, cred, req)
}

// CheckBalance checks account balance through the appropriate provider
func (r *Registry) CheckBalance(ctx context.Context, cred *credential.ProviderCredential) (*BalanceResp, error) {
	provider, err := r.GetProviderForCredential(ctx, cred)
	if err != nil {
		return nil, err
	}
	
	if !r.supportsOperation(provider, OpBalance) {
		return nil, &ProviderError{
			Code:    "operation_not_supported",
			Message: fmt.Sprintf("provider %s does not support balance inquiry", provider.Name()),
		}
	}
	
	return provider.CheckBalance(ctx, cred)
}

// GetTransactionStatus checks transaction status through the appropriate provider
func (r *Registry) GetTransactionStatus(ctx context.Context, cred *credential.ProviderCredential, externalID string) (*StatusResp, error) {
	provider, err := r.GetProviderForCredential(ctx, cred)
	if err != nil {
		return nil, err
	}
	
	if !r.supportsOperation(provider, OpStatus) {
		return nil, &ProviderError{
			Code:    "operation_not_supported",
			Message: fmt.Sprintf("provider %s does not support status inquiry", provider.Name()),
		}
	}
	
	return provider.GetTransactionStatus(ctx, cred, externalID)
}

// ParseWebhook parses webhook through the appropriate provider
func (r *Registry) ParseWebhook(ctx context.Context, cred *credential.ProviderCredential, body []byte, headers map[string]string) (Event, error) {
	provider, err := r.GetProviderForCredential(ctx, cred)
	if err != nil {
		return Event{}, err
	}
	
	return provider.ParseWebhook(body, headers)
}

// ValidateWebhook validates webhook signature through the appropriate provider
func (r *Registry) ValidateWebhook(ctx context.Context, cred *credential.ProviderCredential, body []byte, headers map[string]string) error {
	provider, err := r.GetProviderForCredential(ctx, cred)
	if err != nil {
		return err
	}
	
	return provider.ValidateWebhook(body, headers, cred.WebhookToken)
}

// Helper types and functions

// ProviderInfo contains metadata about a provider
type ProviderInfo struct {
	Type                 ProviderType      `json:"type"`
	Name                 string            `json:"name"`
	SupportedOperations  []OperationType   `json:"supported_operations"`
	RequiredCredentials  []CredentialField `json:"required_credentials"`
}

// supportsOperation checks if a provider supports a specific operation
func (r *Registry) supportsOperation(provider Provider, operation OperationType) bool {
	for _, op := range provider.SupportedOperations() {
		if op == operation {
			return true
		}
	}
	return false
}

// operationTypesToStrings converts operation types to strings for logging
func operationTypesToStrings(ops []OperationType) []string {
	var strs []string
	for _, op := range ops {
		strs = append(strs, string(op))
	}
	return strs
}