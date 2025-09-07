package event

import (
	"fmt"
	"strings"
	"time"
)

// Event represents a payment event from a provider
type Event struct {
	ID                   int64
	TenantID             int64
	ProviderCredentialID int64
	Type                 Type
	ExternalID           string
	Amount               int64
	MSISDN               string
	InvoiceRef           string
	TransactionID        string
	Status               string
	ResponseDescription  string
	RawJSON              []byte
	ReceivedAt           time.Time
	ProcessedAt          *time.Time
	ProcessingStatus     ProcessingStatus
}

// Type represents different types of payment events
type Type string

const (
	TypeSTK         Type = "stk"
	TypeC2B         Type = "c2b"
	TypeB2C         Type = "b2c"
	TypeBalance     Type = "balance"
	TypeBulkTransfer Type = "bulk_transfer"
)

// ProcessingStatus represents the event processing status
type ProcessingStatus string

const (
	ProcessingPending   ProcessingStatus = "pending"
	ProcessingQueued    ProcessingStatus = "queued"
	ProcessingCompleted ProcessingStatus = "completed"
	ProcessingFailed    ProcessingStatus = "failed"
)

// NewEvent creates a new event with validation
func NewEvent(tenantID, credentialID int64, eventType Type, externalID string, rawJSON []byte) (*Event, error) {
	if err := validateEventCreation(tenantID, credentialID, eventType, externalID); err != nil {
		return nil, err
	}
	
	return &Event{
		TenantID:             tenantID,
		ProviderCredentialID: credentialID,
		Type:                 eventType,
		ExternalID:           externalID,
		RawJSON:              rawJSON,
		ReceivedAt:           time.Now(),
		ProcessingStatus:     ProcessingPending,
	}, nil
}

// UpdateProcessingStatus updates the event processing status
func (e *Event) UpdateProcessingStatus(status ProcessingStatus) error {
	if !e.CanChangeStatus(status) {
		return fmt.Errorf("cannot change status from %s to %s", e.ProcessingStatus, status)
	}
	
	e.ProcessingStatus = status
	
	if status == ProcessingCompleted || status == ProcessingFailed {
		now := time.Now()
		e.ProcessedAt = &now
	}
	
	return nil
}

// MarkForReprocessing marks the event for reprocessing
func (e *Event) MarkForReprocessing() error {
	if e.ProcessingStatus == ProcessingPending {
		return fmt.Errorf("event is already pending processing")
	}
	
	e.ProcessingStatus = ProcessingQueued
	e.ProcessedAt = nil
	
	return nil
}

// IsProcessed checks if the event has been processed
func (e *Event) IsProcessed() bool {
	return e.ProcessingStatus == ProcessingCompleted || e.ProcessingStatus == ProcessingFailed
}

// CanChangeStatus checks if status can be changed
func (e *Event) CanChangeStatus(newStatus ProcessingStatus) bool {
	switch e.ProcessingStatus {
	case ProcessingPending:
		return newStatus == ProcessingQueued || newStatus == ProcessingCompleted || newStatus == ProcessingFailed
	case ProcessingQueued:
		return newStatus == ProcessingCompleted || newStatus == ProcessingFailed
	case ProcessingCompleted:
		return newStatus == ProcessingQueued // Allow reprocessing
	case ProcessingFailed:
		return newStatus == ProcessingQueued // Allow retry
	}
	return false
}

// EnrichWithPaymentData enriches event with payment-specific data
func (e *Event) EnrichWithPaymentData(amount int64, msisdn, invoiceRef, transactionID, status, description string) {
	e.Amount = amount
	e.MSISDN = msisdn
	e.InvoiceRef = invoiceRef
	e.TransactionID = transactionID
	e.Status = status
	e.ResponseDescription = description
}

// validateEventCreation validates event creation parameters
func validateEventCreation(tenantID, credentialID int64, eventType Type, externalID string) error {
	if tenantID <= 0 {
		return fmt.Errorf("invalid tenant ID: %d", tenantID)
	}
	
	if credentialID <= 0 {
		return fmt.Errorf("invalid credential ID: %d", credentialID)
	}
	
	if !isValidEventType(eventType) {
		return fmt.Errorf("invalid event type: %s", eventType)
	}
	
	if strings.TrimSpace(externalID) == "" {
		return fmt.Errorf("external ID is required")
	}
	
	return nil
}

// isValidEventType checks if event type is valid
func isValidEventType(eventType Type) bool {
	validTypes := []Type{TypeSTK, TypeC2B, TypeB2C, TypeBalance, TypeBulkTransfer}
	for _, valid := range validTypes {
		if eventType == valid {
			return true
		}
	}
	return false
}