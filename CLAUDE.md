# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Backend (Go API)
```bash
# Start infrastructure (PostgreSQL, Redis)
docker compose up -d

# Set environment variables from .env
cp .env.example .env
export $(cat .env | xargs)

# Run database migrations
make migrate

# Build and run the API server
make run
# or directly: go run ./cmd/api

# Tidy dependencies
make tidy
# or: go mod tidy

# Build binary
go build -o api ./cmd/api
```

### Frontend Applications
```bash
# Start marketing site (www.paymatch.co)
cd web/marketing && npm run dev
# Runs on http://localhost:3000

# Start dashboard/console (app.paymatch.co)  
cd web/dashboard && npm run dev --port 3001
# Runs on http://localhost:3001

# Build for production
cd web/marketing && npm run build
cd web/dashboard && npm run build

# Install dependencies
cd web/marketing && npm install
cd web/dashboard && npm install
```

### Database Operations
```bash
# Run all migrations in sequence
make migrate

# Connect to database directly
psql "$DB_DSN"

# Manual tenant/API key setup (for testing)
# See README.md for SQL commands to create test tenant and credentials
```

### Testing API Endpoints
```bash
# Test STK Push (requires valid API key)
curl -X POST http://localhost:8080/v1/payments/stk \
  -H "Authorization: Bearer <YOUR_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"amount":1,"phone":"2547XXXXXXXX","accountRef":"INV-1001","description":"Test"}'

# Test health endpoint
curl http://localhost:8080/healthz
```

## Architecture Overview

PayMatch is a multi-tenant SaaS payment rails aggregator that enables businesses to accept mobile money payments through multiple providers (currently M-Pesa, with support for Airtel Money, T-Kash, and others planned).

### Core Architecture Patterns

**Multi-Provider Modular Architecture**: The system uses a provider registry pattern where each payment provider (M-Pesa, Airtel, etc.) implements a common `Provider` interface. This allows adding new providers without changing core business logic.

**Event-Driven Processing**: All payment events (webhooks) are persisted to a `payment_events` table and processed asynchronously by a reconciliation worker. This ensures reliable payment processing even during high load or provider outages.

**Multi-Tenancy**: Each tenant has isolated credentials, API keys, and payment data. Tenants can have multiple provider configurations (different shortcodes, environments).

**Encryption at Rest**: All sensitive provider credentials (M-Pesa consumer keys, passkeys) are encrypted using AES-256 before storage.

### Key Components

1. **Provider Registry** (`internal/provider/registry.go`): Central hub that routes payment operations to appropriate providers based on tenant credentials.

2. **Modular Provider Services** (`internal/provider/mpesa/`): Each provider has dedicated services for different operations:
   - `stk.go` - STK Push payments
   - `c2b.go` - Customer-to-Business payments  
   - `b2c.go` - Business-to-Customer transfers
   - `auth.go` - Provider authentication with token caching
   - `webhook.go` - Webhook payload parsing

3. **Event Processing** (`internal/core/reconcile/worker.go`): Background worker that processes payment events, updating payment status and triggering business logic.

4. **HTTP Layer** (`internal/http/`): Clean separation between HTTP handling and business logic. Handlers are thin wrappers that call provider registry methods.

### Database Schema Patterns

**Multi-Provider Credentials**: The `provider_credentials` table uses both `provider` (legacy) and `provider_type` fields to support multiple provider types per tenant.

**Event Sourcing**: All payment events are stored in `payment_events` with idempotency based on `(tenant_id, event_type, external_id)`.

**Audit Trail**: Payment state changes are tracked through the events table, providing full payment lifecycle visibility.

### Critical Integration Points

**Webhook Processing**: The system provides both generic webhook endpoints (`/hooks/{provider}/{shortcode}`) and provider-specific endpoints. All webhooks are validated, parsed, and converted to standardized `Event` types.

**Provider Authentication**: M-Pesa uses OAuth tokens with automatic refresh and caching. Other providers will implement similar patterns in their respective service modules.

**Reconciliation**: The worker processes events sequentially, handling payment confirmations, failures, and timeout scenarios.

## Code Organization Principles

### Modular Services Architecture
- Business logic lives in `internal/provider/{provider_name}/` 
- Each provider implements the common `Provider` interface
- HTTP handlers are thin and delegate to provider services via the registry
- NO business logic in HTTP handlers - they only handle request/response

### Key Rules for Development
- Always use the provider registry pattern - never directly instantiate provider services in handlers
- All payment provider implementations must be in `internal/provider/{provider_name}/` directories
- Use the existing `base/` utilities for common functionality (HTTP clients, validation)
- Follow the established service pattern: each operation (STK, C2B, B2C) gets its own service file
- Webhook parsing must convert provider-specific payloads to standardized `core.Event` types

### Database Migrations
Migrations are numbered sequentially in `internal/store/postgres/migrations/`. Always add new migrations with the next number in sequence. Migration files beginning with `003_` may run in parallel - they were created to handle different schema updates.

### Environment Configuration
The app loads configuration through the `config` package which reads from environment variables. Provider credentials are encrypted before storage and decrypted at runtime using the AES key from `AES_256_KEY_BASE64`.

## Development Best Practices

- Always check the file structure and detect changes before suggesting modifications
- Explain recommendations clearly and wait for user confirmation before making changes
- Ensure suggestions have >95% confidence rating
- Use the modular provider architecture - avoid duplicating business logic in handlers
- Follow existing patterns for new provider implementations
- All sensitive credentials must be encrypted using the `crypto` package before database storage