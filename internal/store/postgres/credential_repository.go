package postgres

import (
	"context"
	"database/sql"
	
	"paymatch/internal/domain/credential"
	
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// credentialRepository implements CredentialRepository interface with pure data access
type credentialRepository struct {
	db *pgxpool.Pool
}

// NewCredentialRepository creates a new credential repository
func NewCredentialRepository(db *pgxpool.Pool) *credentialRepository {
	return &credentialRepository{db: db}
}

// Save saves a credential (insert or update)
func (r *credentialRepository) Save(ctx context.Context, c *credential.ProviderCredential) error {
	if c.ID == 0 {
		return r.insert(ctx, c)
	}
	return r.update(ctx, c)
}

// FindByID finds a credential by ID
func (r *credentialRepository) FindByID(ctx context.Context, id int64) (*credential.ProviderCredential, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, provider, shortcode, environment, 
		       webhook_token, c2b_mode, c2b_bill_ref_required, c2b_bill_ref_regex, is_active
		FROM provider_credentials 
		WHERE id = $1`, id)
	
	return r.scanCredential(row)
}

// FindByShortcode finds a credential by shortcode
func (r *credentialRepository) FindByShortcode(ctx context.Context, shortcode string) (*credential.ProviderCredential, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, provider, shortcode, environment, 
		       webhook_token, c2b_mode, c2b_bill_ref_required, c2b_bill_ref_regex, is_active
		FROM provider_credentials 
		WHERE shortcode = $1 AND is_active = true`, shortcode)
	
	return r.scanCredential(row)
}

// FindByWebhookToken finds a credential by webhook token
func (r *credentialRepository) FindByWebhookToken(ctx context.Context, token string) (*credential.ProviderCredential, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, provider, shortcode, environment, 
		       webhook_token, c2b_mode, c2b_bill_ref_required, c2b_bill_ref_regex, is_active
		FROM provider_credentials 
		WHERE webhook_token = $1 AND is_active = true`, token)
	
	return r.scanCredential(row)
}

// FindByTenantID finds credentials by tenant ID
func (r *credentialRepository) FindByTenantID(ctx context.Context, tenantID int64) ([]*credential.ProviderCredential, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, provider, shortcode, environment, 
		       webhook_token, c2b_mode, c2b_bill_ref_required, c2b_bill_ref_regex, is_active
		FROM provider_credentials 
		WHERE tenant_id = $1 AND is_active = true
		ORDER BY id DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var credentials []*credential.ProviderCredential
	for rows.Next() {
		c, err := r.scanCredentialFromRows(rows)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, c)
	}
	
	return credentials, rows.Err()
}

// Deactivate marks a credential as inactive
func (r *credentialRepository) Deactivate(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE provider_credentials 
		SET is_active = false, updated_at = now() 
		WHERE id = $1`, id)
	return err
}

// insert creates a new credential record
func (r *credentialRepository) insert(ctx context.Context, c *credential.ProviderCredential) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO provider_credentials (tenant_id, provider, shortcode, environment, 
		                                 webhook_token, c2b_mode, c2b_bill_ref_required, c2b_bill_ref_regex, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		c.TenantID, c.Provider, c.Shortcode, string(c.Environment),
		c.WebhookToken, string(c.C2BConfiguration.Mode), c.C2BConfiguration.BillRefRequired, c.C2BConfiguration.BillRefRegex, c.IsActive).Scan(&c.ID)
	
	return err
}

// update modifies an existing credential record
func (r *credentialRepository) update(ctx context.Context, c *credential.ProviderCredential) error {
	_, err := r.db.Exec(ctx, `
		UPDATE provider_credentials 
		SET provider = $1, shortcode = $2, environment = $3, 
		    webhook_token = $4, c2b_mode = $5, c2b_bill_ref_required = $6, c2b_bill_ref_regex = $7, is_active = $8
		WHERE id = $9`,
		c.Provider, c.Shortcode, string(c.Environment),
		c.WebhookToken, string(c.C2BConfiguration.Mode), c.C2BConfiguration.BillRefRequired, c.C2BConfiguration.BillRefRegex, c.IsActive, c.ID)
	
	return err
}

// scanCredential scans a single row into credential domain object
func (r *credentialRepository) scanCredential(row pgx.Row) (*credential.ProviderCredential, error) {
	var c credential.ProviderCredential
	var provider, environment, c2bMode string
	var billRefRequired sql.NullBool
	var billRefRegex sql.NullString
	
	err := row.Scan(
		&c.ID, &c.TenantID, &provider, &c.Shortcode, &environment,
		&c.WebhookToken, &c2bMode, &billRefRequired, &billRefRegex, &c.IsActive)
	if err != nil {
		return nil, err
	}
	
	c.Provider = provider
	c.ProviderType = credential.ProviderType(provider)
	c.Environment = credential.Environment(environment)
	c.C2BConfiguration.Mode = credential.C2BMode(c2bMode)
	if billRefRequired.Valid {
		c.C2BConfiguration.BillRefRequired = billRefRequired.Bool
	}
	if billRefRegex.Valid {
		c.C2BConfiguration.BillRefRegex = billRefRegex.String
	}
	
	return &c, nil
}

// scanCredentialFromRows scans rows into credential domain object
func (r *credentialRepository) scanCredentialFromRows(rows pgx.Rows) (*credential.ProviderCredential, error) {
	var c credential.ProviderCredential
	var provider, environment, c2bMode string
	var billRefRequired sql.NullBool
	var billRefRegex sql.NullString
	
	err := rows.Scan(
		&c.ID, &c.TenantID, &provider, &c.Shortcode, &environment,
		&c.WebhookToken, &c2bMode, &billRefRequired, &billRefRegex, &c.IsActive)
	if err != nil {
		return nil, err
	}
	
	c.Provider = provider
	c.ProviderType = credential.ProviderType(provider)
	c.Environment = credential.Environment(environment)
	c.C2BConfiguration.Mode = credential.C2BMode(c2bMode)
	if billRefRequired.Valid {
		c.C2BConfiguration.BillRefRequired = billRefRequired.Bool
	}
	if billRefRegex.Valid {
		c.C2BConfiguration.BillRefRegex = billRefRegex.String
	}
	
	return &c, nil
}