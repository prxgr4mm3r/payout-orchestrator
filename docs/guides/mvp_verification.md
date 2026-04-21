# MVP Verification

## Automated Smoke Test

The smoke test runs the full MVP path against a real PostgreSQL instance:

- create an authenticated client
- create a funding source over HTTP
- create a payout over HTTP
- let the background processor claim and execute the outbox event
- assert the payout reaches `succeeded`
- assert the outbox event ends in `processed`

Run it with a reachable PostgreSQL database:

```bash
DB_URL=postgres://postgres:postgres@localhost:5432/payout?sslmode=disable go test ./cmd/api -run TestMVPPayoutSmoke -count=1
```

The test creates its own temporary PostgreSQL schema and applies the repo migrations there, so it does not modify the default `public` schema.

If you prefer a separate variable for smoke runs:

```bash
PAYOUT_SMOKE_DB_URL=postgres://postgres:postgres@localhost:5432/payout?sslmode=disable go test ./cmd/api -run TestMVPPayoutSmoke -count=1
```

## Manual Verification

Start dependencies:

```bash
make up
```

Run the API with the processor enabled:

```bash
DB_URL=postgres://postgres:postgres@localhost:5432/payout?sslmode=disable PROCESSOR_ENABLED=true go run ./cmd/api
```

Seed a client directly in PostgreSQL and capture its API key:

```sql
INSERT INTO clients (name)
VALUES ('manual-smoke-client')
RETURNING id, api_key;
```

Create a funding source:

```bash
curl -sS \
  -H 'X-API-Key: <api-key>' \
  -H 'Content-Type: application/json' \
  -d '{"name":"Main account","type":"bank_account","payment_account_id":"acct_manual_123"}' \
  http://localhost:8080/funding-sources
```

Create a payout:

```bash
curl -sS \
  -H 'X-API-Key: <api-key>' \
  -H 'Idempotency-Key: manual-payout-1' \
  -H 'Content-Type: application/json' \
  -d '{"funding_source_id":"<funding-source-id>","external_id":"manual-ext-1","recipient_name":"Manual recipient","recipient_account_id":"acct_recipient_manual_123","amount":"125.50","currency":"USDC"}' \
  http://localhost:8080/payouts
```

Poll the payout until it reaches a final status:

```bash
curl -sS \
  -H 'X-API-Key: <api-key>' \
  http://localhost:8080/payouts/<payout-id>
```

Expected final state:

- payout status is `succeeded`
- corresponding `outbox_events` row is `processed`
