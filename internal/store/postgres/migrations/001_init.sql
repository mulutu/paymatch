-- Tenants & keys
CREATE TABLE IF NOT EXISTS tenants (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tenant_api_keys (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  key_hash TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_used_at TIMESTAMPTZ
);

-- Provider credentials
CREATE TABLE IF NOT EXISTS provider_credentials (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  provider TEXT NOT NULL,                        -- 'mpesa_daraja'
  shortcode TEXT NOT NULL,
  passkey_enc TEXT NOT NULL,
  consumer_key_enc TEXT NOT NULL,
  consumer_secret_enc TEXT NOT NULL,
  environment TEXT NOT NULL,                    -- 'sandbox'|'production'
  webhook_token TEXT UNIQUE NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_provider_credentials_shortcode ON provider_credentials(shortcode);
CREATE INDEX IF NOT EXISTS idx_provider_credentials_tenant ON provider_credentials(tenant_id);

-- Payments & events
CREATE TABLE IF NOT EXISTS payment_events (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  provider_credential_id BIGINT NOT NULL REFERENCES provider_credentials(id),
  event_type TEXT NOT NULL,
  external_id TEXT NOT NULL,
  payload_json JSONB NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  processed_at TIMESTAMPTZ,
  status TEXT,
  UNIQUE (tenant_id, event_type, external_id)
);

CREATE TABLE IF NOT EXISTS payments (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  invoice_no TEXT,
  msisdn_hash TEXT,
  amount INT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'KES',
  status TEXT NOT NULL DEFAULT 'pending',
  method TEXT NOT NULL DEFAULT 'mpesa',
  provider_credential_id BIGINT REFERENCES provider_credentials(id),
  external_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_payments_tenant_invoice ON payments(tenant_id, invoice_no);
CREATE INDEX IF NOT EXISTS idx_payments_tenant_created ON payments(tenant_id, created_at);