package postgres

import (
	"context"
	"database/sql"
	
	"paymatch/internal/domain/event"
	"paymatch/internal/domain/payment"
	"paymatch/internal/store/repositories"
	
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// unitOfWork implements UnitOfWork interface
type unitOfWork struct {
	db *pgxpool.Pool
}

// NewUnitOfWork creates a new unit of work
func NewUnitOfWork(db *pgxpool.Pool) repositories.UnitOfWork {
	return &unitOfWork{db: db}
}

// Begin starts a new transaction
func (uow *unitOfWork) Begin(ctx context.Context) (repositories.Transaction, error) {
	tx, err := uow.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	
	return &transaction{
		tx:          tx,
		paymentRepo: &paymentRepository{db: uow.db}, // Will be overridden with tx
		eventRepo:   &eventRepository{db: uow.db},   // Will be overridden with tx
	}, nil
}

// transaction implements Transaction interface
type transaction struct {
	tx          pgx.Tx
	paymentRepo *paymentRepository
	eventRepo   *eventRepository
}

// Commit commits the transaction
func (t *transaction) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

// Rollback rolls back the transaction
func (t *transaction) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}

// PaymentRepository returns a transactional payment repository
func (t *transaction) PaymentRepository() repositories.PaymentRepository {
	return &transactionalPaymentRepository{tx: t.tx}
}

// EventRepository returns a transactional event repository
func (t *transaction) EventRepository() repositories.EventRepository {
	return &transactionalEventRepository{tx: t.tx}
}

// Transactional repository implementations that use pgx.Tx instead of pgxpool.Pool

// transactionalPaymentRepository wraps payment operations in a transaction
type transactionalPaymentRepository struct {
	tx pgx.Tx
}

func (r *transactionalPaymentRepository) Save(ctx context.Context, p *payment.Payment) error {
	if p.ID == 0 {
		return r.insert(ctx, p)
	}
	return r.update(ctx, p)
}

func (r *transactionalPaymentRepository) FindByID(ctx context.Context, id int64) (*payment.Payment, error) {
	row := r.tx.QueryRow(ctx, `
		SELECT id, tenant_id, invoice_no, amount, currency, status, method, external_id, msisdn_hash, created_at, updated_at
		FROM payments 
		WHERE id = $1`, id)
	
	return scanPayment(row)
}

func (r *transactionalPaymentRepository) FindByExternalID(ctx context.Context, tenantID int64, externalID string) (*payment.Payment, error) {
	row := r.tx.QueryRow(ctx, `
		SELECT id, tenant_id, invoice_no, amount, currency, status, method, external_id, msisdn_hash, created_at, updated_at
		FROM payments 
		WHERE tenant_id = $1 AND external_id = $2`, tenantID, externalID)
	
	return scanPayment(row)
}

