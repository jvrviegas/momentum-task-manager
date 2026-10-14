# Momentum backend

A Go backend with PostgreSQL on Neon that will replace Taskwarrior as Momentum's source of truth. It is being built; until backend mode ships, the terminal and macOS apps keep using Taskwarrior.

See [ADR 0002](../docs/adr/0002-go-postgres-backend.md) for the decision and the [implementation plan](../docs/plans/backend.md) for the work.

## Run

```sh
go run ./cmd/momentum-server
curl localhost:8080/health
```

| Variable | Meaning |
|---|---|
| `PORT` | Port to listen on; Cloud Run sets it. Defaults to `8080`. |

The server stops on SIGINT or SIGTERM and gives requests in flight 8 seconds to finish. The health check is `/health`, not `/healthz`, because Cloud Run reserves some paths that end in `z`.

## Logs

Logs are JSON lines in the shape Cloud Logging reads. Task contents, tokens, and email addresses must never reach them (ADR 0002, P3), so only the attribute keys listed in `internal/logging` keep their values; everything else is logged as `[redacted]`. Requests are logged by route pattern, never by path, query, headers, or body. Add a key to the list only for values that can never hold user data, and keep log messages constant.

## Container image

Build from the repository root, because the module uses `../core`:

```sh
docker build -f backend/Dockerfile .
```
