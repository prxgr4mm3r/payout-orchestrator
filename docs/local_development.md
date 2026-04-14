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

## Check health

```bash
curl http://localhost:8080/healthz
```

Expected response:

```text
ok
```
