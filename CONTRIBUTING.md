# Contributing to Momentum

## Project shape

- `cmd/momentum` — CLI parsing, startup validation, and version metadata
- `internal/app` — Bubble Tea model, messages, commands, actions, sync state, composition
- `internal/ui` — pure rendering and overlay components
- `internal/taskwarrior` — direct `task` argv adapter and captured errors
- `internal/domain` — task decoding, view classification, sorting, and edit diffs
- `internal/config` — strict TOML and XDG resolution
- `internal/quickadd` — trigger parser and suggestions
- `internal/diagnostics` — doctor checks and redacted logging

## Before a change

Read [`DESIGN.md`](./DESIGN.md) and the relevant implementation-plan task. Keep Taskwarrior as the source of truth. Do not add persistence, a shell, a background service, telemetry, or direct access to `~/.task`.

## Tests and safety

Run:

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

Keep commits focused and explain any design departure in `IMPLEMENTATION_PLAN.md`. Prefer small pure functions and typed messages over shared callbacks. Run the full local gates before opening a review.

## Release

Releases are tag-driven and use GoReleaser. Do not create a GitHub repository, publish a release, or add a Homebrew tap without explicit approval. Release targets are Linux/macOS on AMD64/ARM64.
