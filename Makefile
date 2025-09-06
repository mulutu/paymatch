run:
	go run ./cmd/api
tidy:
	go mod tidy
migrate:
	psql "$$DB_DSN" -f internal/store/postgres/migrations/001_init.sql || true
	psql "$$DB_DSN" -f internal/store/postgres/migrations/002_usage.sql || true