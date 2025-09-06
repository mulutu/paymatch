-- 003_event_queue.sql
-- Outbox queue for processing/reprocessing provider events

CREATE TABLE IF NOT EXISTS event_queue (
  id BIGSERIAL PRIMARY KEY,
  tenant_id BIGINT NOT NULL REFERENCES tenants(id),
  event_id BIGINT NOT NULL REFERENCES payment_events(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'pending',            -- pending|delivering|done|failed
  attempts INT NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (event_id)
);

CREATE INDEX IF NOT EXISTS idx_event_queue_due
  ON event_queue(next_attempt_at)
  WHERE status IN ('pending','failed');
