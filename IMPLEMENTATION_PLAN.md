# Momentum Trackable Implementation Plan

**Design:** [`DESIGN.md`](./DESIGN.md)  
**Status:** Ready for implementation  
**Progress:** 20 / 24 tasks complete  
**Module:** `github.com/jvrviegas/momentum`

## Status Legend

- `[ ]` Pending
- `[~]` In progress
- `[x]` Complete
- `[!]` Blocked

## Working Rules

1. Read `DESIGN.md` before implementation and treat its requirements as approved.
2. Confirm current stable Bubble Tea, Bubbles, and Lip Gloss APIs from official Charm documentation before pinning versions.
3. Use one focused commit per task after a local Git repository is initialized.
4. Co-locate tests with every implementation task; do not defer tests to a later task.
5. Never run integration tests against `~/.taskrc` or `~/.task`.
6. Never execute user input through a shell.
7. Update this file's task status, progress count, and completion log after every task.
8. Record any design departure under **Deviations and Decisions** before continuing.

## Prerequisites

- [x] Install the current stable Go toolchain; `go version` was unavailable during planning.
- [x] Confirm `task --version` reports Taskwarrior 3.x (planning environment: 3.4.2).
- [ ] Do not create a GitHub repository yet.
- [ ] Cloud Run and Neon are manual user setup and do not block local-only application development.

## Gate Commands

| Gate | Commands |
|---|---|
| Format | `test -z "$(gofmt -l .)"` |
| Unit | `go test ./...` |
| Vet | `go vet ./...` |
| Build | `go build ./cmd/momentum` |
| Race | `go test -race ./...` |
| Full | Format + Unit + Vet + Build |
| Cross-platform | `GOOS=linux GOARCH=amd64 go build ./cmd/momentum` and `GOOS=darwin GOARCH=arm64 go build ./cmd/momentum` |

Integration tests that require Taskwarrior must create temporary `TASKRC` and `data.location` values and restore the process environment afterward. They may skip with an explicit reason when a compatible `task` binary is unavailable.

## Test Coverage Matrix

| Layer | Required tests | Parallel-safe? | Minimum coverage expectation |
|---|---|---:|---|
| `internal/domain` | Unit | Yes | Classification, grouping, ordering, edit diff |
| `internal/config` | Unit | Yes | Defaults, XDG resolution, strict validation |
| `internal/quickadd` | Unit | Yes | Every trigger, escaping, duplicates, suggestions |
| `internal/taskwarrior` | Unit + isolated integration | Integration tests: No | Argv, decoding, errors, real temporary Taskwarrior |
| `internal/app` | Unit/update-loop | Yes with fake client | State transitions, timers, modal precedence |
| `internal/ui` | Rendering/component tests | Yes | Normal, responsive, empty, overlay, error states |
| `internal/diagnostics` | Unit | Yes | Redaction, checks, no mutation |
| `cmd/momentum` | CLI tests + build | Yes | Commands, flags, exit behavior |

## Execution Map

```text
T01 -> T02
       |-> T03 [P]
       |-> T04 [P]
       |-> T05 [P]
       `-> T06 [P]

T03 + T04 + T05 + T06 -> T07
T04 + T05             -> T08 [P]
T03 + T04             -> T09 [P]
T04                   -> T10 [P]

T02 + T07 -> T11
T11 -> T12
T11 -> T13 [P]
T11 -> T14 [P]
T11 -> T15 [P]
T11 -> T16 [P]
T08 + T11 -> T17
T09 + T11 -> T18
T10 + T11 -> T19

