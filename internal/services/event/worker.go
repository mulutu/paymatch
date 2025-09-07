package event

import (
	"context"
	"time"

	"paymatch/internal/domain/event"
	"paymatch/internal/store/repositories"

	"github.com/rs/zerolog/log"
)

// Worker handles background processing of payment events
type Worker struct {
	eventRepo  repositories.EventRepository
	processor  *Processor
	pollEvery  time.Duration
	batchSize  int
}

// NewWorker creates a new event processing worker
func NewWorker(
	eventRepo repositories.EventRepository,
	processor *Processor,
	pollEvery time.Duration,
	batchSize int,
) *Worker {
	if pollEvery == 0 {
		pollEvery = 2 * time.Second
	}
	if batchSize == 0 {
		batchSize = 50
	}
	
	return &Worker{
		eventRepo:  eventRepo,
		processor:  processor,
		pollEvery:  pollEvery,
		batchSize:  batchSize,
	}
}

// Run starts the worker and processes events until context is cancelled
func (w *Worker) Run(ctx context.Context) {
	log.Info().
		Dur("poll_every", w.pollEvery).
		Int("batch_size", w.batchSize).
		Msg("event processing worker started")
	
	ticker := time.NewTicker(w.pollEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("event processing worker stopping")
			return
		case <-ticker.C:
			if err := w.processNextBatch(ctx); err != nil {
				log.Error().Err(err).Msg("error processing event batch")
			}
		}
	}
}

// processNextBatch fetches and processes the next batch of unprocessed events
func (w *Worker) processNextBatch(ctx context.Context) error {
	events, err := w.eventRepo.FindUnprocessed(ctx, w.batchSize)
	if err != nil {
		return err
	}
	
	if len(events) == 0 {
		return nil // No events to process
	}
	
	log.Debug().Int("count", len(events)).Msg("processing event batch")
	
	for _, event := range events {
		if err := w.processEvent(ctx, event); err != nil {
			log.Error().
				Err(err).
				Int64("event_id", event.ID).
				Int64("tenant_id", event.TenantID).
				Str("type", string(event.Type)).
				Str("external_id", event.ExternalID).
				Msg("failed to process event")
			// Continue processing other events even if one fails
		}
	}
	
	return nil
}

// processEvent processes a single event with error handling and logging
func (w *Worker) processEvent(ctx context.Context, event *event.Event) error {
	log.Debug().
		Int64("event_id", event.ID).
		Int64("tenant_id", event.TenantID).
		Str("type", string(event.Type)).
		Str("external_id", event.ExternalID).
		Msg("processing event")
	
	start := time.Now()
	err := w.processor.ProcessEvent(ctx, event)
	duration := time.Since(start)
	
	if err != nil {
		log.Error().
			Err(err).
			Int64("event_id", event.ID).
			Dur("duration", duration).
			Msg("event processing failed")
		return err
	}
	
	log.Info().
		Int64("event_id", event.ID).
		Int64("tenant_id", event.TenantID).
		Str("type", string(event.Type)).
		Dur("duration", duration).
		Msg("event processed successfully")
	
	return nil
}

// ProcessEventByID processes a specific event by ID (useful for manual reprocessing)
func (w *Worker) ProcessEventByID(ctx context.Context, eventID int64) error {
	event, err := w.eventRepo.FindByID(ctx, eventID)
	if err != nil {
		return err
	}
	
	return w.processor.ProcessEvent(ctx, event)
}

// ReprocessEvent marks an event for reprocessing and processes it
func (w *Worker) ReprocessEvent(ctx context.Context, tenantID, eventID int64) error {
	// Mark for reprocessing
	if err := w.eventRepo.MarkForReprocessing(ctx, tenantID, eventID); err != nil {
		return err
	}
	
	// Process the event
	return w.ProcessEventByID(ctx, eventID)
}