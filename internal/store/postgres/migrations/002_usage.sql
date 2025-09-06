CREATE TABLE IF NOT EXISTS usage_counters (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  period_ym TEXT NOT NULL,                       -- e.g. '2025-09'
  api_calls INT NOT NULL DEFAULT 0,
  events_ingested INT NOT NULL DEFAULT 0,
  reconciled_count INT NOT NULL DEFAULT 0,
  UNIQUE (tenant_id, period_ym)
);