package postgres

import (
	"context"
	"database/sql"
	
	"paymatch/internal/domain/payment"
	
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// paymentRepository implements PaymentRepository interface with pure data access
type paymentRepository struct {
	db *pgxpool.Pool
}

// NewPaymentRepository creates a new payment repository
func NewPaymentRepository(db *pgxpool.Pool) *paymentRepository {
	return &paymentRepository{db: db}
}

// Save saves a payment (insert or update)
func (r *paymentRepository) Save(ctx context.Context, p *payment.Payment) error {
	if p.ID == 0 {
		return r.insert(ctx, p)
	}
	return r.update(ctx, p)
}

// FindByID finds a payment by ID
func (r *paymentRepository) FindByID(ctx context.Context, id int64) (*payment.Payment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, invoice_no, amount, currency, status, method, external_id, msisdn_hash, created_at, updated_at
		FROM payments 
		WHERE id = $1`, id)
	
	return r.scanPayment(row)
}

// FindByExternalID finds a payment by external ID and tenant
func (r *paymentRepository) FindByExternalID(ctx context.Context, tenantID int64, externalID string) (*payment.Payment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, tenant_id, invoice_no, amount, currency, status, method, external_id, msisdn_hash, created_at, updated_at
		FROM payments 
		WHERE tenant_id = $1 AND external_id = $2`, tenantID, externalID)
	
	return r.scanPayment(row)
}

// FindByTenantID finds payments by tenant with pagination
func (r *paymentRepository) FindByTenantID(ctx context.Context, tenantID int64, limit, offset int) ([]*payment.Payment, error) {
	rows, err := r.db.Query(ctx, `
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
		p, err := r.scanPaymentFromRows(rows)
		if err != nil {
			return nil, err
		}
		payments = append(payments, p)
	}
	
	return payments, rows.Err()
}

// UpdateStatus updates only the payment status
func (r *paymentRepository) UpdateStatus(ctx context.Context, id int64, status payment.Status) error {
	_, err := r.db.Exec(ctx, `
		UPDATE payments 
		SET status = $1, updated_at = now() 
		WHERE id = $2`, string(status), id)
	return err
}

// insert creates a new payment record
func (r *paymentRepository) insert(ctx context.Context, p *payment.Payment) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO payments (tenant_id, invoice_no, amount, currency, status, method, external_id, msisdn_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		p.TenantID, p.InvoiceNo, int64(p.Amount), string(p.Currency), string(p.Status), 
		string(p.Method), p.ExternalID, p.MSISDNHash, p.CreatedAt, p.UpdatedAt).Scan(&p.ID)
	
	return err
}

// update modifies an existing payment record
func (r *paymentRepository) update(ctx context.Context, p *payment.Payment) error {
	_, err := r.db.Exec(ctx, `
		UPDATE payments 
		SET invoice_no = $1, amount = $2, currency = $3, status = $4, method = $5, 
		    external_id = $6, msisdn_hash = $7, updated_at = $8
		WHERE id = $9`,
		p.InvoiceNo, int64(p.Amount), string(p.Currency), string(p.Status), 
		string(p.Method), p.ExternalID, p.MSISDNHash, p.UpdatedAt, p.ID)
	
	return err
}

// scanPayment scans a single row into payment domain object
func (r *paymentRepository) scanPayment(row pgx.Row) (*payment.Payment, error) {
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
func (r *paymentRepository) scanPaymentFromRows(rows pgx.Rows) (*payment.Payment, error) {
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