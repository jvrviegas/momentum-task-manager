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
| `TRUSTED_PROXY_HOPS` | How many proxies in front of the server append the client's address to `X-Forwarded-For`, from 0 to 10. Defaults to `0`, which uses the connection's address and ignores the header, since clients can forge it. |

The server stops on SIGINT or SIGTERM and gives requests in flight 8 seconds to finish. The health check is `/health`, not `/healthz`, because Cloud Run reserves some paths that end in `z`.

## Sign-in

Apps sign in with the device authorization flow (RFC 8628): they call `POST /v1/auth/device`, show the code, and open `PUBLIC_URL/device`, where the person signs in with Google (OpenID Connect) or GitHub (OAuth; GitHub offers no OpenID Connect to OAuth apps). The app polls `POST /v1/auth/token` until it gets a token, then sends it as `Authorization: Bearer <token>`. The server stores only token hashes; tokens last 90 days, and `POST /v1/auth/sign-out` revokes one. Only `ALLOWED_EMAILS` can sign in; a provider's account joins the user with the same verified address.

The routes that work without a token are limited per client, in memory on each instance, in 10-minute windows: 30 requests to start a sign-in, submit the code at `/device`, or come back from a provider, together, and 300 polls. Over the limit, starting answers 429 with `rate_limited` and the form and callback a 429 page, each with `Retry-After`; polling answers `slow_down`, as RFC 8628 does, so the apps wait longer. The client is the entry `TRUSTED_PROXY_HOPS` places from the end of `X-Forwarded-For`, so set it to the number of proxies that append to that header.

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

## Tasks

The task routes in [`core/api`](../core/api/routes.go) need a token, as do `GET /v1/version`, which answers with the server's build version, and `PUT /v1/time-zone`. `GET /v1/tasks?status=pending|recurring|completed` lists tasks with their urgency, as Taskwarrior computes it; pending leaves out waiting tasks, and completed needs `ended_after`. Create, modify, complete, delete, start, and stop change only pending tasks and answer with the task as it is afterwards. Each needs an `Idempotency-Key` header: the server applies a key once within 24 hours, answers a retry with the first response, and refuses the key for a different request. Each change is written to `task_events` in the same transaction. `POST /v1/undo`, also with a key, reverts the latest change made with the same token unless its tasks have changed since. It undoes one step only, and undoing a create removes the task.

A task created with a `recurrence` and a `due` date starts a series: a template, listed with `status=recurring`, and its first pending instance, which is the answer. Completing or deleting the instance creates the next one, due at the series' next occurrence that is on or after today; missed ones are skipped. Occurrences keep their wall-clock time in the user's time zone across DST changes, and months count calendar months. The zone is UTC until an app sets it: the apps send their device's IANA zone with `PUT /v1/time-zone` before their first change. A `recurrence` sent to an instance or a template changes the series' rule, and one sent to another pending task makes it the first instance of a new series. `POST /v1/tasks/{uuid}/stop-recurrence` ends a series as Taskwarrior's `until:today` does; the pending instance stays as the last. Undo reverts every task a change reached, such as a completed instance and the next one.

`POST /v1/project-migrations/preview` lists the pending tasks a project rename moves, with their old and new projects; with `includeSubprojects`, `work.client` moves with `work`, but `workshop` does not. Waiting and recurring tasks keep their project, as in Taskwarrior mode, and backend mode has no contexts, so every other pending task of the user is in scope. `POST /v1/project-migrations/apply`, with a key, sends the preview's tasks back and moves them all in one transaction, or none with a conflict if the rename would no longer move exactly those. Undo refuses a migration. The project catalog stays in the apps' config.

`POST /v1/import/preview` takes up to 1,000 tasks as Taskwarrior exported them, converted to the task shape, and answers which the user lacks, which they have unchanged, and which differ, with the differing fields; it changes nothing. `POST /v1/import/apply`, with a key, creates the lacking ones in one transaction, with their UUIDs, statuses (a template is `recurring` and needs a due date), timestamps to the second, and unrecognized fields in `extra`, and leaves the others as they are, so importing again changes nothing. A UUID another user's task holds is a conflict. Undo refuses an import.

```sh
curl -X POST localhost:8080/v1/tasks -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" -d '{"description":"Try the backend","tags":["next"]}'
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
