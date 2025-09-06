package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	middlewarex "paymatch/internal/http/middleware"
	"paymatch/internal/store/postgres"

	"github.com/jackc/pgx/v5/pgxpool"
)

type replayReq struct {
	EventIDs []int64 `json:"eventIds,omitempty"`
	SinceISO string  `json:"since,omitempty"` // RFC3339
	UntilISO string  `json:"until,omitempty"` // RFC3339
	Max      int     `json:"max,omitempty"`   // default 200, max 1000
}

func ReplayEvents(repo *postgres.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := middlewarex.TenantID(r.Context())
		if !ok {
			http.Error(w, "tenant not found", 401)
			return
		}

		var in replayReq
		_ = json.NewDecoder(r.Body).Decode(&in)

		count := 0
		if len(in.EventIDs) > 0 {
			for _, id := range in.EventIDs {
				if err := repo.EnqueueEvent(r.Context(), tenantID, id); err == nil {
					count++
				}
			}
		} else {
			max := in.Max
			if max <= 0 || max > 1000 {
				max = 200
			}
			var since, until *time.Time
			if in.SinceISO != "" {
				if t, err := time.Parse(time.RFC3339, in.SinceISO); err == nil {
					since = &t
				}
			}
			if in.UntilISO != "" {
				if t, err := time.Parse(time.RFC3339, in.UntilISO); err == nil {
					until = &t
				}
			}
			count = replayByWindow(r.Context(), repo.DB(), tenantID, since, until, max, repo)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"requeued": count})
	}
}

func replayByWindow(ctx context.Context, db *pgxpool.Pool, tenantID int64, since, until *time.Time, max int, repo *postgres.Repo) int {
	q := `
	  SELECT id FROM payment_events
	   WHERE tenant_id=$1
	     AND ($2::timestamptz IS NULL OR received_at >= $2)
	     AND ($3::timestamptz IS NULL OR received_at <= $3)
	   ORDER BY received_at ASC
	   LIMIT $4`
	rows, err := db.Query(ctx, q, tenantID, since, until, max)
	if err != nil {
		return 0
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			if err := repo.EnqueueEvent(ctx, tenantID, id); err == nil {
				count++
			}
		}
	}
	return count
}
