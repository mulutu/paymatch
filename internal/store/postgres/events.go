package postgres

import (
	"context"
	"time"

	"paymatch/internal/provider"

	"github.com/jackc/pgx/v5"
)

type EventRow struct {
	ID                   int64
	TenantID             int64
	ProviderCredentialID int64
	EventType            string
	ExternalID           string
	PayloadJSON          []byte
	ReceivedAt           time.Time
}

type EventListItem struct {
	ID         int64     `json:"id"`
	Type       string    `json:"type"`
	ExternalID string    `json:"externalId"`
	ReceivedAt time.Time `json:"receivedAt"`
	Status     *string   `json:"status"`
}

// SaveEvent upserts by (tenant,event_type,external_id) and returns id.
func (r *Repo) SaveEvent(ctx context.Context, tenantID, credID int64, evt provider.Event) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO payment_events
			(tenant_id, provider_credential_id, event_type, external_id,
			 payload_json, invoice_ref, msisdn, amount)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (tenant_id, event_type, external_id) DO UPDATE
		  SET payload_json = EXCLUDED.payload_json,
		      invoice_ref  = COALESCE(NULLIF(EXCLUDED.invoice_ref,''), payment_events.invoice_ref),
		      msisdn       = COALESCE(NULLIF(EXCLUDED.msisdn,''),      payment_events.msisdn),
		      amount       = CASE WHEN EXCLUDED.amount>0 THEN EXCLUDED.amount ELSE payment_events.amount END
		RETURNING id`,
		tenantID, credID, string(evt.Type), evt.ExternalID,
		evt.RawJSON, evt.InvoiceRef, evt.MSISDN, evt.Amount,
	).Scan(&id)
	return id, err
}

func (r *Repo) FetchUnprocessedEvents(ctx context.Context, limit int) ([]EventRow, error) {
	rows, err := r.db.Query(ctx, `SELECT id, tenant_id, provider_credential_id, event_type, external_id, payload_json, received_at
		FROM payment_events WHERE processed_at IS NULL ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventRow
	for rows.Next() {
		var e EventRow
		if err := rows.Scan(&e.ID, &e.TenantID, &e.ProviderCredentialID, &e.EventType, &e.ExternalID, &e.PayloadJSON, &e.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkEventProcessedTx: mark processed within an existing tx
func (r *Repo) MarkEventProcessedTx(ctx context.Context, tx pgx.Tx, id int64, status string) error {
	_, err := tx.Exec(ctx, `UPDATE payment_events SET processed_at=now(), status=$2 WHERE id=$1`, id, status)
	return err
}

func (r *Repo) ListEvents(ctx context.Context, tenantID int64, limit, offset int) ([]EventListItem, error) {
	rows, err := r.db.Query(ctx, `SELECT id, event_type, external_id, received_at, status
		FROM payment_events WHERE tenant_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventListItem
	for rows.Next() {
		var it EventListItem
		if err := rows.Scan(&it.ID, &it.Type, &it.ExternalID, &it.ReceivedAt, &it.Status); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
