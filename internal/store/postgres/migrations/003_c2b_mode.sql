-- Per-shortcode C2B behaviour knobs
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='provider_credentials' AND column_name='c2b_mode') THEN
    ALTER TABLE provider_credentials ADD COLUMN c2b_mode TEXT NOT NULL DEFAULT 'paybill';
    ALTER TABLE provider_credentials ADD CONSTRAINT chk_provider_credentials_c2b_mode CHECK (c2b_mode IN ('paybill','buygoods'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='provider_credentials' AND column_name='bill_ref_required') THEN
    ALTER TABLE provider_credentials ADD COLUMN bill_ref_required BOOLEAN NOT NULL DEFAULT TRUE;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='provider_credentials' AND column_name='bill_ref_regex') THEN
    ALTER TABLE provider_credentials ADD COLUMN bill_ref_regex TEXT;
  END IF;
END$$;
