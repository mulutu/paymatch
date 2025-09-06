package postgres

import (
	"context"
	"strconv"
	"time"
)

type DueEvent struct {
	QueueID              int64
	EventID              int64
	TenantID             int64
	ProviderCredentialID int64 // <-- needed by worker.UpsertPayment(...)
	Type                 string
	ExtID                string
	RawJSON              []byte
}

func (r *Repo) EnqueueEvent(ctx context.Context, tenantID, eventID int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO event_queue (tenant_id, event_id)
		VALUES ($1, $2)
		ON CONFLICT (event_id) DO UPDATE
		  SET status='pending',
		      attempts=0,
		      next_attempt_at=now(),
		      updated_at=now()`,
		tenantID, eventID,
	)
	return err
}

func (r *Repo) FetchDueEvents(ctx context.Context, limit int) ([]DueEvent, error) {
	rows, err := r.db.Query(ctx, `
		WITH due AS (
		  SELECT id FROM event_queue
		  WHERE status IN ('pending','failed')
		    AND next_attempt_at <= now()
		  ORDER BY next_attempt_at ASC
		  LIMIT $1
		  FOR UPDATE SKIP LOCKED
		)
		UPDATE event_queue q
		   SET status='delivering', updated_at=now()
		  FROM due d
		 WHERE q.id = d.id
		RETURNING q.id, q.event_id, q.tenant_id
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type picked struct{ qid, eid, tid int64 }
	var ids []picked
	for rows.Next() {
		var qid, eid, tid int64
		if err := rows.Scan(&qid, &eid, &tid); err != nil {
			return nil, err
		}
		ids = append(ids, picked{qid, eid, tid})
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Build IN list for event details
	evIDs := make([]any, 0, len(ids))
	ph := ""
	for i, p := range ids {
		if i > 0 {
			ph += ","
		}
		ph += "$" + strconv.Itoa(i+1)
		evIDs = append(evIDs, p.eid)
	}

	// IMPORTANT: select provider_credential_id
	evRows, err := r.db.Query(ctx, `
	  SELECT e.id,
	         e.tenant_id,
	         e.provider_credential_id,
	         e.event_type,
	         e.external_id,
	         e.payload_json
	    FROM payment_events e
	   WHERE e.id IN (`+ph+`)`, evIDs...)
	if err != nil {
		return nil, err
	}
	defer evRows.Close()

	meta := map[int64]DueEvent{}
	for evRows.Next() {
		var eid, tid, pcid int64
		var et, ext string
		var raw []byte
		if err := evRows.Scan(&eid, &tid, &pcid, &et, &ext, &raw); err != nil {
			return nil, err
		}
		meta[eid] = DueEvent{
			EventID:              eid,
			TenantID:             tid,
			ProviderCredentialID: pcid, // <-- populate
			Type:                 et,
			ExtID:                ext,
			RawJSON:              raw,
		}
	}

	// Stitch queue ids
	out := make([]DueEvent, 0, len(ids))
	for _, p := range ids {
		di := meta[p.eid]
		di.QueueID = p.qid
		out = append(out, di)
	}
	return out, nil
}

func (r *Repo) MarkEventDone(ctx context.Context, queueID, eventID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE event_queue
		   SET status='done', updated_at=now()
		 WHERE id=$1;

		UPDATE payment_events
		   SET processed_at = COALESCE(processed_at, now()),
		       status='ok'
		 WHERE id=$2 AND status IS DISTINCT FROM 'ok'`,
		queueID, eventID,
	)
	return err
}

func (r *Repo) MarkEventFailed(ctx context.Context, queueID, eventID int64, attempts int, lastErr string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE event_queue
		   SET status='failed',
		       attempts=$3,
		       next_attempt_at=now()+$4::interval,
		       last_error=$5,
		       updated_at=now()
		 WHERE id=$1;

		UPDATE payment_events
		   SET status='error'
		 WHERE id=$2 AND status IS DISTINCT FROM 'error'`,
		queueID, eventID, attempts+1, backoffAttempt(attempts).String(), truncate(lastErr, 800),
	)
	return err
}

func backoffAttempt(n int) time.Duration {
	switch {
	case n <= 0:
		return time.Minute
	case n == 1:
		return 5 * time.Minute
	case n == 2:
		return 30 * time.Minute
	case n == 3:
		return 2 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
