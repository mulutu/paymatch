package postgres

import (
	"context"
	"database/sql"
	
	"paymatch/internal/domain/event"
	
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// eventRepository implements EventRepository interface with pure data access
type eventRepository struct {
	db *pgxpool.Pool
}

// NewEventRepository creates a new event repository
func NewEventRepository(db *pgxpool.Pool) *eventRepository {
	return &eventRepository{db: db}
}

// Save saves an event (insert or update)
func (r *eventRepository) Save(ctx context.Context, e *event.Event) error {
	if e.ID == 0 {
		return r.insert(ctx, e)
	}
	return r.update(ctx, e)
}

// FindByID finds an event by ID
func (r *eventRepository) FindByID(ctx context.Context, id int64) (*event.Event, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, provider_credential_id, event_type, external_id, amount, 
		       msisdn, invoice_ref, transaction_id, status, response_description, 
		       payload_json, received_at, processed_at, processing_status
		FROM payment_events 
		WHERE id = $1`, id)
	
	return r.scanEvent(row)
}

// FindUnprocessed finds unprocessed events
func (r *eventRepository) FindUnprocessed(ctx context.Context, limit int) ([]*event.Event, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, provider_credential_id, event_type, external_id, amount, 
		       msisdn, invoice_ref, transaction_id, status, response_description, 
		       payload_json, received_at, processed_at, processing_status
		FROM payment_events 
		WHERE processing_status IN ('pending', 'queued')
		ORDER BY received_at ASC 
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	return r.scanEvents(rows)
}

// FindByTenantID finds events by tenant with pagination
func (r *eventRepository) FindByTenantID(ctx context.Context, tenantID int64, limit, offset int) ([]*event.Event, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, provider_credential_id, event_type, external_id, amount, 
		       msisdn, invoice_ref, transaction_id, status, response_description, 
		       payload_json, received_at, processed_at, processing_status
		FROM payment_events 
		WHERE tenant_id = $1 
		ORDER BY received_at DESC 
		LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	return r.scanEvents(rows)
}

// MarkProcessed marks an event as processed with status
func (r *eventRepository) MarkProcessed(ctx context.Context, id int64, status event.ProcessingStatus) error {
	_, err := r.db.Exec(ctx, `
		UPDATE payment_events 
		SET processing_status = $1, processed_at = now(), updated_at = now()
		WHERE id = $2`, string(status), id)
	return err
}

// MarkForReprocessing marks an event for reprocessing
func (r *eventRepository) MarkForReprocessing(ctx context.Context, tenantID, eventID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE payment_events 
		SET processing_status = 'queued', processed_at = NULL, updated_at = now()
		WHERE id = $1 AND tenant_id = $2`, eventID, tenantID)
	return err
}

