package provider

import (
	"context"
	"paymatch/internal/domain/credential"
)

// Provider defines the interface all payment providers must implement
type Provider interface {
	// Core payment operations
	STKPush(ctx context.Context, cred *credential.ProviderCredential, req STKPushReq) (*STKPushResp, error)
	B2C(ctx context.Context, cred *credential.ProviderCredential, req B2CReq) (*B2CResp, error)
	BulkTransfer(ctx context.Context, cred *credential.ProviderCredential, req BulkTransferReq) (*BulkTransferResp, error)

	// Webhook processing
	ParseWebhook(body []byte, headers map[string]string) (Event, error)
	ValidateWebhook(body []byte, headers map[string]string, webhookToken string) error

	// Provider metadata
	Name() string
	SupportedOperations() []OperationType
	RequiredCredentialFields() []CredentialField

	// Account operations
	CheckBalance(ctx context.Context, cred *credential.ProviderCredential) (*BalanceResp, error)
	GetTransactionStatus(ctx context.Context, cred *credential.ProviderCredential, externalID string) (*StatusResp, error)
}
