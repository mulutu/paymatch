package event

import (
	"context"
	"time"

	"paymatch/internal/store/repositories"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReplayService handles event replay operations
type ReplayService struct {
	eventRepo repositories.EventRepository
	db        *pgxpool.Pool // For direct SQL queries in replayByTimeWindow
}

// NewReplayService creates a new event replay service
func NewReplayService(eventRepo repositories.EventRepository, db *pgxpool.Pool) *ReplayService {
	return &ReplayService{
		eventRepo: eventRepo,
		db:        db,
	}
}

// ReplayRequest represents an event replay request
type ReplayRequest struct {
	EventIDs []int64    `json:"eventIds,omitempty"`
	Since    *time.Time `json:"since,omitempty"`
	Until    *time.Time `json:"until,omitempty"`
	Max      int        `json:"max,omitempty"`
}

// ReplayResponse represents the result of an event replay operation
type ReplayResponse struct {
	RequeuedCount int `json:"requeued"`
}

// ReplayEvents replays events for a tenant based on the request parameters
func (s *ReplayService) ReplayEvents(ctx context.Context, tenantID int64, req ReplayRequest) (*ReplayResponse, error) {
	count := 0
	
	if len(req.EventIDs) > 0 {
		// Replay specific events by ID
		for _, id := range req.EventIDs {
			if err := s.eventRepo.MarkForReprocessing(ctx, tenantID, id); err == nil {
				count++
			}
		}
	} else {
		// Replay events by time window
		max := req.Max
		if max <= 0 || max > 1000 {
			max = 200
		}
		
		count = s.replayByTimeWindow(ctx, tenantID, req.Since, req.Until, max)
	}
	
	return &ReplayResponse{RequeuedCount: count}, nil
}

// replayByTimeWindow replays events within a time window
func (s *ReplayService) replayByTimeWindow(ctx context.Context, tenantID int64, since, until *time.Time, max int) int {
	query := `
		SELECT id FROM payment_events
		WHERE tenant_id=$1
		  AND ($2::timestamptz IS NULL OR received_at >= $2)
		  AND ($3::timestamptz IS NULL OR received_at <= $3)
		ORDER BY received_at ASC
		LIMIT $4`
		
	rows, err := s.db.Query(ctx, query, tenantID, since, until, max)
	if err != nil {
		return 0
	}
	defer rows.Close()
	
	count := 0
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			if err := s.eventRepo.MarkForReprocessing(ctx, tenantID, id); err == nil {
				count++
			}
		}
	}
	
	return count
}