package provider

import (
	"paymatch/internal/config"
	"paymatch/internal/store/repositories"
)

// NewProviderRegistry creates and configures the provider registry (providers registered externally)
func NewProviderRegistry(cfg config.Cfg, credentialRepo repositories.CredentialRepository) *Registry {
	registry := NewRegistry(cfg, credentialRepo)
	return registry
}

// GetAvailableProviders returns a list of all available provider types
func GetAvailableProviders() []ProviderType {
	return []ProviderType{
		ProviderMpesa,
		// Add other providers here as they become available
	}
}

// IsProviderSupported checks if a provider type is supported
func IsProviderSupported(providerType ProviderType) bool {
	availableProviders := GetAvailableProviders()
	for _, available := range availableProviders {
		if available == providerType {
			return true
		}
	}
	return false
}