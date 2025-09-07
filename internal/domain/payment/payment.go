package payment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Payment represents a financial payment transaction
type Payment struct {
	ID         int64
	TenantID   int64
	InvoiceNo  string
	Amount     Money
	Currency   Currency
	Status     Status
	Method     Method
	ExternalID string
	MSISDNHash string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Money represents a monetary amount in smallest currency unit (cents)
type Money int64

// Currency represents a currency code
type Currency string

const (
	KES Currency = "KES"
	USD Currency = "USD"
)

// Status represents payment status
type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Method represents payment method
type Method string

const (
	MethodMpesa Method = "mpesa"
	MethodCard  Method = "card"
	MethodBank  Method = "bank"
)

// MSISDN represents a mobile phone number with validation
type MSISDN struct {
	value string
}

// NewMSISDN creates a new MSISDN with validation
func NewMSISDN(phone string) (*MSISDN, error) {
	normalized := strings.TrimSpace(phone)
	if normalized == "" {
		return nil, fmt.Errorf("phone number cannot be empty")
	}
	
	// Basic phone validation (can be enhanced)
	if len(normalized) < 10 || len(normalized) > 15 {
		return nil, fmt.Errorf("invalid phone number format: %s", phone)
	}
	
	return &MSISDN{value: strings.ToLower(normalized)}, nil
}

// String returns the normalized phone number
func (m *MSISDN) String() string {
	return m.value
}

// Hash returns a privacy-preserving hash of the phone number
func (m *MSISDN) Hash() string {
	h := sha256.Sum256([]byte(m.value))
	return hex.EncodeToString(h[:])
}

// NewPayment creates a new payment with validation
func NewPayment(tenantID int64, invoice string, amount Money, currency Currency, method Method, externalID string, msisdn *MSISDN) (*Payment, error) {
	if err := validatePaymentCreation(tenantID, amount, externalID); err != nil {
		return nil, err
	}
	
	var msisdnHash string
	if msisdn != nil {
		msisdnHash = msisdn.Hash()
	}
	
	return &Payment{
		TenantID:   tenantID,
		InvoiceNo:  invoice,
		Amount:     amount,
		Currency:   currency,
		Status:     StatusPending,
		Method:     method,
		ExternalID: externalID,
		MSISDNHash: msisdnHash,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

// Update updates payment fields following business rules
func (p *Payment) Update(invoice string, amount Money, status Status, msisdn *MSISDN) error {
	if err := p.validateUpdate(amount, status); err != nil {
		return err
	}
	
	// Business rule: Only update non-empty values
	if invoice != "" {
		p.InvoiceNo = invoice
	}
	
	if amount > 0 {
		p.Amount = amount
	}
	
	if status != "" {
		p.Status = status
	}
	
	if msisdn != nil {
		p.MSISDNHash = msisdn.Hash()
	}
	
	p.UpdatedAt = time.Now()
	return nil
}

// IsCompleted checks if payment is in completed state
func (p *Payment) IsCompleted() bool {
	return p.Status == StatusCompleted
}

// CanBeUpdated checks if payment can be modified
func (p *Payment) CanBeUpdated() bool {
	return p.Status == StatusPending
}

// validatePaymentCreation validates payment creation
func validatePaymentCreation(tenantID int64, amount Money, externalID string) error {
	if tenantID <= 0 {
		return fmt.Errorf("invalid tenant ID: %d", tenantID)
	}
	
	if amount <= 0 {
		return fmt.Errorf("amount must be positive: %d", amount)
	}
	
	if strings.TrimSpace(externalID) == "" {
		return fmt.Errorf("external ID is required")
	}
	
	return nil
}

// validateUpdate validates payment update
func (p *Payment) validateUpdate(amount Money, status Status) error {
	if !p.CanBeUpdated() && status != StatusCompleted && status != StatusFailed {
		return fmt.Errorf("payment %d cannot be updated in status %s", p.ID, p.Status)
	}
	
	if amount < 0 {
		return fmt.Errorf("amount cannot be negative: %d", amount)
	}
	
	return nil
}

// DomainError represents a domain-level error
type DomainError struct {
	Message string
	Code    string
}

func (e DomainError) Error() string {
	return fmt.Sprintf("domain error [%s]: %s", e.Code, e.Message)
}

// Domain error codes
const (
	ErrInvalidAmount    = "INVALID_AMOUNT"
	ErrInvalidStatus    = "INVALID_STATUS"
	ErrInvalidTenant    = "INVALID_TENANT"
	ErrPaymentReadOnly  = "PAYMENT_READ_ONLY"
)