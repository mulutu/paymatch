package repositories

import (
	"context"
	
	"paymatch/internal/domain/payment"
	"paymatch/internal/domain/event"
	"paymatch/internal/domain/credential"
	"paymatch/internal/domain/tenant"
)

// PaymentRepository defines the contract for payment data access
type PaymentRepository interface {
	Save(ctx context.Context, payment *payment.Payment) error
	FindByID(ctx context.Context, id int64) (*payment.Payment, error)
	FindByExternalID(ctx context.Context, tenantID int64, externalID string) (*payment.Payment, error)
	FindByTenantID(ctx context.Context, tenantID int64, limit, offset int) ([]*payment.Payment, error)
	UpdateStatus(ctx context.Context, id int64, status payment.Status) error
}

// EventRepository defines the contract for event data access
type EventRepository interface {
	Save(ctx context.Context, event *event.Event) error
	FindByID(ctx context.Context, id int64) (*event.Event, error)
	FindUnprocessed(ctx context.Context, limit int) ([]*event.Event, error)
	FindByTenantID(ctx context.Context, tenantID int64, limit, offset int) ([]*event.Event, error)
	MarkProcessed(ctx context.Context, id int64, status event.ProcessingStatus) error
	MarkForReprocessing(ctx context.Context, tenantID, eventID int64) error
}

// CredentialRepository defines the contract for credential data access
type CredentialRepository interface {
	Save(ctx context.Context, cred *credential.ProviderCredential) error
	FindByID(ctx context.Context, id int64) (*credential.ProviderCredential, error)
	FindByShortcode(ctx context.Context, shortcode string) (*credential.ProviderCredential, error)
	FindByWebhookToken(ctx context.Context, token string) (*credential.ProviderCredential, error)
	FindByTenantID(ctx context.Context, tenantID int64) ([]*credential.ProviderCredential, error)
	Deactivate(ctx context.Context, id int64) error
}

// TenantRepository defines the contract for tenant data access  
type TenantRepository interface {
	Save(ctx context.Context, tenant *tenant.Tenant) error
	FindByID(ctx context.Context, id int64) (*tenant.Tenant, error)
	FindByAPIKeyHash(ctx context.Context, keyHash string) (*tenant.Tenant, error)
	SaveAPIKey(ctx context.Context, apiKey *tenant.APIKey) error
	FindAPIKeyByHash(ctx context.Context, keyHash string) (*tenant.APIKey, error)
}

// UnitOfWork defines transactional operations
type UnitOfWork interface {
	Begin(ctx context.Context) (Transaction, error)
}

// Transaction defines a database transaction
type Transaction interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	PaymentRepository() PaymentRepository
	EventRepository() EventRepository
}