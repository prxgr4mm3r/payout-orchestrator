# Local Development

## Start dependencies

```bash
make up
```

## Run migrations

```bash
make migrate-up
```

## Start the API

```bash
make run-api
```

The API now requires a reachable PostgreSQL instance on startup. If `DB_URL` is missing or points to an unavailable database, the process exits immediately.

When `PROCESSOR_ENABLED=true`, the API also requires `RABBITMQ_URL` and publishes payout jobs to `PAYOUT_QUEUE_NAME` (default: `payout.jobs`).

## Start the payout worker

```bash
make run-payout-worker
```

The payout worker requires:

- `DB_URL`
- `RABBITMQ_URL`
- optional `PAYOUT_QUEUE_NAME` (default: `payout.jobs`)

## Check health

```bash
curl http://localhost:8080/healthz
```

Expected response:

```text
ok
```

## MVP verification

For the automated smoke test and the manual payout verification flow, see [mvp_verification.md](/home/prxgr4mmer/payout-orchestrator/docs/guides/mvp_verification.md).
