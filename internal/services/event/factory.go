package event

import (
	"time"

	"paymatch/internal/services/payment"
	"paymatch/internal/store/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkerConfig holds configuration for the event worker
type WorkerConfig struct {
	PollInterval time.Duration
	BatchSize    int
}

// DefaultWorkerConfig returns sensible defaults for the worker
func DefaultWorkerConfig() WorkerConfig {
	return WorkerConfig{
		PollInterval: 2 * time.Second,
		BatchSize:    50,
	}
}

// NewEventProcessingSystem creates a fully configured event processing system
func NewEventProcessingSystem(
	db *pgxpool.Pool,
	paymentService *payment.Service,
	config WorkerConfig,
) (*Worker, error) {
	// Create repositories - these need to be the concrete implementations
	eventRepo := postgres.NewEventRepository(db)
	unitOfWork := postgres.NewUnitOfWork(db)
	
	// Create processor with dependencies
	processor := NewProcessor(eventRepo, paymentService, unitOfWork)
	
	// Create worker
	worker := NewWorker(eventRepo, processor, config.PollInterval, config.BatchSize)
	
	return worker, nil
}