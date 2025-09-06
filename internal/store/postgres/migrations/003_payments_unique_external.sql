-- Ensure idempotency on C2B/any provider with external IDs (e.g., TransID)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint 
     WHERE conname = 'uq_payments_tenant_external'
  ) THEN
    ALTER TABLE payments
      ADD CONSTRAINT uq_payments_tenant_external
      UNIQUE (tenant_id, external_id);
  END IF;
END$$;
