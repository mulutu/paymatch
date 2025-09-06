package postgres

import (
	"context"
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

// EnqueueEvent: bring an existing event back to the processing queue by
// clearing processed_at and tagging status. This is used ONLY when a user
// explicitly asks for replay or the system issues a retry.
func (r *Repo) EnqueueEvent(ctx context.Context, tenantID, eventID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE payment_events
		   SET processed_at = NULL,
		       status = 'queued',
		       updated_at = now()
		 WHERE id = $1 AND tenant_id = $2`,
		eventID, tenantID,
	)
	return err
}
