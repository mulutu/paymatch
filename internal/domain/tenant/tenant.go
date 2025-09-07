package tenant

import (
	"fmt"
	"strings"
)

// Tenant represents a business tenant in the system
type Tenant struct {
	ID     int64
	Name   string
	Status Status
}

// Status represents tenant status
type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusClosed    Status = "closed"
)

// APIKey represents a tenant API key
type APIKey struct {
	ID       int64
	TenantID int64
	Name     string
	KeyHash  string
	IsActive bool
}

// NewTenant creates a new tenant with validation
func NewTenant(name string) (*Tenant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("tenant name is required")
	}
	
	if len(name) < 2 || len(name) > 100 {
		return nil, fmt.Errorf("tenant name must be between 2 and 100 characters")
	}
	
	return &Tenant{
		Name:   name,
		Status: StatusActive,
	}, nil
}

// NewAPIKey creates a new API key with validation
func NewAPIKey(tenantID int64, name, keyHash string) (*APIKey, error) {
	if tenantID <= 0 {
		return nil, fmt.Errorf("invalid tenant ID: %d", tenantID)
	}
	
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	
	if keyHash == "" {
		return nil, fmt.Errorf("key hash is required")
	}
	
	return &APIKey{
		TenantID: tenantID,
		Name:     name,
		KeyHash:  keyHash,
		IsActive: true,
	}, nil
}

// IsActive checks if tenant is active
func (t *Tenant) IsActive() bool {
	return t.Status == StatusActive
}

// Suspend suspends the tenant
func (t *Tenant) Suspend() error {
	if t.Status == StatusClosed {
		return fmt.Errorf("cannot suspend closed tenant")
	}
	
	t.Status = StatusSuspended
	return nil
}

// Activate activates the tenant
func (t *Tenant) Activate() error {
	if t.Status == StatusClosed {
		return fmt.Errorf("cannot activate closed tenant")
	}
	
	t.Status = StatusActive
	return nil
}

// Close permanently closes the tenant
func (t *Tenant) Close() error {
	t.Status = StatusClosed
	return nil
}

// CanPerformOperations checks if tenant can perform operations
func (t *Tenant) CanPerformOperations() bool {
	return t.Status == StatusActive
}

// IsValidForAPIKey checks if API key is valid for this tenant
func (a *APIKey) IsValidForTenant(tenantID int64) bool {
	return a.TenantID == tenantID && a.IsActive
}

// Deactivate deactivates the API key
func (a *APIKey) Deactivate() {
	a.IsActive = false
}

// Activate activates the API key
func (a *APIKey) Activate() {
	a.IsActive = true
}