package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type PaymentRow struct {
	ID         int64     `json:"id"`
	InvoiceNo  *string   `json:"invoiceNo"`
	Amount     int       `json:"amount"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"`
	Method     string    `json:"method"`
	ExternalID *string   `json:"externalId"`
	CreatedAt  time.Time `json:"createdAt"`
}

// HashMSISDN returns a stable SHA256 hex of the MSISDN (lowercased, trimmed).
func HashMSISDN(msisdn string) string {
	s := []byte(strings.ToLower(strings.TrimSpace(msisdn)))
	h := sha256.Sum256(s)
	return hex.EncodeToString(h[:])
}

// UpsertPaymentByExternalID inserts/updates a payment idempotently per tenant+TransID.
func (r *Repo) UpsertPaymentByExternalID(
	ctx context.Context,
	tenantID, credID int64,
	externalID string,
	invoiceNo string,
	amount int,
	msisdn string,
) error {
	if externalID == "" {
		return fmt.Errorf("externalID required")
	}
	msisdnHash := HashMSISDN(msisdn)

	_, err := r.db.Exec(ctx, `
		INSERT INTO payments (
			tenant_id, invoice_no, msisdn_hash, amount, currency, status, method,
			provider_credential_id, external_id
		)
		VALUES ($1, $2, $3, $4, 'KES', 'pending', 'mpesa', $5, $6)
		ON CONFLICT (tenant_id, external_id) DO UPDATE
		  SET amount     = EXCLUDED.amount,
		      invoice_no = CASE WHEN EXCLUDED.invoice_no IS NULL OR EXCLUDED.invoice_no = ''
		                        THEN payments.invoice_no ELSE EXCLUDED.invoice_no END,
		      updated_at = now()
	`, tenantID, invoiceNo, msisdnHash, amount, credID, externalID)
	return err
}

// UpsertPendingPayment is used during STK init to record a row keyed by tenant+external_id.
func (r *Repo) UpsertPendingPayment(
	ctx context.Context,
	tenantID int64,
	credID int64,
	invoice string,
	amount int,
	externalID string,
) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO payments (
			tenant_id,
			invoice_no,
			amount,
			status,
			method,
			provider_credential_id,
			external_id
		)
		VALUES ($1,$2,$3,'pending','mpesa',$4,$5)
		ON CONFLICT (tenant_id, external_id) DO UPDATE
		  SET amount     = EXCLUDED.amount,
		      invoice_no = EXCLUDED.invoice_no,
		      updated_at = now()
	`, tenantID, invoice, amount, credID, externalID)
	return err
}

// UpsertPayment updates first by (tenant_id, external_id); if none, inserts.
func (r *Repo) UpsertPayment(
	ctx context.Context,
	tenantID, credID int64,
	invoice, msisdn string,
	amount int,
	currency, method, externalID, status string,
) error {
	var msHash string
	if msisdn != "" {
		msHash = HashMSISDN(msisdn)
	}

	// Try update by tenant+external_id; if none, insert
	tag, err := r.db.Exec(ctx, `UPDATE payments
		SET invoice_no=COALESCE(NULLIF($3,''), invoice_no),
			msisdn_hash=COALESCE(NULLIF($4,''), msisdn_hash),
			amount=CASE WHEN $5>0 THEN $5 ELSE amount END,
			currency=COALESCE(NULLIF($6,''), currency),
			status=COALESCE(NULLIF($7,''), status),
			method=COALESCE(NULLIF($8,''), method),
			provider_credential_id=COALESCE($9, provider_credential_id),
			updated_at=now()
		WHERE tenant_id=$1 AND external_id=$2`,
		tenantID, externalID, invoice, msHash, amount, currency, status, method, credID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	_, err = r.db.Exec(ctx, `INSERT INTO payments(tenant_id, invoice_no, msisdn_hash, amount, currency, status, method, provider_credential_id, external_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		tenantID, invoice, msHash, amount, currency, status, method, credID, externalID)
	return err
}

func (r *Repo) ListPayments(ctx context.Context, tenantID int64, limit, offset int) ([]PaymentRow, error) {
	rows, err := r.db.Query(ctx, `SELECT id, invoice_no, amount, currency, status, method, external_id, created_at
		FROM payments WHERE tenant_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PaymentRow
	for rows.Next() {
		var p PaymentRow
		if err := rows.Scan(&p.ID, &p.InvoiceNo, &p.Amount, &p.Currency, &p.Status, &p.Method, &p.ExternalID, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
