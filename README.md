# PayMatch (SaaS) — Quick start

1. **Start infra**
   ```bash
   docker compose up -d
   ```
2. **Set env**
   ```bash
   cp .env.example .env
   export $(cat .env | xargs)
   ```
3. **Migrate DB**
   ```bash
   make migrate
   ```
4. **Seed a tenant & API key (psql)**
   ```sql
   INSERT INTO tenants(name) VALUES ('DemoCo') RETURNING id; -- note the id
   -- generate an API key string and hash with repo helper (or any sha256 tool)
   INSERT INTO tenant_api_keys(tenant_id, key_hash, name) VALUES (<TENANT_ID>, '<SHA256_HEX>', 'default');
   -- encrypt Daraja creds using AES_256_KEY_BASE64 (use a tiny Go tool or psql function)
   INSERT INTO provider_credentials(tenant_id, provider, shortcode, passkey_enc, consumer_key_enc, consumer_secret_enc, environment, webhook_token)
   VALUES (<TENANT_ID>, 'mpesa_daraja', '174379', '<ENC_PASSKEY>', '<ENC_CONSUMER_KEY>', '<ENC_CONSUMER_SECRET>', 'sandbox', '<RANDOM_TOKEN>');
   ```
5. **Run API**
   ```bash
   make run
   ```
6. **Test STK**
   ```bash
   curl -X POST http://localhost:8080/v1/payments/stk \
     -H "Authorization: Bearer <YOUR_API_KEY>" \
     -H "Content-Type: application/json" \
     -d '{"amount":1,"phone":"2547XXXXXXXX","accountRef":"INV-1001","description":"Test"}'
   ```
7. **Expose webhooks** (ngrok etc.) and set callback URLs in Daraja.