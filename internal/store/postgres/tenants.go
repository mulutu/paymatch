package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

type Tenant struct {
	ID     int64
	Name   string
	Status string
}

type TenantAPIKey struct {
	ID       int64
	TenantID int64
	KeyHash  string
}

func (r *Repo) LookupTenantByAPIKeyHash(ctx context.Context, keyHash string) (Tenant, error) {
	row := r.db.QueryRow(ctx, `SELECT t.id, t.name, t.status
		FROM tenant_api_keys k
		JOIN tenants t ON t.id=k.tenant_id
		WHERE k.key_hash=$1`, keyHash)
	var t Tenant
	if err := row.Scan(&t.ID, &t.Name, &t.Status); err != nil {
		return Tenant{}, err
	}
	return t, nil
}

// Helper to pre-hash API keys for seeding
func HashAPIKey(key string) string { h := sha256.Sum256([]byte(key)); return hex.EncodeToString(h[:]) }

// CreateTenant inserts a tenant.
func (r *Repo) CreateTenant(ctx context.Context, name string) (Tenant, error) {
	row := r.db.QueryRow(ctx, `INSERT INTO tenants(name) VALUES($1) RETURNING id, name, status`, name)
	var t Tenant
	if err := row.Scan(&t.ID, &t.Name, &t.Status); err != nil {
		return Tenant{}, err
	}
	return t, nil
}

// InsertAPIKey stores a hashed API key for a tenant.
func (r *Repo) InsertAPIKey(ctx context.Context, tenantID int64, keyName, keyHash string) (int64, error) {
	row := r.db.QueryRow(ctx, `INSERT INTO tenant_api_keys(tenant_id, key_hash, name) VALUES($1,$2,$3) RETURNING id`,
		tenantID, keyHash, keyName)
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}