// insert creates a new event record
func (r *eventRepository) insert(ctx context.Context, e *event.Event) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO payment_events (tenant_id, provider_credential_id, event_type, external_id, 
		                           amount, msisdn, invoice_ref, transaction_id, status, 
		                           response_description, payload_json, received_at, processing_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (tenant_id, event_type, external_id) DO UPDATE SET
		    payload_json = EXCLUDED.payload_json,
		    amount = CASE WHEN EXCLUDED.amount > 0 THEN EXCLUDED.amount ELSE payment_events.amount END,
		    msisdn = COALESCE(NULLIF(EXCLUDED.msisdn, ''), payment_events.msisdn),
		    invoice_ref = COALESCE(NULLIF(EXCLUDED.invoice_ref, ''), payment_events.invoice_ref),
		    transaction_id = COALESCE(NULLIF(EXCLUDED.transaction_id, ''), payment_events.transaction_id),
		    status = COALESCE(NULLIF(EXCLUDED.status, ''), payment_events.status),
		    response_description = COALESCE(NULLIF(EXCLUDED.response_description, ''), payment_events.response_description),
		    updated_at = now()
		RETURNING id`,
		e.TenantID, e.ProviderCredentialID, string(e.Type), e.ExternalID,
		e.Amount, e.MSISDN, e.InvoiceRef, e.TransactionID, e.Status,
		e.ResponseDescription, e.RawJSON, e.ReceivedAt, string(e.ProcessingStatus)).Scan(&e.ID)
	
	return err
}

// update modifies an existing event record
func (r *eventRepository) update(ctx context.Context, e *event.Event) error {
	_, err := r.db.Exec(ctx, `
		UPDATE payment_events 
		SET provider_credential_id = $1, event_type = $2, external_id = $3,
		    amount = $4, msisdn = $5, invoice_ref = $6, transaction_id = $7,
		    status = $8, response_description = $9, payload_json = $10,
		    processed_at = $11, processing_status = $12, updated_at = now()
		WHERE id = $13`,
		e.ProviderCredentialID, string(e.Type), e.ExternalID,
		e.Amount, e.MSISDN, e.InvoiceRef, e.TransactionID,
		e.Status, e.ResponseDescription, e.RawJSON,
		e.ProcessedAt, string(e.ProcessingStatus), e.ID)
	
	return err
}

// scanEvent scans a single row into event domain object
func (r *eventRepository) scanEvent(row pgx.Row) (*event.Event, error) {
	var e event.Event
	var amount sql.NullInt64
	var msisdn, invoiceRef, transactionID, status, responseDesc sql.NullString
	var processedAt sql.NullTime
	
	err := row.Scan(
		&e.ID, &e.TenantID, &e.ProviderCredentialID, &e.Type, &e.ExternalID,
		&amount, &msisdn, &invoiceRef, &transactionID, &status, &responseDesc,
		&e.RawJSON, &e.ReceivedAt, &processedAt, &e.ProcessingStatus)
	if err != nil {
		return nil, err
	}
	
	if amount.Valid {
		e.Amount = amount.Int64
	}
	if msisdn.Valid {
		e.MSISDN = msisdn.String
	}
	if invoiceRef.Valid {
		e.InvoiceRef = invoiceRef.String
	}
	if transactionID.Valid {
		e.TransactionID = transactionID.String
	}
	if status.Valid {
		e.Status = status.String
	}
	if responseDesc.Valid {
		e.ResponseDescription = responseDesc.String
	}
	if processedAt.Valid {
		e.ProcessedAt = &processedAt.Time
	}
	
	return &e, nil
}

// scanEvents scans multiple rows into event domain objects
func (r *eventRepository) scanEvents(rows pgx.Rows) ([]*event.Event, error) {
	var events []*event.Event
	for rows.Next() {
		e, err := r.scanEventFromRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	
	return events, rows.Err()
}

// scanEventFromRows scans rows into event domain object
func (r *eventRepository) scanEventFromRows(rows pgx.Rows) (*event.Event, error) {
	var e event.Event
	var amount sql.NullInt64
	var msisdn, invoiceRef, transactionID, status, responseDesc sql.NullString
	var processedAt sql.NullTime
	
	err := rows.Scan(
		&e.ID, &e.TenantID, &e.ProviderCredentialID, &e.Type, &e.ExternalID,
		&amount, &msisdn, &invoiceRef, &transactionID, &status, &responseDesc,
		&e.RawJSON, &e.ReceivedAt, &processedAt, &e.ProcessingStatus)
	if err != nil {
		return nil, err
	}
	
	if amount.Valid {
		e.Amount = amount.Int64
	}
	if msisdn.Valid {
		e.MSISDN = msisdn.String
	}
	if invoiceRef.Valid {
		e.InvoiceRef = invoiceRef.String
	}
	if transactionID.Valid {
		e.TransactionID = transactionID.String
	}
	if status.Valid {
		e.Status = status.String
	}
	if responseDesc.Valid {
		e.ResponseDescription = responseDesc.String
	}
	if processedAt.Valid {
		e.ProcessedAt = &processedAt.Time
	}
	
	return &e, nil
}