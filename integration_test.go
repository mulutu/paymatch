package main

import (
	"testing"

	"paymatch/internal/config"
	"paymatch/internal/provider"
	"paymatch/internal/provider/mpesa"
)

// TestPureArchitectureIntegration tests the basic integration of pure architecture components
func TestPureArchitectureIntegration(t *testing.T) {
	// Test config loading (without requiring actual env vars)
	cfg := config.Cfg{
		App: config.AppCfg{
			Env:  "test",
			Port: "8080",
		},
		Sec: config.SecurityCfg{
			AESKey: make([]byte, 32), // Dummy key for testing
		},
	}

	// Test provider registry initialization
	registry := provider.NewProviderRegistry(cfg, nil)
	if registry == nil {
		t.Fatal("failed to create provider registry")
	}

	// Test M-Pesa provider creation and registration
	mpesaProvider := mpesa.New(cfg)
	if mpesaProvider == nil {
		t.Fatal("failed to create M-Pesa provider")
	}

	registry.RegisterProvider(provider.ProviderMpesa, mpesaProvider)
	
	// Verify provider is registered
	providers := registry.ListProviders()
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}

	if providers[0] != provider.ProviderMpesa {
		t.Fatalf("expected M-Pesa provider, got %s", providers[0])
	}

	// Test provider metadata
	info, err := registry.GetProviderInfo(provider.ProviderMpesa)
	if err != nil {
		t.Fatalf("failed to get provider info: %v", err)
	}

	if info.Name != "M-Pesa (Safaricom Daraja)" {
		t.Fatalf("unexpected provider name: %s", info.Name)
	}

	// Test supported operations
	expectedOperations := []provider.OperationType{
		provider.OpSTKPush,
		provider.OpC2B,
		provider.OpB2C,
		provider.OpBalance,
		provider.OpStatus,
	}

	if len(info.SupportedOperations) != len(expectedOperations) {
		t.Fatalf("expected %d operations, got %d", len(expectedOperations), len(info.SupportedOperations))
	}

	t.Log("Pure architecture integration test passed")
	t.Log("✅ Provider registry system working")
	t.Log("✅ M-Pesa provider integration working") 
	t.Log("✅ Provider metadata and operations working")
}

// TestProviderTypes tests provider type constants and availability
func TestProviderTypes(t *testing.T) {
	availableProviders := provider.GetAvailableProviders()
	
	if len(availableProviders) == 0 {
		t.Fatal("no available providers")
	}

	// Test M-Pesa is available
	found := false
	for _, p := range availableProviders {
		if p == provider.ProviderMpesa {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("M-Pesa provider not found in available providers")
	}

	// Test provider support check
	if !provider.IsProviderSupported(provider.ProviderMpesa) {
		t.Fatal("M-Pesa provider should be supported")
	}

	t.Log("Provider type system working correctly")
}