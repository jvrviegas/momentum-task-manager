# Momentum backend

A Go backend with PostgreSQL on Neon that will replace Taskwarrior as Momentum's source of truth. It is being built; until backend mode ships, the terminal and macOS apps keep using Taskwarrior.

See [ADR 0002](../docs/adr/0002-go-postgres-backend.md) for the decision and the [implementation plan](../docs/plans/backend.md) for the work.

## Run

[`compose.yaml`](../compose.yaml) at the repository root starts a disposable PostgreSQL server, applies the migrations, and starts the server:

```sh
docker compose up -d --build   # from the repository root
curl localhost:8080/health
docker compose down            # removes the containers and their data
```

PostgreSQL listens on `localhost:5432` (user and password `postgres`). The server uses the `momentum` database; `momentum_dev` stays empty for `migration-diff`. To run the server from source instead, stop the `server` service and run, here:

```sh
DATABASE_URL=postgres://postgres:postgres@localhost:5432/momentum?sslmode=disable go run ./cmd/momentum-server
```

`momentum-server migrate` applies pending migrations to `DATABASE_URL` and exits; Compose runs it before the server.

| Variable | Meaning |
|---|---|
| `DATABASE_URL` | PostgreSQL to use. Required. |
| `PORT` | Port to listen on; Cloud Run sets it. Defaults to `8080`. |
| `PUBLIC_URL` | The server's address as browsers reach it, for the sign-in page and provider callbacks. Defaults to `http://localhost:<PORT>`. |
| `ALLOWED_EMAILS` | Comma-separated addresses that may sign in. Empty, nobody can. |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | A Google OAuth client; without them, Google sign-in is not offered. |
| `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` | A GitHub OAuth app; without them, GitHub sign-in is not offered. |

The server stops on SIGINT or SIGTERM and gives requests in flight 8 seconds to finish. The health check is `/health`, not `/healthz`, because Cloud Run reserves some paths that end in `z`.

## Sign-in

Apps sign in with the device authorization flow (RFC 8628): they call `POST /v1/auth/device`, show the code, and open `PUBLIC_URL/device`, where the person signs in with Google (OpenID Connect) or GitHub (OAuth; GitHub offers no OpenID Connect to OAuth apps). The app polls `POST /v1/auth/token` until it gets a token, then sends it as `Authorization: Bearer <token>`. The server stores only token hashes; tokens last 90 days, and `POST /v1/auth/sign-out` revokes one. Only `ALLOWED_EMAILS` can sign in; a provider's account joins the user with the same verified address.

To sign in locally, register an OAuth client with each provider you want, with these callback URLs, and put its credentials and your address in an `.env` file next to `compose.yaml` (it is ignored by Git):

| Provider | Where | Callback URL |
|---|---|---|
| Google | Google Cloud console, APIs & Services, Credentials, OAuth client ID, "Web application" | `http://localhost:8080/auth/callback/google` |
| GitHub | GitHub, Settings, Developer settings, OAuth Apps | `http://localhost:8080/auth/callback/github` |

```sh
ALLOWED_EMAILS=you@example.com
GITHUB_CLIENT_ID=…
GITHUB_CLIENT_SECRET=…
```

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
docker compose up -d --wait postgres   # from the repository root
MOMENTUM_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable go test ./...
```

## Container image

Build from the repository root, because the module uses `../core`:

```sh
docker build -f backend/Dockerfile .
```
