run:
	go run ./cmd/api
tidy:
	go mod tidy
migrate:
	@echo "Setting up database permissions..."
	sudo -u postgres psql -d paymatch -c "GRANT CREATE ON SCHEMA public TO paymatch;" || true
	@echo "Running migrations as paymatch user..."
	@export $$(cat .env | xargs) && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/001_init.sql && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/002_usage.sql && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/003_c2b_mode.sql && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/003_event_queue.sql && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/003_payments_idempotency.sql && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/003_payments_unique_external.sql && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/003_reconcile.sql && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/004_multi_provider_support.sql && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/005_event_fields.sql && \
	psql "$$DB_DSN" -f internal/store/postgres/migrations/006_processing_status.sql
	@echo "Migration completed!"