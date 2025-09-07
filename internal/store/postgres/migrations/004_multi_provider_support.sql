-- Multi-provider support migration
-- This migration adds support for flexible provider credentials and multiple provider types

-- Add new columns to provider_credentials for better flexibility
ALTER TABLE provider_credentials 
ADD COLUMN provider_type TEXT,
ADD COLUMN credentials_json JSONB DEFAULT '{}',
ADD COLUMN config_json JSONB DEFAULT '{}',
ADD COLUMN capabilities TEXT[] DEFAULT '{}';

-- Update existing M-Pesa records with provider_type
UPDATE provider_credentials 
SET provider_type = 'mpesa_daraja' 
WHERE provider = 'mpesa_daraja' OR provider = 'mpesa';

-- Add index for provider_type for better query performance
CREATE INDEX IF NOT EXISTS idx_provider_credentials_provider_type ON provider_credentials(provider_type);
CREATE INDEX IF NOT EXISTS idx_provider_credentials_tenant_provider ON provider_credentials(tenant_id, provider_type);

-- Add check constraint to ensure provider_type is set for new records
ALTER TABLE provider_credentials 
ADD CONSTRAINT chk_provider_type_not_null 
CHECK (provider_type IS NOT NULL AND provider_type != '');

-- Create a table to store provider configurations (optional, for future use)
CREATE TABLE IF NOT EXISTS provider_configs (
  id BIGSERIAL PRIMARY KEY,
  provider_type TEXT NOT NULL,
  environment TEXT NOT NULL DEFAULT 'sandbox',
  config_json JSONB NOT NULL DEFAULT '{}',
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(provider_type, environment)
);

-- Insert default configurations for supported providers
INSERT INTO provider_configs (provider_type, environment, config_json) VALUES
('mpesa_daraja', 'sandbox', '{
  "base_url": "https://sandbox.safaricom.co.ke",
  "timeout_seconds": 30,
  "supported_operations": ["stk_push", "c2b", "b2c", "balance", "status"],
  "min_amount": 1,
  "max_amount": 70000,
  "currency": "KES"
}'),
('mpesa_daraja', 'production', '{
  "base_url": "https://api.safaricom.co.ke",
  "timeout_seconds": 30,
  "supported_operations": ["stk_push", "c2b", "b2c", "balance", "status"],
  "min_amount": 1,
  "max_amount": 70000,
  "currency": "KES"
}'),
('airtel_money', 'sandbox', '{
  "base_url": "https://openapiuat.airtel.africa",
  "timeout_seconds": 30,
  "supported_operations": ["stk_push", "b2c", "balance"],
  "min_amount": 1,
  "max_amount": 50000,
  "currency": "KES"
}'),
('airtel_money', 'production', '{
  "base_url": "https://openapi.airtel.africa",
  "timeout_seconds": 30,
  "supported_operations": ["stk_push", "b2c", "balance"],
  "min_amount": 1,
  "max_amount": 50000,
  "currency": "KES"
}')
ON CONFLICT (provider_type, environment) DO UPDATE SET
  config_json = EXCLUDED.config_json,
  updated_at = now();

-- Create provider operation logs table for tracking operations
CREATE TABLE IF NOT EXISTS provider_operation_logs (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  provider_credential_id BIGINT NOT NULL REFERENCES provider_credentials(id),
  operation_type TEXT NOT NULL,
  request_id TEXT NOT NULL,
  external_id TEXT,
  status TEXT NOT NULL DEFAULT 'pending',
  request_data JSONB,
  response_data JSONB,
  error_message TEXT,
  duration_ms INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

-- Add indexes for operation logs
CREATE INDEX IF NOT EXISTS idx_provider_operation_logs_tenant ON provider_operation_logs(tenant_id, created_at);
CREATE INDEX IF NOT EXISTS idx_provider_operation_logs_credential ON provider_operation_logs(provider_credential_id);
CREATE INDEX IF NOT EXISTS idx_provider_operation_logs_external_id ON provider_operation_logs(external_id);
CREATE INDEX IF NOT EXISTS idx_provider_operation_logs_request_id ON provider_operation_logs(request_id);

-- Update payment_events to support multiple event types
ALTER TABLE payment_events 
ADD COLUMN provider_type TEXT,
ADD COLUMN operation_type TEXT,
ADD COLUMN correlation_id TEXT;

-- Update existing payment_events records
UPDATE payment_events 
SET provider_type = 'mpesa_daraja'
WHERE provider_type IS NULL;

-- Add indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_payment_events_provider_type ON payment_events(provider_type);
CREATE INDEX IF NOT EXISTS idx_payment_events_operation_type ON payment_events(operation_type);
CREATE INDEX IF NOT EXISTS idx_payment_events_correlation_id ON payment_events(correlation_id);

-- Add provider metadata to payments table
ALTER TABLE payments 
ADD COLUMN provider_type TEXT,
ADD COLUMN provider_transaction_id TEXT,
ADD COLUMN provider_data JSONB DEFAULT '{}';

-- Update existing payments records
UPDATE payments 
SET provider_type = 'mpesa_daraja'
WHERE provider_type IS NULL;

-- Add index for provider_transaction_id
CREATE INDEX IF NOT EXISTS idx_payments_provider_transaction_id ON payments(provider_transaction_id);
CREATE INDEX IF NOT EXISTS idx_payments_provider_type ON payments(provider_type);

-- Comments for documentation
COMMENT ON TABLE provider_configs IS 'Global configurations for different payment providers and environments';
COMMENT ON TABLE provider_operation_logs IS 'Logs of all operations performed through payment providers for auditing and debugging';
COMMENT ON COLUMN provider_credentials.provider_type IS 'Standardized provider type identifier (e.g., mpesa_daraja, airtel_money)';
COMMENT ON COLUMN provider_credentials.credentials_json IS 'JSON storage for flexible provider-specific credentials';
COMMENT ON COLUMN provider_credentials.config_json IS 'JSON storage for provider-specific configuration overrides';
COMMENT ON COLUMN provider_credentials.capabilities IS 'Array of operations this credential configuration supports';