# Contributing to Momentum

## Project shape

Momentum is a monorepo:

- `tui/` — the terminal app, and the Go code the macOS app shares (module `github.com/jvrviegas/momentum-task-manager/tui`)
  - `cmd/momentum` — CLI parsing, startup validation, and version metadata
  - `internal/app` — Bubble Tea model, messages, commands, actions, sync state, composition
  - `internal/ui` — pure rendering and overlay components
  - `internal/taskwarrior` — direct `task` argv adapter and captured errors
  - `internal/domain` — task decoding, view classification, sorting, and edit diffs
  - `internal/config` — strict TOML and XDG resolution
  - `internal/quickadd` — trigger parser and suggestions
  - `internal/diagnostics` — doctor checks and redacted logging
  - `internal/bridge`, `cmd/momentum-bridge` — JSON-lines service that exposes the packages above to the macOS app
- `macos/` — native SwiftUI app (see [`macos/README.md`](./macos/README.md)); run `xcodebuild -scheme Momentum test` there when it changes
- `backend/` — planned replacement for Taskwarrior; not started

## Before a change

Read [`tui/DESIGN.md`](./tui/DESIGN.md) and the relevant implementation-plan task. Keep Taskwarrior as the source of truth. Do not add persistence, a shell, a background service, telemetry, or direct access to `~/.task`.

## Tests and safety

Run from `tui/`:

```sh
test -z "$(gofmt -l .)"
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/momentum
```

Taskwarrior integration tests must use temporary `TASKRC` and `TASKDATA` values. They must not run in parallel and must retain the guard that rejects `~/.taskrc` and `~/.task`. Never use a real sync server or production task database in tests.

UI tests should assert terminal display width with Lip Gloss helpers and cover wide, compact, narrow, empty, loading, modal, suggestion, error, and local-only states. Subprocess code must use `exec.CommandContext` with separate arguments.

## Changes

Keep commits focused and explain any design departure in `tui/IMPLEMENTATION_PLAN.md`. Prefer small pure functions and typed messages over shared callbacks. Run the full local gates before opening a review.

## Release

TUI releases are tag-driven: pushing `tui/vX.Y.Z` builds with GoReleaser and publishes a GitHub release on that tag, with the version's `tui/CHANGELOG.md` section as notes. The `tui/` prefix lets Go resolve `go install …/tui/cmd/momentum@vX.Y.Z` and leaves room for macOS and backend releases. Do not create a GitHub repository, publish a release, or add a Homebrew tap without explicit approval. Release targets are Linux/macOS on AMD64/ARM64.
