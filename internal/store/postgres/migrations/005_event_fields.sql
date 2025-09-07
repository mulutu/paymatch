-- 005_event_fields.sql
-- Add missing fields to payment_events table for pure architecture event processing

-- Add missing columns to payment_events table
ALTER TABLE payment_events 
ADD COLUMN IF NOT EXISTS amount BIGINT,
ADD COLUMN IF NOT EXISTS msisdn TEXT,
ADD COLUMN IF NOT EXISTS invoice_ref TEXT,
ADD COLUMN IF NOT EXISTS transaction_id TEXT,
ADD COLUMN IF NOT EXISTS response_description TEXT;

-- Add indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_payment_events_transaction_id ON payment_events(transaction_id) WHERE transaction_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_payment_events_msisdn ON payment_events(msisdn) WHERE msisdn IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_payment_events_invoice_ref ON payment_events(invoice_ref) WHERE invoice_ref IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN payment_events.amount IS 'Transaction amount in minor units (e.g., cents)';
COMMENT ON COLUMN payment_events.msisdn IS 'Phone number associated with the transaction';
COMMENT ON COLUMN payment_events.invoice_ref IS 'Invoice or account reference for the transaction';
COMMENT ON COLUMN payment_events.transaction_id IS 'Provider transaction ID or receipt number';
COMMENT ON COLUMN payment_events.response_description IS 'Description or message from the provider response';