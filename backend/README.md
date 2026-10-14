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

## Store and migrations

`internal/store` keeps users and tasks in PostgreSQL through [ent](https://entgo.io), with pgx as the driver. The schema is in `internal/store/ent/schema`; after changing it, regenerate the client and write a migration:

```sh
go generate ./...
DEV_DATABASE_URL=postgres://postgres:postgres@localhost:5432/momentum_dev?sslmode=disable go run ./cmd/migration-diff <name>
```

`migration-diff` replays `migrations/` on that empty local database and writes the difference as a new file. Review it before committing, and never edit a committed migration; add a new one instead. `store.Migrate` applies pending files, each in its own transaction, and records them in `momentum_migrations.revisions`.

Task reads and writes go through privacy rules: they need a signed-in user in the context (`viewer.NewContext`), and see and change only that user's rows.

## Tests

The store tests need a local PostgreSQL server. Each test creates its own database there and drops it afterwards. The tests refuse any host that is not this machine, and skip when the variable is unset:

```sh
docker run -d --name momentum-pg -e POSTGRES_PASSWORD=postgres -p 127.0.0.1:5432:5432 postgres:17-alpine
MOMENTUM_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable go test ./...
```

## Container image

Build from the repository root, because the module uses `../core`:

```sh
docker build -f backend/Dockerfile .
```
