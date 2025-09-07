package postgres

import (
	"context"
	
	"paymatch/internal/domain/tenant"
	
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// tenantRepository implements TenantRepository interface with pure data access
type tenantRepository struct {
	db *pgxpool.Pool
}

// NewTenantRepository creates a new tenant repository
func NewTenantRepository(db *pgxpool.Pool) *tenantRepository {
	return &tenantRepository{db: db}
}

// Save saves a tenant (insert or update)
func (r *tenantRepository) Save(ctx context.Context, t *tenant.Tenant) error {
	if t.ID == 0 {
		return r.insert(ctx, t)
	}
	return r.update(ctx, t)
}

// FindByID finds a tenant by ID
func (r *tenantRepository) FindByID(ctx context.Context, id int64) (*tenant.Tenant, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, name, status
		FROM tenants 
		WHERE id = $1`, id)
	
	return r.scanTenant(row)
}

// FindByAPIKeyHash finds a tenant by API key hash
func (r *tenantRepository) FindByAPIKeyHash(ctx context.Context, keyHash string) (*tenant.Tenant, error) {
	row := r.db.QueryRow(ctx, `
		SELECT t.id, t.name, t.status
		FROM tenants t
		JOIN api_keys ak ON t.id = ak.tenant_id
		WHERE ak.key_hash = $1 AND ak.is_active = true AND t.status = 'active'`, keyHash)
	
	return r.scanTenant(row)
}

// SaveAPIKey saves an API key record
func (r *tenantRepository) SaveAPIKey(ctx context.Context, apiKey *tenant.APIKey) error {
	if apiKey.ID == 0 {
		return r.insertAPIKey(ctx, apiKey)
	}
	return r.updateAPIKey(ctx, apiKey)
}

// FindAPIKeyByHash finds an API key by hash
func (r *tenantRepository) FindAPIKeyByHash(ctx context.Context, keyHash string) (*tenant.APIKey, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, name, key_hash, is_active
		FROM api_keys 
		WHERE key_hash = $1 AND is_active = true`, keyHash)
	
	return r.scanAPIKey(row)
}

// insert creates a new tenant record
func (r *tenantRepository) insert(ctx context.Context, t *tenant.Tenant) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO tenants (name, status)
		VALUES ($1, $2)
		RETURNING id`,
		t.Name, string(t.Status)).Scan(&t.ID)
	
	return err
}

// update modifies an existing tenant record
func (r *tenantRepository) update(ctx context.Context, t *tenant.Tenant) error {
	_, err := r.db.Exec(ctx, `
		UPDATE tenants 
		SET name = $1, status = $2
		WHERE id = $3`,
		t.Name, string(t.Status), t.ID)
	
	return err
}

// insertAPIKey creates a new API key record
func (r *tenantRepository) insertAPIKey(ctx context.Context, apiKey *tenant.APIKey) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO api_keys (tenant_id, name, key_hash, is_active)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		apiKey.TenantID, apiKey.Name, apiKey.KeyHash, apiKey.IsActive).Scan(&apiKey.ID)
	
	return err
}

// updateAPIKey modifies an existing API key record
func (r *tenantRepository) updateAPIKey(ctx context.Context, apiKey *tenant.APIKey) error {
	_, err := r.db.Exec(ctx, `
		UPDATE api_keys 
		SET name = $1, is_active = $2
		WHERE id = $3`,
		apiKey.Name, apiKey.IsActive, apiKey.ID)
	
	return err
}

// scanTenant scans a single row into tenant domain object
func (r *tenantRepository) scanTenant(row pgx.Row) (*tenant.Tenant, error) {
	var t tenant.Tenant
	var status string
	
	err := row.Scan(&t.ID, &t.Name, &status)
	if err != nil {
		return nil, err
	}
	
	t.Status = tenant.Status(status)
	
	return &t, nil
}

// scanAPIKey scans a single row into API key domain object
func (r *tenantRepository) scanAPIKey(row pgx.Row) (*tenant.APIKey, error) {
	var apiKey tenant.APIKey
	
	err := row.Scan(
		&apiKey.ID, &apiKey.TenantID, &apiKey.Name, &apiKey.KeyHash, &apiKey.IsActive)
	if err != nil {
		return nil, err
	}
	
	return &apiKey, nil
}