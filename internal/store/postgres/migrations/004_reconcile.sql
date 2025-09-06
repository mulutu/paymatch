-- 003_reconcile.sql

ALTER TABLE payment_events
  ADD COLUMN invoice_ref TEXT,
  ADD COLUMN msisdn TEXT,
  ADD COLUMN amount INT;

CREATE INDEX IF NOT EXISTS idx_payment_events_invoice
  ON payment_events(tenant_id, invoice_ref);
