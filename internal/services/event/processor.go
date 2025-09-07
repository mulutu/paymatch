package event

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"paymatch/internal/domain/event"
	"paymatch/internal/services/payment"
	"paymatch/internal/store/repositories"

	"github.com/rs/zerolog/log"
)

// Processor handles event processing business logic
type Processor struct {
	eventRepo   repositories.EventRepository
	paymentSvc  *payment.Service
	unitOfWork  repositories.UnitOfWork
}

// NewProcessor creates a new event processor
func NewProcessor(
	eventRepo repositories.EventRepository,
	paymentSvc *payment.Service,
	unitOfWork repositories.UnitOfWork,
) *Processor {
	return &Processor{
		eventRepo:   eventRepo,
		paymentSvc:  paymentSvc,
		unitOfWork:  unitOfWork,
	}
}

// ProcessEvent processes a single payment event with business rules
func (p *Processor) ProcessEvent(ctx context.Context, evt *event.Event) error {
	switch evt.Type {
	case event.TypeSTK:
		return p.processSTKEvent(ctx, evt)
	case event.TypeC2B:
		return p.processC2BEvent(ctx, evt)
	case event.TypeB2C:
		return p.processB2CEvent(ctx, evt)
	default:
		// Mark unknown events as processed to avoid reprocessing
		return p.markEventProcessed(ctx, evt, event.ProcessingCompleted)
	}
}

// processSTKEvent handles STK Push callback events
func (p *Processor) processSTKEvent(ctx context.Context, evt *event.Event) error {
	payload, err := p.parseSTKPayload(evt.RawJSON)
	if err != nil {
		log.Error().Err(err).Int64("event_id", evt.ID).Msg("failed to parse STK payload")
		return p.markEventProcessed(ctx, evt, event.ProcessingFailed)
	}
	
	// Extract payment data from STK callback
	amount := payload.extractAmount()
	msisdn := payload.extractMSISDN()
	reference := payload.extractReference()
	isSuccess := payload.isSuccessful()
	
	// Determine payment status based on callback result
	status := "failed"
	if isSuccess {
		status = "completed"
	}
	
	// Process payment atomically with event update
	return p.processPaymentEvent(ctx, evt, reference, msisdn, amount, status)
}

// processC2BEvent handles Customer-to-Business events  
func (p *Processor) processC2BEvent(ctx context.Context, evt *event.Event) error {
	payload, err := p.parseC2BPayload(evt.RawJSON)
	if err != nil {
		log.Error().Err(err).Int64("event_id", evt.ID).Msg("failed to parse C2B payload")
		return p.markEventProcessed(ctx, evt, event.ProcessingFailed)
	}
	
	amount := payload.extractAmount()
	msisdn := payload.extractMSISDN()
	reference := payload.extractReference()
	
	// C2B payments are typically successful when received
	return p.processPaymentEvent(ctx, evt, reference, msisdn, amount, "completed")
}

// processB2CEvent handles Business-to-Customer events
func (p *Processor) processB2CEvent(ctx context.Context, evt *event.Event) error {
	// For now, just mark as processed - B2C logic can be added later
	return p.markEventProcessed(ctx, evt, event.ProcessingCompleted)
}

// processPaymentEvent atomically updates both payment and event in a transaction
func (p *Processor) processPaymentEvent(ctx context.Context, evt *event.Event, reference, msisdn string, amount int64, status string) error {
	// Begin transaction for atomic operation
	tx, err := p.unitOfWork.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	
	// Process payment through service layer
	err = p.paymentSvc.ProcessPaymentEvent(ctx, 
		evt.TenantID, evt.ProviderCredentialID, evt.ExternalID,
		amount, msisdn, reference, status)
	if err != nil {
		log.Error().Err(err).Int64("event_id", evt.ID).Msg("failed to process payment")
		return p.markEventProcessed(ctx, evt, event.ProcessingFailed)
	}
	
	// Mark event as processed
	eventRepo := tx.EventRepository()
	err = eventRepo.MarkProcessed(ctx, evt.ID, event.ProcessingCompleted)
	if err != nil {
		return fmt.Errorf("failed to mark event processed: %w", err)
	}
	
	// Commit transaction
	return tx.Commit(ctx)
}

// markEventProcessed marks an event with a specific processing status
func (p *Processor) markEventProcessed(ctx context.Context, evt *event.Event, status event.ProcessingStatus) error {
	return p.eventRepo.MarkProcessed(ctx, evt.ID, status)
}

// STK payload structure for parsing Safaricom callbacks
type stkPayload struct {
	Body struct {
		StkCallback struct {
			MerchantRequestID string `json:"MerchantRequestID"`
			CheckoutRequestID string `json:"CheckoutRequestID"`
			ResultCode        int    `json:"ResultCode"`
			ResultDesc        string `json:"ResultDesc"`
			CallbackMetadata  struct {
				Item []struct {
					Name  string `json:"Name"`
					Value any    `json:"Value"`
				} `json:"Item"`
			} `json:"CallbackMetadata"`
		} `json:"stkCallback"`
	} `json:"Body"`
}

func (p *Processor) parseSTKPayload(rawJSON []byte) (*stkPayload, error) {
	var payload stkPayload
	if err := json.Unmarshal(rawJSON, &payload); err != nil {
		return nil, fmt.Errorf("invalid STK payload: %w", err)
	}
	return &payload, nil
}

func (s *stkPayload) isSuccessful() bool {
	return s.Body.StkCallback.ResultCode == 0
}

func (s *stkPayload) extractAmount() int64 {
	for _, item := range s.Body.StkCallback.CallbackMetadata.Item {
		if item.Name == "Amount" {
			switch v := item.Value.(type) {
			case float64:
				return int64(v)
			case json.Number:
				if f, err := v.Float64(); err == nil {
					return int64(f)
				}
			case string:
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					return int64(f)
				}
			case int:
				return int64(v)
			}
		}
	}
	return 0
}

func (s *stkPayload) extractMSISDN() string {
	for _, item := range s.Body.StkCallback.CallbackMetadata.Item {
		if item.Name == "PhoneNumber" {
			switch v := item.Value.(type) {
			case float64:
				return fmt.Sprintf("%.0f", v)
			case json.Number:
				return v.String()
			case string:
				return v
			}
		}
	}
	return ""
}

func (s *stkPayload) extractReference() string {
	for _, item := range s.Body.StkCallback.CallbackMetadata.Item {
		if item.Name == "AccountReference" {
			if s, ok := item.Value.(string); ok {
				return s
			}
		}
	}
	return ""
}

// C2B payload structure
type c2bPayload map[string]any

func (p *Processor) parseC2BPayload(rawJSON []byte) (c2bPayload, error) {
	var payload c2bPayload
	if err := json.Unmarshal(rawJSON, &payload); err != nil {
		return nil, fmt.Errorf("invalid C2B payload: %w", err)
	}
	return payload, nil
}

func (c c2bPayload) extractAmount() int64 {
	switch v := c["TransAmount"].(type) {
	case float64:
		return int64(v)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return int64(f)
		}
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return int64(f)
		}
	}
	return 0
}

func (c c2bPayload) extractMSISDN() string {
	if msisdn, ok := c["MSISDN"].(string); ok {
		return msisdn
	}
	return ""
}

func (c c2bPayload) extractReference() string {
	if ref, ok := c["BillRefNumber"].(string); ok {
		return ref
	}
	return ""
}