T12 + T13 + T14 + T15 + T16 + T17 + T18 + T19 -> T20
T07 + T20 -> T21
T20 -> T22 [P]
T21 -> T23
T22 + T23 -> T24
```

Tasks marked `[P]` may run in parallel only when their dependencies are complete and they do not modify shared files concurrently.

---

## Phase 1 — Foundation

### [x] T01: Scaffold the Go module and repository layout

**What:** Create the minimal compilable module, package directories, entry point, license, and ignore rules.  
**Where:** `go.mod`, `cmd/momentum/main.go`, `internal/*/doc.go`, `LICENSE`, `.gitignore`  
**Depends on:** None  
**Requirements:** Architecture, Distribution  
**Commit:** `chore: scaffold momentum go module`

**Done when:**

- [x] Module path is `github.com/jvrviegas/momentum`.
- [x] Package boundaries match `DESIGN.md`.
- [x] No GitHub remote is created.
- [x] `go build ./cmd/momentum` passes.
- [x] `go test ./...` passes.

**Verify:**

```sh
go list ./...
go build ./cmd/momentum
go test ./...
```

### [x] T02: Pin the terminal UI and configuration dependencies

**What:** Verify official current APIs, add only required dependencies, and document version rationale.  
**Where:** `go.mod`, `go.sum`, `docs/dependencies.md`  
**Depends on:** T01  
**Requirements:** Architecture, Visual Design  
**Commit:** `chore: add verified tui dependencies`

**Done when:**

- [x] Bubble Tea, Bubbles, Lip Gloss, and a strict TOML decoder are pinned.
- [x] Each dependency is justified in `docs/dependencies.md`.
- [x] No unnecessary CLI framework or fuzzy library is added.
- [x] A minimal Bubble Tea model compiles.
- [x] Full gate passes.

**Verify:**

```sh
go mod tidy
go mod verify
go test ./...
go vet ./...
go build ./cmd/momentum
```

---

## Phase 2 — Independent Core Packages

### [x] T03: Implement domain task decoding types [P]

**What:** Define Taskwarrior export-facing and domain task types with optional timestamps and retained raw fields.  
**Where:** `internal/domain/task.go`, `internal/domain/task_test.go`  
**Depends on:** T02  
**Requirements:** ROW-01–ROW-02, DETAIL-02, Task data model  
**Commit:** `feat(domain): define task model`

**Done when:**

- [x] UUID, description, status, project, priority, dates, start, wait, tags, annotations, dependencies, recurrence, urgency, ID, and raw fields are represented.
- [x] Optional fields remain distinguishable from zero values.
- [x] Timestamp fixtures parse correctly.
- [x] At least 6 focused test cases pass.
- [x] `go test ./internal/domain` passes.

### [x] T04: Implement view classification and sorting [P]

**What:** Build pure Inbox/Today derivation, Today grouping, deduplication, deterministic ordering, and UUID selection restoration.  
**Where:** `internal/domain/views.go`, `internal/domain/views_test.go`  
**Depends on:** T02  
**Requirements:** VIEW-01–VIEW-10  
**Commit:** `feat(domain): derive inbox and today views`

**Done when:**

- [x] Inbox includes all pending input tasks.
- [x] Today precedence exactly matches the design.
- [x] Scheduled-before-today alone does not classify as overdue.
- [x] Sorting uses descending urgency with deterministic ties.
- [x] Local timezone and midnight boundary cases are tested.
- [x] At least 12 table-driven cases pass.
- [x] `go test ./internal/domain` passes.

### [x] T05: Implement strict configuration loading [P]

**What:** Add defaults, XDG path resolution, TOML decoding, validation, duration parsing, and icon environment override.  
**Where:** `internal/config/config.go`, `internal/config/config_test.go`  
**Depends on:** T02  
**Requirements:** REFRESH-02–REFRESH-03, Configuration, Sync configuration  
**Commit:** `feat(config): load and validate momentum settings`

**Done when:**

- [x] Missing config uses approved defaults.
- [x] CLI path override outranks XDG/default paths.
- [x] Unknown keys and invalid enum/duration values fail clearly.
- [x] `0s` refresh disables background refresh.
- [x] `MOMENTUM_ICONS` overrides TOML.
- [x] At least 10 test cases pass without reading user config.
- [x] `go test ./internal/config` passes.

### [x] T06: Implement quick-add parsing and suggestion context [P]

**What:** Parse command-bar text into description and typed metadata, detect active suggestion context, and support escaping.  
**Where:** `internal/quickadd/parser.go`, `internal/quickadd/suggest.go`, corresponding `_test.go` files  
**Depends on:** T02  
**Requirements:** ADD-01–ADD-06 and trigger grammar  
**Commit:** `feat(quickadd): parse task capture syntax`

**Done when:**

- [x] All five triggers parse at token boundaries.
- [x] Email addresses remain description text.
- [x] Escaped triggers become literals.
- [x] Duplicate scalar fields and empty descriptions return typed errors.
- [x] Multiple unique tags work.
- [x] Suggestion context tracks token and cursor position.
- [x] Fuzzy ordering is deterministic.
- [x] At least 20 parser/suggestion cases pass.
- [x] `go test ./internal/quickadd` passes.

---

## Phase 3 — Taskwarrior and Sync Core

### [x] T07: Implement the Taskwarrior read adapter

**What:** Execute pending export and project discovery safely, decode JSON, capture typed errors, and respect active context.  
**Where:** `internal/taskwarrior/client.go`, `export.go`, `errors.go`, tests and fixtures  
**Depends on:** T03, T04, T05, T06  
**Requirements:** VIEW-01, VIEW-10, Taskwarrior Integration  
**Commit:** `feat(taskwarrior): load pending tasks`

**Done when:**

- [x] Commands use `exec.CommandContext` and argv only.
- [x] Export fixtures decode into domain tasks.
- [x] Non-zero exits retain redacted stderr and exit metadata.
- [x] Project suggestions combine script-oriented output and exported projects.
- [x] Actual user tags are collected without virtual tags.
- [x] Active context can be shown without changing it.
- [x] Unit tests use a fake executable/runner.
- [x] At least 10 adapter tests pass.
- [x] `go test ./internal/taskwarrior` passes.

### [x] T08: Implement UUID-based mutation commands [P]

**What:** Add safe argv builders/executors for add, modify, done, delete, start, stop, and undo.  
**Where:** `internal/taskwarrior/mutations.go`, `mutations_test.go`  
**Depends on:** T04, T05  
**Requirements:** MUT-01–MUT-09, ADD-05, EDIT-05–EDIT-07  
**Commit:** `feat(taskwarrior): add task mutation adapter`

**Done when:**

- [x] Every existing-task mutation uses UUID, not numeric ID.
- [x] User input never enters a shell command string.
- [x] Deletion/undo can run non-interactively after app confirmation.
- [x] Hook and validation failures return errors.
- [x] Clearing supported fields produces correct Taskwarrior args.
- [x] Add translates every quick-add field correctly.
- [x] At least 16 exact-argv cases pass.
- [x] `go test ./internal/taskwarrior` passes.

### [x] T09: Implement edit snapshot and diff generation [P]

**What:** Convert tasks into structured edit fields and generate minimal supported-field modifications.  
**Where:** `internal/domain/edit.go`, `internal/domain/edit_test.go`  
**Depends on:** T03, T04  
**Requirements:** EDIT-01–EDIT-07  
**Commit:** `feat(domain): generate non-destructive task edits`

**Done when:**

- [x] Unchanged fields produce an empty diff.
- [x] Clearing each optional field is represented distinctly.
- [x] Tag additions/removals are deterministic.
- [x] Unsupported/raw fields never enter the diff.
- [x] At least 12 edit cases pass.
- [x] `go test ./internal/domain` passes.

### [x] T10: Implement synchronization state machine [P]

**What:** Model startup sync, periodic sync, mutation delay, undo window, retries, manual sync, and shutdown decisions as pure transitions.  
**Where:** `internal/app/sync_state.go`, `sync_state_test.go`  
**Depends on:** T04  
**Requirements:** Synchronization Design, REFRESH-04  
**Commit:** `feat(sync): model automatic synchronization`

**Done when:**

- [x] Startup, 5-minute periodic, and 15-second mutation timers are represented.
- [x] Retry progression is 15s, 30s, 1m, 2m, 5m capped.
- [x] Success resets backoff.
- [x] Manual sync closes the undo window.
- [x] Disabled/local-only behavior schedules no sync.
- [x] Unsynced quit choices are represented.
- [x] At least 15 state-transition cases pass.
- [x] `go test ./internal/app` passes.

---

## Phase 4 — Application Shell and UI Components

### [x] T11: Implement root application state and message contracts

**What:** Create the root Bubble Tea model, typed messages, focus/modal precedence, async command dispatch, and fake-client test harness.  
**Where:** `internal/app/model.go`, `messages.go`, `commands.go`, tests  
**Depends on:** T02, T07  
**Requirements:** Asynchronous execution, MUT-05, REFRESH-04–REFRESH-05  
**Commit:** `feat(app): add root bubble tea state machine`

**Done when:**

- [x] Initial loading, ready, refreshing, mutating, modal, and error states are explicit.
- [x] No client call blocks `Update`.
- [x] Mutations are serialized.
- [x] Overlay input takes precedence over global keys.
- [x] Existing content remains during refresh.
- [x] At least 12 update-loop tests pass with a fake client.
- [x] `go test ./internal/app` passes.

### [x] T12: Implement theme and responsive layout primitives

**What:** Add Tokyo Night-inspired dark/light themes, icon modes, breakpoints, width-safe truncation, and shared styles.  
**Where:** `internal/ui/theme.go`, `layout.go`, `icons.go`, tests  
**Depends on:** T11  
**Requirements:** ROW-03–ROW-04, Visual Design  
**Commit:** `feat(ui): add adaptive momentum theme`

**Done when:**

- [x] Auto/dark/light themes resolve deterministically in tests.
- [x] Unicode, Nerd Font, and ASCII icon sets exist.
- [x] Layout chooses sidebar, tabs, or minimum-size state by width.
- [x] Width calculations handle Unicode safely.
- [x] At least 10 rendering/layout cases pass.
- [x] `go test ./internal/ui` passes.

### [x] T13: Implement sidebar, tabs, task list, and empty states [P]

**What:** Render view navigation, counts, Today sections, compact task rows, selection, scrolling, and empty content.  
**Where:** `internal/ui/sidebar.go`, `tasklist.go`, `empty.go`, tests  
**Depends on:** T11  
**Requirements:** VIEW-03, VIEW-09, ROW-01–ROW-04, Empty states  
**Commit:** `feat(ui): render task views`

**Done when:**

- [x] Wide mode shows sidebar; compact mode shows top tabs.
- [x] Today section ordering and labels are correct.
- [x] Rows hide metadata progressively at narrow widths.
- [x] Empty-state copy matches the design.
- [x] Selection and scrolling remain visible.
- [x] At least 10 rendering cases pass.
- [x] `go test ./internal/ui` passes.

### [x] T14: Implement quick-add command bar and suggestions [P]

**What:** Build the command-bar UI, contextual suggestion popup, keyboard behavior, parse errors, and submission messages.  
**Where:** `internal/ui/quickadd.go`, tests  
**Depends on:** T11  
**Requirements:** ADD-01–ADD-06, Autocomplete behavior  
**Commit:** `feat(ui): add contextual quick capture bar`

**Done when:**

- [x] `Ctrl+K`, `Tab`, arrows, Ctrl+P/N, Enter, and two-stage Escape work.
- [x] Suggestions render above the input and stay inside terminal bounds.
- [x] Parse errors display without losing entered text.
- [x] Submission emits typed app messages, not direct process calls.
- [x] At least 12 interaction cases pass.
- [x] `go test ./internal/ui` passes.

### [x] T15: Implement structured edit modal [P]

**What:** Build six editable fields, direct initial focus, keyboard traversal, suggestions, changed-state display, save, and cancel.  
**Where:** `internal/ui/edit.go`, tests  
**Depends on:** T11  
**Requirements:** EDIT-01–EDIT-07  
**Commit:** `feat(ui): add structured task editor`

**Done when:**

- [x] Direct shortcuts focus the specified field.
- [x] Up/Down and Tab/Shift+Tab traverse fields.
- [x] Suggestion-open behavior temporarily owns Up/Down.
- [x] Project/tag/date autocomplete and priority selection work.
- [x] Ctrl+S emits only a validated edit submission.
- [x] Escape discards changes.
- [x] At least 15 interaction cases pass.
- [x] `go test ./internal/ui` passes.

### [x] T16: Implement details, confirmation, help, and quit modals [P]

**What:** Add read-only details, delete confirmation, generated key help, minimum-size warning, and unsynced-quit choices.  
**Where:** `internal/ui/details.go`, `confirm.go`, `help.go`, `quit.go`, tests  
**Depends on:** T11  
**Requirements:** DETAIL-01–DETAIL-04, MUT-03, Keyboard Map, Shutdown  
**Commit:** `feat(ui): add task and confirmation overlays`

**Done when:**

- [x] Details show supported and useful raw fields safely.
- [x] `e` transitions from details to edit.
- [x] Delete accepts only explicit confirmation.
- [x] Help derives text from key definitions.
- [x] Quit modal supports sync, local quit, and cancel.
- [x] At least 12 modal cases pass.
- [x] `go test ./internal/ui` passes.

---

## Phase 5 — Feature Integration

### [x] T17: Wire task actions and refresh lifecycle

**What:** Connect quick add, edit, completion, start/stop, deletion, undo, refresh, toasts, and stable selection to the adapter.  
**Where:** `internal/app/actions.go`, root model tests  
**Depends on:** T08, T11  
**Requirements:** MUT-01–MUT-10, REFRESH-01–REFRESH-05, VIEW-10  
**Commit:** `feat(app): connect task actions and refresh`

**Done when:**

- [x] Each action dispatches the expected async client method.
- [x] Exactly one refresh follows each success.
- [x] Errors preserve visible state.
- [x] Completion/deletion choose the next valid selection.
- [x] Auto-refresh pauses during active input overlays.
- [x] At least 14 update-loop cases pass.
- [x] `go test ./internal/app` passes.

### [x] T18: Wire structured editing end to end

**What:** Connect direct edit shortcuts, modal snapshots, minimal diffs, mutation submission, and error recovery.  
**Where:** `internal/app/edit.go`, tests  
**Depends on:** T09, T11  
**Requirements:** EDIT-01–EDIT-07  
**Commit:** `feat(app): connect structured task editing`

**Done when:**

- [x] Each direct shortcut opens the correct task and field.
- [x] Empty diffs close without invoking Taskwarrior.
- [x] Supported changes invoke one modify operation.
- [x] Rejected edits retain user input and show the error.
- [x] At least 10 app edit-flow cases pass.
- [x] `go test ./internal/app` passes.

### [x] T19: Wire synchronization, retry, undo countdown, and shutdown

**What:** Connect the sync state machine to timers and `task sync`, refresh after sync, footer status, manual sync, and quit flow.  
**Where:** `internal/taskwarrior/sync.go`, `internal/app/sync.go`, tests  
**Depends on:** T10, T11  
**Requirements:** Synchronization Design, Local-only behavior, Shutdown  
**Commit:** `feat(sync): integrate taskwarrior synchronization`

**Done when:**

- [x] Local tasks render before startup sync begins.
- [x] Successful sync triggers refresh and resets retry state.
- [x] Mutation delay and undo countdown are visible.
- [x] Failed sync leaves the application operational.
- [x] Manual sync and all quit choices work.
- [x] No timers are installed outside the running process.
- [x] At least 16 sync-flow cases pass.
- [x] `go test ./internal/app ./internal/taskwarrior` passes.

### [x] T20: Integrate navigation, search, mouse, and final responsive composition

**What:** Compose all UI components and implement complete keyboard/mouse routing and local fuzzy search.  
**Where:** `internal/app/update.go`, `internal/app/view.go`, `internal/ui/search.go`, tests  
**Depends on:** T12, T13, T14, T15, T16, T17, T18, T19  
**Requirements:** SEARCH-01–SEARCH-05, Keyboard and Mouse Map, Visual Design  
**Commit:** `feat(app): compose interactive momentum interface`

**Done when:**

- [x] Every approved global key works in the correct state.
- [x] Overlay keys cannot accidentally trigger global mutations.
- [x] Search filters description/project/tags and reports matches.
- [x] Limited mouse behavior works.
- [x] Wide, compact, narrow, and minimum-size rendering passes tests.
- [x] At least 20 composition/update cases pass.
- [x] Full gate and race gate pass.

---

## Phase 6 — CLI, Diagnostics, and Real Integration

### [ ] T21: Implement CLI commands, diagnostics, and redacted logging

**What:** Add inbox/today startup commands, config/debug/version/help flags, doctor checks, XDG state logs, and redaction.  
**Where:** `cmd/momentum/*`, `internal/diagnostics/*`, tests  
**Depends on:** T07, T20  
**Requirements:** CLI Surface, Diagnostics, Privacy  
**Commit:** `feat(cli): add doctor and runtime options`

**Done when:**

- [ ] Every documented command/flag parses and exits correctly.
- [ ] `doctor` performs no task mutation and prints no secrets.
- [ ] Debug logs use XDG state paths and redact sensitive values.
- [ ] Startup validates Taskwarrior availability/version.
- [ ] Inbox/today arguments override automatic startup selection.
- [ ] At least 14 CLI/diagnostic/redaction cases pass.
- [ ] Full gate passes.

### [ ] T22: Add isolated Taskwarrior integration tests [P]

**What:** Exercise export, add, modify, done, start/stop, delete, undo, context, and error handling against temporary Taskwarrior data.  
**Where:** `internal/taskwarrior/integration_test.go`, `internal/taskwarrior/testdata/`  
**Depends on:** T20  
**Requirements:** Taskwarrior Integration, Testing Strategy  
**Commit:** `test(taskwarrior): add isolated integration coverage`

**Done when:**

- [ ] Tests set temporary config and data paths.
- [ ] A guard fails before any test can address the real user database.
- [ ] Lifecycle tests cover every v1 mutation.
- [ ] Export and active-context behavior are verified on Taskwarrior 3.x.
- [ ] Hooks/validation failure propagation has coverage.
- [ ] At least 8 integration scenarios pass when Taskwarrior is available.
- [ ] `go test ./internal/taskwarrior -run Integration -v` passes.

### [ ] T23: Perform cross-platform build and interactive UAT

**What:** Validate the complete application on Linux first, then macOS, recording results and any approved breakpoint/style adjustments.  
**Where:** `docs/UAT.md`, implementation fixes as required  
**Depends on:** T21  
**Requirements:** All acceptance criteria  
**Commit:** `test: validate momentum user workflows`

**Done when:**

- [ ] Full and race gates pass on Linux.
- [ ] Linux UAT covers empty/add/edit/search/actions/offline/sync/quit.
- [ ] macOS full gate and equivalent UAT pass.
- [ ] Terminal widths above 100, around 80, around 50, and below minimum are checked.
- [ ] Unicode, Nerd Font, and ASCII modes are checked.
- [ ] No test touches production Taskwarrior data.
- [ ] `docs/UAT.md` records pass/fail evidence for every acceptance criterion.

---

## Phase 7 — Documentation and Release Readiness

### [ ] T24: Add user documentation, CI, and release configuration

**What:** Complete README, contributing guide, example config, manual sync guide, screenshots placeholders, CI, and tagged release automation.  
**Where:** `README.md`, `CONTRIBUTING.md`, `config.example.toml`, `docs/sync.md`, `.github/workflows/*`, `.goreleaser.yaml`  
**Depends on:** T22, T23  
**Requirements:** Distribution, Dotfiles, Self-hosted Sync, Acceptance Criteria  
**Commit:** `docs: prepare momentum for initial release`

**Done when:**

- [ ] README documents requirements, install, quick add, edit, keys, config, and privacy.
- [ ] Sync guide covers manual Neon/Cloud Run setup without embedding secrets.
- [ ] Dotfiles guidance covers tracked `.taskrc`, untracked `secrets.rc`, and mode `0600`.
- [ ] CI runs formatting, tests, vet, and Linux/macOS build checks.
- [ ] GoReleaser targets Linux/macOS on AMD64/ARM64.
- [ ] No GitHub repository or release is created without separate user approval.
- [ ] Full and cross-platform gates pass locally.

---

## Dependency Cross-check

| Task | Declared dependencies | Execution map | Status |
|---|---|---|---|
| T01 | None | Root | Match |
| T02 | T01 | T01 -> T02 | Match |
| T03 | T02 | T02 -> T03 | Match |
| T04 | T02 | T02 -> T04 | Match |
| T05 | T02 | T02 -> T05 | Match |
| T06 | T02 | T02 -> T06 | Match |
| T07 | T03, T04, T05, T06 | Four-way join -> T07 | Match |
| T08 | T04, T05 | T04 + T05 -> T08 | Match |
| T09 | T03, T04 | T03 + T04 -> T09 | Match |
| T10 | T04 | T04 -> T10 | Match |
| T11 | T02, T07 | T02 + T07 -> T11 | Match |
| T12 | T11 | T11 -> T12 | Match |
| T13 | T11 | T11 -> T13 | Match |
| T14 | T11 | T11 -> T14 | Match |
| T15 | T11 | T11 -> T15 | Match |
| T16 | T11 | T11 -> T16 | Match |
| T17 | T08, T11 | T08 + T11 -> T17 | Match |
| T18 | T09, T11 | T09 + T11 -> T18 | Match |
| T19 | T10, T11 | T10 + T11 -> T19 | Match |
| T20 | T12–T19 | Eight-way join -> T20 | Match |
| T21 | T07, T20 | T07 + T20 -> T21 | Match |
| T22 | T20 | T20 -> T22 | Match |
| T23 | T21 | T21 -> T23 | Match |
| T24 | T22, T23 | T22 + T23 -> T24 | Match |

## Test Co-location Cross-check

| Tasks | Layer | Required | Planned | Status |
|---|---|---|---|---|
| T03, T04, T09 | Domain | Unit | Tests in each task | Match |
| T05 | Config | Unit | Tests in task | Match |
| T06 | Quick add | Unit | Tests in task | Match |
| T07, T08, T19, T22 | Taskwarrior | Unit + isolated integration | Unit co-located; integration after runnable composition | Match |
| T10, T11, T17–T20 | App | Update-loop/unit | Tests in each task | Match |
| T12–T16, T20 | UI | Rendering/component | Tests in each task | Match |
| T21 | Diagnostics/CLI | Unit + build | Tests and full gate in task | Match |
| T23 | Complete product | UAT | UAT evidence in task | Match |
| T24 | Docs/CI | Build/CI validation | Cross-platform/full gates in task | Match |

## Manual Cloud Run/Neon Checklist

This checklist is operational guidance, not an implementation dependency.

- [ ] Create a Neon PostgreSQL project/database.
- [ ] Pin a TaskChampion PostgreSQL server image release.
- [ ] Apply that release's `postgres/schema.sql` in Neon.
- [ ] Generate one client UUID with `uuidgen`.
- [ ] Generate a separate random 256-bit encryption secret.
- [ ] Pre-create the allowed client row or bootstrap once before setting `CREATE_CLIENTS=false`.
- [ ] Store Neon's direct TLS connection string in Google Secret Manager.
- [ ] Deploy the official PostgreSQL image to Cloud Run.
- [ ] Configure `CONNECTION`, `CLIENT_ID`, `CREATE_CLIENTS=false`, and appropriate logging.
- [ ] Set minimum instances to 0 and maximum instances to 1.
- [ ] Permit unauthenticated ingress and retain the generated HTTPS URL.
- [ ] Create Linux `secrets.rc` with mode `0600` and configure the shared values.
- [ ] Run the first `task sync` from Linux.
- [ ] Configure macOS as an empty second replica and run `task sync`.
- [ ] Set recurrence on for Linux and off for macOS.
- [ ] Verify a task created and synced from each machine reaches the other after two sync calls.
- [ ] Store client UUID and encryption secret in a password manager.

## Deferred Backlog

- [ ] Custom smart views and arbitrary Taskwarrior filters.
- [ ] Configurable keybindings with conflict validation.
- [ ] Context switching inside Momentum.
- [ ] Annotation, dependency, recurrence, and UDA editing.
- [ ] Bulk selection and operations.
- [ ] Task duplication.
- [ ] Desktop notifications/background agent.
- [ ] Homebrew tap.
- [ ] Dedicated conflict-resolution UI if real-world use requires it.
- [ ] Native Windows support.

## Deviations and Decisions

Record implementation-time departures here before proceeding.

| Date | Task | Decision/deviation | Reason | Approved by |
|---|---|---|---|---|
| 2026-09-08 | T01 | `chore: scaffold momentum go module` | `go list ./...`; `go build ./cmd/momentum`; `go test ./...` | Go 1.27.1 installed; Taskwarrior 3.5.0 confirmed. |
| 2026-09-08 | T02 | `chore: add verified tui dependencies` | `go mod verify`; `go test ./...`; `go vet ./...`; `go build ./cmd/momentum` | Charm v2 modules and BurntSushi TOML v1.6.0 pinned; API/version rationale in `docs/dependencies.md`. |
| 2026-09-08 | T03 | `feat(domain): define task model` | `go test ./internal/domain` | Export dates parse into optional localizable times; raw and unknown JSON properties retained. |
| 2026-09-08 | T04 | `feat(domain): derive inbox and today views` | `go test ./internal/domain` | Pure local-date classification, precedence, section grouping, urgency sorting, and UUID selection restoration implemented. |
| 2026-09-08 | T05 | `feat(config): load and validate momentum settings` | `go test ./internal/config` | Defaults, injectable XDG resolution, strict TOML unknown-key checks, duration validation, and icon override implemented. |
| 2026-09-08 | T06 | `feat(quickadd): parse task capture syntax` | `go test ./internal/quickadd` | Boundary-aware five-trigger parser, escaping, typed errors, and deterministic contextual suggestions implemented. |
| 2026-09-08 | T09 | `feat(domain): generate non-destructive task edits` | `go test ./internal/domain ./internal/taskwarrior` | Editable snapshots, explicit clears, deterministic tag set diffs, and unsupported-field isolation implemented. |
| 2026-09-08 | T07 | `feat(taskwarrior): load pending tasks` | `go test ./internal/taskwarrior` | Direct argv runner, pending export decoding, project/tag discovery, context read, finite timeout, and redacted typed errors implemented. |
| 2026-09-08 | T08 | `feat(taskwarrior): add task mutation adapter` | `go test ./internal/taskwarrior` | UUID-only add/modify/completion/start-stop/delete/undo argv builders and execution paths implemented with non-interactive deletion. |
| 2026-09-08 | T10 | `feat(sync): model automatic synchronization` | `go test ./internal/app` | Pure startup/periodic/mutation timers, capped retry backoff, undo grace, manual sync, and shutdown transitions implemented. |
| 2026-09-08 | T11 | `feat(app): add root bubble tea state machine` | `go test ./internal/app` | Typed messages, async command factories, explicit loading/ready/refresh/mutation/error states, serialized mutation gate, and UUID selection restoration implemented. |
| 2026-09-08 | T12 | `feat(ui): add adaptive momentum theme` | `go test ./internal/ui` | Tokyo Night-inspired dark/light palettes, explicit icon sets, centralized responsive breakpoints, and display-width-safe truncation implemented. |
| 2026-09-08 | T13 | `feat(ui): render task views` | `go test ./internal/ui` | Responsive sidebar/tabs, counts, width-safe task rows, selection scrolling, and approved Inbox/Today empty states implemented. |
| 2026-09-08 | T14 | `feat(ui): add contextual quick capture bar` | `go test ./internal/ui` | Bubbles text input, trigger-aware suggestions, keyboard navigation, bounded popup rendering, typed submission, and non-destructive parse errors implemented. |
| 2026-09-08 | T15 | `feat(ui): add structured task editor` | `go test ./internal/ui ./internal/domain` | Six-field structured editor with direct focus, traversal precedence, field suggestions, changed-state markers, validation, minimal diff submission, and cancellation implemented. |
| 2026-09-08 | T16 | `feat(ui): add task and confirmation overlays` | `go test ./internal/ui` | Read-only details with raw fields, destructive y/n confirmation, generated key help, minimum-size warning, and unsynced quit choices implemented. |
| 2026-09-08 | T17 | `feat(app): connect task actions and refresh` | `go test ./internal/app` | Selected-task actions, confirmation-gated deletion, quick-add/edit routing, serialized mutation dispatch, success refresh, stale-content errors, and selection preservation implemented. |
| 2026-09-08 | T18 | `feat(app): connect structured task editing` | `go test ./internal/app` | Direct field shortcuts, minimal diff submission, empty-diff short circuit, UUID modify routing, and rejected-edit input retention implemented. |
| 2026-09-08 | T19 | `feat(sync): integrate taskwarrior synchronization` | `go test ./internal/app ./internal/taskwarrior` | Native sync/config-readiness adapter, startup/periodic/retry timers, local-only fallback, undo countdown, manual sync, and shutdown flow integrated. |
| 2026-09-08 | T20 | `feat(app): compose interactive momentum interface` | `go test ./...` | Responsive full-screen composition, local fuzzy search, global/overlay routing, keyboard navigation, limited mouse interaction, and modal rendering integrated. |

## Completion Log

| Date | Task | Commit | Verification | Notes |
|---|---|---|---|---|
| — | — | — | — | — |
