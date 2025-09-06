-- Optional: strengthen idempotency for payments created from events
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = 'uniq_payments_tenant_external' AND n.nspname = 'public'
    ) THEN
        CREATE UNIQUE INDEX uniq_payments_tenant_external
        ON payments(tenant_id, external_id)
        WHERE external_id IS NOT NULL;
    END IF;
END $$;
