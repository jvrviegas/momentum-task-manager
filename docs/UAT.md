# Momentum UAT

Date: 2026-09-08  
Host: macOS arm64 (`Darwin Mac.lan`), Go 1.27.1, Taskwarrior 3.5.0  
Linux runtime: Debian sid arm64 container, Go 1.27.1, Taskwarrior 3.5.0  
Scope: local-only and isolated Taskwarrior workflows; no sync server was configured.

## Automated gates

| Gate | Result | Evidence |
|---|---|---|
| Format | PASS | `test -z "$(gofmt -l .)"` |
| Unit | PASS | `go test ./...` |
| Vet | PASS | `go vet ./...` |
| Build | PASS | `go build ./cmd/momentum` |
| Race | PASS | `go test -race ./...` |
| Linux amd64 build | PASS | `GOOS=linux GOARCH=amd64 go build ./cmd/momentum` |
| macOS arm64 build | PASS | `GOOS=darwin GOARCH=arm64 go build ./cmd/momentum` |
| Linux full/race gates | PASS | `golang:1.27` container: format, unit, vet, build, race |
| Real Taskwarrior integration (macOS) | PASS | `go test ./internal/taskwarrior -run Integration -v` (8 isolated scenarios) |
| Real Taskwarrior integration (Linux) | PASS | Debian sid arm64 container with Taskwarrior 3.5.0, 8 isolated scenarios |

## Runtime workflows

All real Taskwarrior checks create a temporary `TASKRC` and `TASKDATA` directory under `t.TempDir()`. The integration guard rejects `~/.taskrc` and `~/.task` before any command is started.

| Workflow | Result | Evidence |
|---|---|---|
| Empty local-only launch and quit (macOS) | PASS | Temporary Taskwarrior data, `[sync].enabled = false`, `expect` smoke run |
| Quick add with project and tag (macOS) | PASS | Temporary TUI run sent `Ctrl+K`, `macOS UAT task #work +testing`, Enter, quit, and verified isolated export |
| Empty/local-only launch and quick add (Linux) | PASS | Debian sid arm64 container, actual Taskwarrior 3.5.0, temporary data, `expect` TTY run |
| Linux edit/search/details/start-stop/delete-confirm/quit routing | PASS | Debian sid arm64 container, actual Taskwarrior 3.5.0, `expect` sent `e`, `/`, Enter/Esc, Enter/Esc, `s`, `s`, `d`, `n`, `q`; export remained intact |
| Add/export/UUID edit/clear | PASS | `TestIntegrationAddAndExport`, `TestIntegrationModifyByUUIDAndClearFields` |
| Complete/start/stop/delete/undo | PASS | `TestIntegrationStartStopDoneLifecycle`, `TestIntegrationDeleteAndUndo` |
| Search and match count | PASS | `internal/ui/search_test.go`, `internal/app/composition_test.go` |
| Offline/retry/undo/shutdown state behavior | PASS | `internal/app/sync_state_test.go`, `internal/app/sync_test.go` |
| Wide terminal (120 columns) | PASS | responsive composition tests |
| Compact terminal (79 columns) | PASS | responsive composition tests |
| Narrow terminal (49 columns) | PASS | metadata suppression tests |
| Minimum terminal (20x5) | PASS | minimum-size rendering tests |
| Unicode/Nerd Font/ASCII icon modes | PASS | `internal/ui/theme_test.go` |

## Acceptance evidence

| # | Criterion | Result |
|---:|---|---|
| 1 | Taskwarrior 3.x launch | PASS on macOS; Linux runtime not available on this host |
| 2 | Inbox/Today overlapping semantics | PASS — domain table tests |
| 3 | Today grouping and no duplicates | PASS — domain and composition tests |
| 4 | Safe quick add and five triggers | PASS — parser, adapter, and runtime smoke tests |
| 5 | Structured edit and unsupported-field preservation | PASS — domain/UI/app/integration tests |
| 6 | UUID mutations and undo | PASS — exact argv and isolated integration tests |
| 7 | Search, navigation, mouse, details, help, responsive layouts | PASS — app/UI tests |
| 8 | Async refresh/sync without blocking Update | PASS — typed command/update-loop tests |
| 9 | Offline/retry/undo-delay/unsynced quit | PASS — pure sync state and app flow tests |
| 10 | Local-only mode | PASS — config, doctor, and runtime smoke tests |
| 11 | No direct Taskwarrior database access | PASS — adapter uses `exec.CommandContext`; isolation guard |
| 12 | Secret-safe diagnostics/logging | PASS — redaction and doctor tests |
| 13 | Isolated automated tests | PASS — full and race gates; integration guard |
| 14 | Installation/configuration/control/sync documentation | PENDING T24 documentation task |

## Reproduction notes

The Linux checks run in an ephemeral Debian sid arm64 container. The production Taskwarrior database is never mounted; each run creates a disposable `TASKRC`/`TASKDATA` pair. The amd64 artifact is also compiled separately as the release target; runtime UAT was executed on Linux arm64 because that is the available container architecture. Sync-network behavior is covered with pure state tests because no self-hosted sync server is provisioned for local UAT.
