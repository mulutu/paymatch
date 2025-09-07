package payment

import (
	"context"
	"fmt"

	"paymatch/internal/domain/payment"
	"paymatch/internal/store/repositories"
)

// Service handles payment business logic
type Service struct {
	paymentRepo repositories.PaymentRepository
	eventRepo   repositories.EventRepository
}

// NewService creates a new payment service
func NewService(paymentRepo repositories.PaymentRepository, eventRepo repositories.EventRepository) *Service {
	return &Service{
		paymentRepo: paymentRepo,
		eventRepo:   eventRepo,
	}
}

// ProcessPaymentEvent processes a payment event and updates payment state
func (s *Service) ProcessPaymentEvent(ctx context.Context, tenantID, credentialID int64, externalID string, amount int64, msisdnStr, invoice, status string) error {
	// Create MSISDN domain object with validation
	var msisdn *payment.MSISDN
	var err error
	if msisdnStr != "" {
		msisdn, err = payment.NewMSISDN(msisdnStr)
		if err != nil {
			return fmt.Errorf("invalid phone number: %w", err)
		}
	}
	
	// Find existing payment
	existingPayment, err := s.paymentRepo.FindByExternalID(ctx, tenantID, externalID)
	if err != nil {
		// Create new payment if not found
		newPayment, err := payment.NewPayment(
			tenantID,
			invoice,
			payment.Money(amount),
			payment.KES,
			payment.MethodMpesa,
			externalID,
			msisdn,
		)
		if err != nil {
			return fmt.Errorf("failed to create payment: %w", err)
		}
		
		// Update status based on event
		if status != "" {
			statusEnum := s.mapStatusFromProvider(status)
			if err := newPayment.Update("", 0, statusEnum, nil); err != nil {
				return fmt.Errorf("failed to update payment status: %w", err)
			}
		}
		
		return s.paymentRepo.Save(ctx, newPayment)
	}
	
	// Update existing payment with business rules
	statusEnum := s.mapStatusFromProvider(status)
	err = existingPayment.Update(invoice, payment.Money(amount), statusEnum, msisdn)
	if err != nil {
		return fmt.Errorf("failed to update payment: %w", err)
	}
	
	return s.paymentRepo.Save(ctx, existingPayment)
}

// CreatePendingPayment creates a new pending payment (for STK push)
func (s *Service) CreatePendingPayment(ctx context.Context, tenantID, credentialID int64, invoice string, amount int64, externalID string) error {
	// Create payment domain object
	newPayment, err := payment.NewPayment(
		tenantID,
		invoice,
		payment.Money(amount),
		payment.KES,
		payment.MethodMpesa,
		externalID,
		nil, // No MSISDN for STK push initially
	)
	if err != nil {
		return fmt.Errorf("failed to create pending payment: %w", err)
	}
	
	return s.paymentRepo.Save(ctx, newPayment)
}

// GetPaymentsByTenant retrieves payments for a tenant with pagination
func (s *Service) GetPaymentsByTenant(ctx context.Context, tenantID int64, limit, offset int) ([]*payment.Payment, error) {
	if limit <= 0 || limit > 200 {
		limit = 50 // Business rule: default pagination
	}
	if offset < 0 {
		offset = 0
	}
	
	return s.paymentRepo.FindByTenantID(ctx, tenantID, limit, offset)
}

// mapStatusFromProvider maps provider status to domain status
func (s *Service) mapStatusFromProvider(providerStatus string) payment.Status {
	switch providerStatus {
	case "completed", "0":
		return payment.StatusCompleted
	case "failed", "1":
		return payment.StatusFailed
	case "cancelled":
		return payment.StatusCancelled
	default:
		return payment.StatusPending
	}
}

// ServiceError represents a payment service error
type ServiceError struct {
	Op      string
	Message string
	Err     error
}

func (e ServiceError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("payment service %s: %s (%v)", e.Op, e.Message, e.Err)
	}
	return fmt.Sprintf("payment service %s: %s", e.Op, e.Message)
}

func (e ServiceError) Unwrap() error {
	return e.Err
}