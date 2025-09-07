package data

import (
	"context"

	"paymatch/internal/domain/event"
	"paymatch/internal/domain/payment"
	"paymatch/internal/store/repositories"
)

// Service handles data retrieval operations
type Service struct {
	paymentRepo repositories.PaymentRepository
	eventRepo   repositories.EventRepository
}

// NewService creates a new data service
func NewService(paymentRepo repositories.PaymentRepository, eventRepo repositories.EventRepository) *Service {
	return &Service{
		paymentRepo: paymentRepo,
		eventRepo:   eventRepo,
	}
}

// ListPayments retrieves paginated payment data for a tenant
func (s *Service) ListPayments(ctx context.Context, tenantID int64, req ListRequest) (*PaymentListResponse, error) {
	// Validate and normalize request
	req.Validate()
	
	// Fetch payments from repository
	payments, err := s.paymentRepo.FindByTenantID(ctx, tenantID, req.Limit, req.Offset)
	if err != nil {
		return nil, &ServiceError{Op: "list_payments", Err: err}
	}
	
	return &PaymentListResponse{
		Payments: payments,
		Limit:    req.Limit,
		Offset:   req.Offset,
	}, nil
}

// ListEvents retrieves paginated event data for a tenant
func (s *Service) ListEvents(ctx context.Context, tenantID int64, req ListRequest) (*EventListResponse, error) {
	// Validate and normalize request
	req.Validate()
	
	// Fetch events from repository
	events, err := s.eventRepo.FindByTenantID(ctx, tenantID, req.Limit, req.Offset)
	if err != nil {
		return nil, &ServiceError{Op: "list_events", Err: err}
	}
	
	return &EventListResponse{
		Events: events,
		Limit:  req.Limit,
		Offset: req.Offset,
	}, nil
}

// ServiceError represents a data service error
type ServiceError struct {
	Op  string
	Err error
}

func (e *ServiceError) Error() string {
	return "data service " + e.Op + ": " + e.Err.Error()
}

func (e *ServiceError) Unwrap() error {
	return e.Err
}

// PaymentListResponse represents paginated payment data
type PaymentListResponse struct {
	Payments []*payment.Payment `json:"payments"`
	Limit    int                `json:"limit"`
	Offset   int                `json:"offset"`
}

// EventListResponse represents paginated event data
type EventListResponse struct {
	Events []*event.Event `json:"events"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}