func (r *transactionalPaymentRepository) FindByTenantID(ctx context.Context, tenantID int64, limit, offset int) ([]*payment.Payment, error) {
	rows, err := r.tx.Query(ctx, `
		SELECT id, tenant_id, invoice_no, amount, currency, status, method, external_id, msisdn_hash, created_at, updated_at
		FROM payments 
		WHERE tenant_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var payments []*payment.Payment
	for rows.Next() {
		p, err := scanPaymentFromRows(rows)
		if err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	
	return payments, rows.Err()
}

func (r *transactionalPaymentRepository) UpdateStatus(ctx context.Context, id int64, status payment.Status) error {
	_, err := r.tx.Exec(ctx, `
		UPDATE payments 
		SET status = $1, updated_at = now() 
		WHERE id = $2`, string(status), id)
	return err
}

func (r *transactionalPaymentRepository) insert(ctx context.Context, p *payment.Payment) error {
	err := r.tx.QueryRow(ctx, `
		INSERT INTO payments (tenant_id, invoice_no, amount, currency, status, method, external_id, msisdn_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		p.TenantID, p.InvoiceNo, int64(p.Amount), string(p.Currency), string(p.Status), 
		string(p.Method), p.ExternalID, p.MSISDNHash, p.CreatedAt, p.UpdatedAt).Scan(&p.ID)
	
	return err
}

func (r *transactionalPaymentRepository) update(ctx context.Context, p *payment.Payment) error {
	_, err := r.tx.Exec(ctx, `
		UPDATE payments 
		SET invoice_no = $1, amount = $2, currency = $3, status = $4, method = $5, 
		    external_id = $6, msisdn_hash = $7, updated_at = $8
		WHERE id = $9`,
		p.InvoiceNo, int64(p.Amount), string(p.Currency), string(p.Status), 
		string(p.Method), p.ExternalID, p.MSISDNHash, p.UpdatedAt, p.ID)
	
	return err
}

// transactionalEventRepository wraps event operations in a transaction
type transactionalEventRepository struct {
	tx pgx.Tx
}

func (r *transactionalEventRepository) Save(ctx context.Context, e *event.Event) error {
	if e.ID == 0 {
		return r.insert(ctx, e)
	}
	return r.update(ctx, e)
}

func (r *transactionalEventRepository) FindByID(ctx context.Context, id int64) (*event.Event, error) {
	row := r.tx.QueryRow(ctx, `
		SELECT id, tenant_id, provider_credential_id, event_type, external_id, amount, 
		       msisdn, invoice_ref, transaction_id, status, response_description, 
		       payload_json, received_at, processed_at, processing_status
		FROM payment_events 
		WHERE id = $1`, id)
	
	return scanEvent(row)
}

func (r *transactionalEventRepository) FindUnprocessed(ctx context.Context, limit int) ([]*event.Event, error) {
	rows, err := r.tx.Query(ctx, `
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
	
	return scanEvents(rows)
}

func (r *transactionalEventRepository) FindByTenantID(ctx context.Context, tenantID int64, limit, offset int) ([]*event.Event, error) {
	rows, err := r.tx.Query(ctx, `
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
	
	return scanEvents(rows)
}

func (r *transactionalEventRepository) MarkProcessed(ctx context.Context, id int64, status event.ProcessingStatus) error {
	_, err := r.tx.Exec(ctx, `
		UPDATE payment_events 
		SET processing_status = $1, processed_at = now(), updated_at = now()
		WHERE id = $2`, string(status), id)
	return err
}

func (r *transactionalEventRepository) MarkForReprocessing(ctx context.Context, tenantID, eventID int64) error {
	_, err := r.tx.Exec(ctx, `
		UPDATE payment_events 
		SET processing_status = 'queued', processed_at = NULL, updated_at = now()
		WHERE id = $1 AND tenant_id = $2`, eventID, tenantID)
	return err
}

func (r *transactionalEventRepository) insert(ctx context.Context, e *event.Event) error {
	err := r.tx.QueryRow(ctx, `
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

func (r *transactionalEventRepository) update(ctx context.Context, e *event.Event) error {
	_, err := r.tx.Exec(ctx, `
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

// Shared scanning functions used by both regular and transactional repositories

// scanPayment scans a single row into payment domain object
func scanPayment(row pgx.Row) (*payment.Payment, error) {
	var p payment.Payment
	var invoiceNo sql.NullString
	var msisdnHash sql.NullString
	
	err := row.Scan(
		&p.ID, &p.TenantID, &invoiceNo, &p.Amount, &p.Currency,
		&p.Status, &p.Method, &p.ExternalID, &msisdnHash, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	
	if invoiceNo.Valid {
		p.InvoiceNo = invoiceNo.String
	}
	if msisdnHash.Valid {
		p.MSISDNHash = msisdnHash.String
	}
	
	return &p, nil
}

// scanPaymentFromRows scans rows into payment domain object
func scanPaymentFromRows(rows pgx.Rows) (*payment.Payment, error) {
	var p payment.Payment
	var invoiceNo sql.NullString
	var msisdnHash sql.NullString
	
	err := rows.Scan(
		&p.ID, &p.TenantID, &invoiceNo, &p.Amount, &p.Currency,
		&p.Status, &p.Method, &p.ExternalID, &msisdnHash, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	
	if invoiceNo.Valid {
		p.InvoiceNo = invoiceNo.String
	}
	if msisdnHash.Valid {
		p.MSISDNHash = msisdnHash.String
	}
	
	return &p, nil
}

// scanEvent scans a single row into event domain object
func scanEvent(row pgx.Row) (*event.Event, error) {
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
func scanEvents(rows pgx.Rows) ([]*event.Event, error) {
	var events []*event.Event
	for rows.Next() {
		e, err := scanEventFromRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	
	return events, rows.Err()
}

// scanEventFromRows scans rows into event domain object
func scanEventFromRows(rows pgx.Rows) (*event.Event, error) {
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