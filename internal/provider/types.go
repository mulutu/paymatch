package provider

import (
	"paymatch/internal/domain/event"
)

// Provider identification
type ProviderType string

const (
	ProviderMpesa       ProviderType = "mpesa_daraja"
	ProviderAirtelMoney ProviderType = "airtel_money"
	ProviderTKash       ProviderType = "tkash"
	ProviderEquitel     ProviderType = "equitel"
)

// Operation types that providers can support
type OperationType string

const (
	OpSTKPush      OperationType = "stk_push"
	OpC2B          OperationType = "c2b"
	OpB2C          OperationType = "b2c"
	OpBulkTransfer OperationType = "bulk_transfer"
	OpBalance      OperationType = "balance"
	OpStatus       OperationType = "status"
)

// Credential field definitions for provider setup
type CredentialField struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"` // text, password, select
	Required    bool   `json:"required"`
	Options     []string `json:"options,omitempty"` // for select fields
}

// STK Push (Customer initiated payments)
type STKPushReq struct {
	Amount           int64  `json:"amount"`
	PhoneNumber      string `json:"phone_number"`
	AccountReference string `json:"account_reference"`
	Description      string `json:"description"`
	CallbackURL      string `json:"callback_url"`
}

type STKPushResp struct {
	ExternalID        string `json:"external_id"`
	Status            string `json:"status"`
	Message           string `json:"message"`
	TransactionID     string `json:"transaction_id,omitempty"`
	ProviderReference string `json:"provider_reference,omitempty"`
}

// B2C (Business to Customer transfers)
type B2CReq struct {
	Amount      int64  `json:"amount"`
	PhoneNumber string `json:"phone_number"`
	CommandID   string `json:"command_id,omitempty"` // SalaryPayment, BusinessPayment, etc.
	Occasion    string `json:"occasion,omitempty"`
	Description string `json:"description"`
	ResultURL   string `json:"result_url"`
	TimeoutURL  string `json:"timeout_url"`
}

type B2CResp struct {
	ExternalID        string `json:"external_id"`
	Status            string `json:"status"`
	Message           string `json:"message"`
	ProviderReference string `json:"provider_reference,omitempty"`
}

// Bulk Transfer
type BulkTransferReq struct {
	Transfers []BulkTransferItem `json:"transfers"`
	BatchID   string            `json:"batch_id"`
}

type BulkTransferItem struct {
	PhoneNumber string `json:"phone_number"`
	Amount      int64  `json:"amount"`
	Reference   string `json:"reference"`
	Description string `json:"description"`
}

type BulkTransferResp struct {
	BatchID             string `json:"batch_id"`
	ProcessedCount      int    `json:"processed_count"`
	FailedCount         int    `json:"failed_count"`
	ResponseDescription string `json:"response_description"`
}

// Balance inquiry
type BalanceResp struct {
	ExternalID       string `json:"external_id"`
	Status           string `json:"status"`
	Message          string `json:"message"`
	AccountBalance   string `json:"account_balance,omitempty"`
	AvailableBalance string `json:"available_balance,omitempty"`
	Currency         string `json:"currency,omitempty"`
}

// Transaction status
type StatusResp struct {
	ExternalID     string `json:"external_id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	ConversationID string `json:"conversation_id,omitempty"`
	TransactionID  string `json:"transaction_id,omitempty"`
}

// Re-export core types for convenience
// Use domain event types
type EventType = event.Type
type Event = event.Event

// Re-export event type constants
const (
	EventSTK         = event.TypeSTK
	EventC2B         = event.TypeC2B
	EventB2C         = event.TypeB2C
	EventBalance     = event.TypeBalance
	EventBulkTransfer = event.TypeBulkTransfer
)

// Transaction status constants
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusTimeout   = "timeout"
)

// Common error types
type ProviderError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	ProviderErr string `json:"provider_error,omitempty"`
}

func (e *ProviderError) Error() string {
	if e.ProviderErr != "" {
		return e.Message + ": " + e.ProviderErr
	}
	return e.Message
}

// Error codes
const (
	ErrInvalidCredentials = "invalid_credentials"
	ErrInsufficientFunds  = "insufficient_funds"
	ErrInvalidPhone       = "invalid_phone"
	ErrInvalidAmount      = "invalid_amount"
	ErrProviderTimeout    = "provider_timeout"
	ErrProviderDown       = "provider_down"
	ErrDuplicateRequest   = "duplicate_request"
	ErrUnknownError       = "unknown_error"
)