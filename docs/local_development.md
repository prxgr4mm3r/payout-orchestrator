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

## Check health

```bash
curl http://localhost:8080/healthz
```

Expected response:

```text
ok
```
