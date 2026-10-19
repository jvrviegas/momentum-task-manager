# Contributing to Momentum

## Project shape

Momentum is a monorepo. The root `go.work` joins its Go modules for local builds, and `tui/go.mod` and `backend/go.mod` point at `../core` with a `replace` directive.

- `core/` — Go code shared by the apps and the backend (module `github.com/jvrviegas/momentum-task-manager/core`)
  - `domain` — task decoding, view classification, sorting, edit diffs, date resolution, urgency, and the backend's recurring series
  - `api` — the backend's HTTP contract: routes, request and response types, errors, and the client the apps use
- `tui/` — the terminal app, and the Go code the macOS app shares (module `github.com/jvrviegas/momentum-task-manager/tui`)
  - `cmd/momentum` — CLI parsing, startup validation, and version metadata
  - `internal/app` — Bubble Tea model, messages, commands, actions, undo window, composition
  - `internal/ui` — pure rendering and overlay components
  - `internal/taskwarrior` — reads `task export` for `momentum import`, the only code that runs `task`
  - `internal/backend` — the server client, the saved sign-in, the device's time zone, and the import plan
  - `internal/config` — strict TOML and XDG resolution
  - `internal/quickadd` — trigger parser and suggestions
  - `internal/diagnostics` — doctor checks and redacted logging
  - `internal/bridge`, `cmd/momentum-bridge` — JSON-lines service that exposes the packages above to the macOS app
- `macos/` — native SwiftUI app (see [`macos/README.md`](./macos/README.md)); run `xcodebuild -scheme Momentum test` there when it changes
- `backend/` — the Momentum server in Go and PostgreSQL (module `github.com/jvrviegas/momentum-task-manager/backend`; [ADR 0002](./docs/adr/0002-go-postgres-backend.md), [plan](./docs/plans/backend.md), [README](./backend/README.md))
  - `cmd/momentum-server` — configuration, listening, graceful shutdown, and the `migrate` command
  - `internal/httpapi` — routes, JSON errors, request logging, and the sign-in pages
  - `internal/auth` — device sign-in, Google and GitHub providers, and sessions
  - `internal/tasks` — task reads and changes, recurring series, project migration, idempotency keys, the `task_events` history, and undo
  - `internal/logging` — JSON logs that redact every attribute outside an allowlist
  - `internal/config` — environment variables
  - `internal/store` — PostgreSQL access, migrations, and the local-only test database guard; `ent/schema` is the schema, the rest of `ent/` is generated, and `rule` and `viewer` limit task access to the signed-in user
  - `migrations` — reviewed migration files, written by `cmd/migration-diff`
- `docs/` — decisions and plans that span more than one folder

## Before a change

Read [`tui/DESIGN.md`](./tui/DESIGN.md) and the relevant implementation-plan task. The Momentum server is the source of truth for tasks ([ADR 0002](./docs/adr/0002-go-postgres-backend.md)). In the apps, do not add a local task store, a shell, a background service, telemetry, or access to `~/.task`.

## Tests and safety

Run in each Go module (`core/`, `tui/`, and `backend/`):

```sh
test -z "$(gofmt -l .)"
go test ./...
go test -race ./...
go vet ./...
```

Then build from `tui/`:

```sh
go build ./cmd/momentum
```

When `backend/` or `core/` changes, also build the server image from the repository root:

```sh
docker build -f backend/Dockerfile .
```

Backend store tests run only against a local PostgreSQL server named by `MOMENTUM_TEST_DATABASE_URL`; `docker compose up -d --wait postgres` starts one from [`compose.yaml`](./compose.yaml) (see [`backend/README.md`](./backend/README.md)), and each test uses a database of its own. Never point them, or `cmd/migration-diff`, at Neon. After changing `backend/internal/store/ent/schema`, run `go generate ./...` in `backend/` and commit the generated code with a reviewed migration.

The import's Taskwarrior tests must use temporary `TASKRC` and `TASKDATA` values. They must not run in parallel and must retain the guard that rejects `~/.taskrc` and `~/.task`. Never use the deployed server or a production task database in tests; the apps' tests use fake servers.

UI tests should assert terminal display width with Lip Gloss helpers and cover wide, compact, narrow, empty, loading, modal, suggestion, error, and unreachable-server states. Subprocess code must use `exec.CommandContext` with separate arguments.

## Changes

Keep commits focused and explain any design departure in the plan the work follows: `tui/IMPLEMENTATION_PLAN.md` for the TUI, `docs/plans/backend.md` for the backend. Prefer small pure functions and typed messages over shared callbacks. Run the full local gates before opening a review.

## Release

TUI releases are tag-driven: pushing `tui/vX.Y.Z` builds with GoReleaser and publishes a GitHub release on that tag, with the version's `tui/CHANGELOG.md` section as notes. The `tui/` prefix lets Go resolve `go install …/tui/cmd/momentum@vX.Y.Z` and leaves room for macOS and backend releases. `go install …@version` refuses a module whose `go.mod` has a `replace` directive, so before the first TUI release that includes `core/`, either publish `core` with `core/vX.Y.Z` tags and drop the replace, or change the install instructions to the release archives. Do not create a GitHub repository, publish a release, or add a Homebrew tap without explicit approval. Release targets are Linux/macOS on AMD64/ARM64.
