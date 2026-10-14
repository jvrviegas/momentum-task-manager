# Implementation plan — Go backend replacing Taskwarrior

**Created:** 2026-10-14  
**Status:** P1–P4 and D1–D10 are approved (2026-10-14). T02 is complete; the other tasks have not started.  
**Baseline:** `d9bce06` on `main`. Recheck HEAD and the working tree before starting.  
**Decision record:** [ADR 0002 / P1–P4](../adr/0002-go-postgres-backend.md)

## Next-agent instructions

1. Read ADR 0002, this plan, [`CONTRIBUTING.md`](../../CONTRIBUTING.md), and [`tui/DESIGN.md`](../../tui/DESIGN.md).
2. P1–P4 are settled; do not reopen them. Do not start a task while a decision it depends on is still Proposed.
3. Mark a task complete only after its tests and gates pass, and record the commit and evidence in the execution log.
4. Tests never touch a production Neon database, a real `~/.taskrc`, or `~/.task`. Use disposable PostgreSQL and isolated Taskwarrior profiles.
5. Commit as [`AGENTS.md`](../../AGENTS.md) describes. Do not push, deploy, or create cloud resources without instruction.

## Current state (verified at planning time)

| Area | Source | Implication |
|---|---|---|
| App-to-storage interface | `tui/internal/taskwarrior/client.go`: `Client` (`ExportPending`, `Add`, `Modify`, `Complete`, `Delete`, `Start`, `Stop`, `Undo`, `Sync`, `Projects`); `tui/internal/bridge/server.go`: bridge `Client` adds `Version`, `ExportCompleted`, `Context`, `StopRecurrence`, `SyncConfigured`, `EstimateUDAReadiness`; `tui/internal/app` type-asserts optional readers (recurring tasks, tags, project migration) | A backend client can implement the same interfaces, so the TUI and the bridge can switch storage in config. |
| Task model | `tui/internal/domain/task.go`: `Task` with status, dates, tags, annotations, dependencies, recurrence parent and mask, urgency, and `RawFields` | Schema columns for the fields Momentum uses; a `jsonb` column keeps every other imported field. |
| Shared rules | `tui/internal/domain` imports no other internal package; it is imported by `app`, `bridge`, `config`, `quickadd`, `taskwarrior`, and `ui` | Moving it to `core/domain` changes import paths only. |
| Dates | `NewTask` and `TaskDiff` carry Taskwarrior date expressions as strings; Taskwarrior resolves and validates them (DESIGN Quick add: "Any Taskwarrior-compatible date expression may be submitted even if it was not suggested"; ADD-06) | The backend needs absolute instants, so something must resolve expressions (D3). |
| Urgency | Read from `task export`; sort key for Inbox, Today, and the planner (`domain/planner.go:154`, DESIGN VIEW-05/06) | Urgency must be computed by Momentum (D4). |
| Recurrence | `domain.ParseRecurrence` validates rules; Taskwarrior generates instances from a template (`status:recurring`, `parent`, `imask`) | The backend generates instances itself (D5). |
| Undo and sync | `tui/internal/app/sync_state.go` and `macos/Momentum/Model/SyncState.swift`: an undo grace period, then `task sync`; `u` runs `task undo` | Backend mode replaces undo with server history (D7) and skips the sync state machine (P1). |
| Contexts and project migration | `tui/internal/taskwarrior/project_migration.go`: active-context filter and guarded single-task commands (ADR 0001) | In backend mode, one transaction; contexts are D6. |
| Capture | `tui/internal/quickadd` runs in the TUI and in the bridge | Stays on the apps' side; the backend receives structured fields. |
| Hosting | `tui/docs/sync.md`: TaskChampion server on Cloud Run with Neon | Keep it running until cutover (T19). |

## Target architecture

```text
TUI (Go) ────────────────────┐
                             ├── HTTPS + JSON /v1 ──▶ momentum-server (Go, Cloud Run) ──▶ Neon PostgreSQL
macOS app ──▶ bridge (Go) ───┘
              both use core/: domain rules, API types, API client
```

```text
go.work          joins core, tui, backend
core/            module …/core: domain (moved from tui/internal/domain), api (wire types and client)
tui/             TUI and bridge; the Taskwarrior adapter stays until T19
backend/         module …/backend: cmd/momentum-server, internal/{httpapi,auth,store,recur}, migrations/
macos/           unchanged apart from sign-in
```

Storage mode is chosen in the apps' config: a `[backend]` section with a `url` selects the backend; without it, Momentum uses Taskwarrior as today.

## Decisions

Approved by the user on 2026-10-14. D9 was revised from the recommendation; the others were approved as recommended.

| ID | Decision | Approved choice | Alternatives not selected | Status |
|---|---|---|---|---|
| D1 | Identity provider and sign-in flow | The backend runs OpenID Connect sign-in with Google and GitHub. Both apps use the device authorization flow (RFC 8628): the app shows a short code and opens the browser. The backend issues one revocable, opaque token per device and stores only its hash. | A managed service (Neon Auth, Clerk, WorkOS, Auth0); a native macOS browser sheet instead of device codes | Approved 2026-10-14 |
| D2 | API style | JSON over HTTPS under `/v1`, standard-library `net/http` routing, request and response types in `core/api` shared by server and client, no code generation | Connect/gRPC with protobuf; OpenAPI-first with generated code | Approved 2026-10-14 |
| D3 | Date expressions | The apps resolve expressions to absolute RFC 3339 instants in the device's time zone before sending. Supported: what Momentum suggests today (`today`, `tomorrow`, weekday names, `next-week`, ISO dates, the editor's `YYYY-MM-DD HH:MM`, natural-language capture) plus an inventory of documented Taskwarrior named dates taken in T03. Anything else is rejected with a message instead of passed through. | The server parses Taskwarrior expressions, which requires each user's time zone on the server | Approved 2026-10-14 |
| D4 | Urgency | Port Taskwarrior's default urgency formula to `core/domain`, so Inbox and Today keep their order, and check it against Taskwarrior's exported urgency | A simpler explicit order (due, priority, age) | Approved 2026-10-14 |
| D5 | Recurrence behavior | A template with one open instance at a time. The next instance is created when the current one is completed or deleted, anchored to the template's due date in the user's time zone. Missed periods are not back-filled. | Copy Taskwarrior's catch-up generation, which creates every missed instance | Approved 2026-10-14 |
| D6 | Contexts and hooks | Not supported in backend mode. Inbox shows all of the user's pending tasks, and project migration covers all pending tasks. | A saved server-side filter that acts as a context | Approved 2026-10-14 |
| D7 | Undo | Every mutation writes its before and after state to a history table in the same transaction. Undo reverts the device's latest mutation if the task has not changed since, offered for `mutation_delay` as today. | Multi-step undo history | Approved 2026-10-14 |
| D8 | Hosting | Cloud Run with minimum 0 instances, and a new Neon project separate from TaskChampion's. The app connects through Neon's pooled endpoint and migrations use the direct one (T01 verifies pgx with the pooler). Neon point-in-time restore for backups; secrets in Secret Manager. | Minimum 1 instance to avoid cold starts, at a monthly cost | Approved 2026-10-14 |
| D9 | Libraries and tests | `ent` for the schema and a generated, type-checked client, with its privacy rules limiting every task query to the signed-in user; Atlas for versioned migration files that are reviewed before applying; `pgx/v5` as the driver through its `database/sql` adapter; `coreos/go-oidc` with `golang.org/x/oauth2` for sign-in. Tests run against disposable PostgreSQL (Docker locally, a service container in CI) behind a guard that refuses non-local connection strings. | `pgx` + `sqlc` + `goose` (the planning recommendation); GORM; Neon branches for tests | Approved 2026-10-14 (revised) |
| D10 | Who can sign up | An allowlist of email addresses configured on the server; nobody else can create an account until sign-up is deliberately opened | Open sign-up; invitations | Approved 2026-10-14 |

## Consequences already settled

| Source | Consequence |
|---|---|
| P1 | No local task store, sync engine, or conflict handling. When the backend is unreachable, the apps say so and offer a retry; nothing is queued. |
| P1 | In backend mode, the sync state machine, the sync countdown, and the unsynced-changes quit prompt are skipped in both apps. |
| P2 | Every query and mutation is scoped to the signed-in user, and isolation is tested across two users. |
| P3 | Task contents are stored as plain columns and never written to logs, errors, or traces. |
| P4 | The Taskwarrior adapter and its tests stay working until T19; both storage modes pass the gates in between. |
| P4 | The import keeps UUIDs, entry, modified, and end timestamps, annotations, dependencies, and every unrecognized field, so a re-export can be compared with the original. |
| Code | The macOS app keeps using the bridge, so Swift changes are limited to sign-in and backend-mode states. |
| Code | `quickadd` stays in the apps for keystroke-speed interpretation; the backend validates structured fields with `core/domain`. |

## Tracker

| Status | ID | Task | Depends on | Verification |
|---|---|---|---|---|
| [x] | T00 | Approve D1–D10 | — | All approved 2026-10-14; D9 revised to `ent` with Atlas |
| [ ] | T01 | Feasibility spike: Neon, Cloud Run, sign-in | T00 | Evidence recorded below; spike code not merged |
| [x] | T02 | Create `core/` and `go.work`; move `domain` | — | Every existing Go test passes unchanged; CI runs `core` and `tui` |
| [ ] | T03 | Date resolution in `core` | T00 (D3), T02 | Table tests with an injected clock and zone, DST cases, comparison with Taskwarrior |
| [ ] | T04 | Urgency in `core` | T00 (D4), T02 | Matches Taskwarrior's exported urgency on fixture tasks |
| [ ] | T05 | API contract in `core/api` | T00 (D2), T02 | JSON round-trip tests for every type and the error format |
| [ ] | T06 | Backend skeleton | T05 | Health check, config, shutdown, and log redaction tests; CI job |
| [ ] | T07 | Schema, migrations, and store | T06, T00 (D9) | Migrations apply to an empty database and match the `ent` schema; privacy-rule and store tests on disposable PostgreSQL |
| [ ] | T08 | Sign-in, sessions, and per-user access | T07, T01 | Device-flow, token-revocation, allowlist, and cross-user isolation tests |
| [ ] | T09 | Task API | T03, T07, T08 | Handler and store tests per endpoint, including idempotent retries |
| [ ] | T10 | Undo | T09, T00 (D7) | Undo and conflict tests |
| [ ] | T11 | Recurrence | T09, T00 (D5) | Generation, stop-series, and DST tests |
| [ ] | T12 | Project migration endpoint | T09 | Preview and apply in one transaction; rollback test |
| [ ] | T13 | Backend client and storage-mode switch | T05, T09 | Client tests against a test server; both modes pass app and bridge tests |
| [ ] | T14 | TUI sign-in and backend mode | T08, T13 | `momentum login`/`logout`, token file mode `0600`, unreachable-backend states |
| [ ] | T15 | macOS sign-in and backend mode | T08, T13 | Swift tests for sign-in and backend-mode states |
| [ ] | T16 | Taskwarrior import | T09, T11 | Dry-run report; import, re-export, and compare on an isolated profile; re-run is a no-op |
| [ ] | T17 | Deployment | T01, T08 | Deployed service passes health and sign-in checks; deploy guide written |
| [ ] | T18 | Parity check with both storage modes | T14, T15, T16, T17 | DESIGN requirement checklist per mode; gaps recorded |
| [ ] | T19 | Cutover and Taskwarrior removal | T18 | Backend is the default; Taskwarrior adapter, sync docs, and TaskChampion retirement each approved separately |

T02 does not depend on any decision and can start now.

## Task details

### T01 — Feasibility spike

- Connect with `ent` over `pgx/v5` through Neon's pooled endpoint and the direct endpoint; record whether prepared statements work through the pooler.
- Measure the first-request latency after idle with Cloud Run and Neon both scaled to zero, then with each warm.
- Run the device authorization flow end to end with the chosen provider (D1), from a terminal and from the macOS app.
- Record the results under "T01 feasibility record" below. If a result contradicts a decision, report it instead of changing the decision.

### T02 — Shared core module

- Create `core/` as module `github.com/jvrviegas/momentum-task-manager/core` and a root `go.work` with `core`, `tui`, and later `backend`.
- Move `tui/internal/domain` to `core/domain` with `git mv`, then rewrite its imports. No behavior change.
- Update CI to run the gates for every module, and update `CONTRIBUTING.md` and `macos/scripts/build-bridge.sh` if paths change.
- Done 2026-10-14: `tui/go.mod` requires `core` through `replace … => ../core`, because `go mod tidy` ignores `go.work` and `core` is unpublished. `go install …@version` rejects a `replace`, so the first TUI release that includes `core/` must publish `core` or change the install instructions (see `CONTRIBUTING.md`, Release). `build-bridge.sh` needed no change; it builds from `tui/`, where the replace applies.

### T03 — Date resolution

- Inventory every expression Momentum suggests, documents, or produces (edit suggestions, capture triggers, natural-language capture, `next-week`).
- Resolve them to absolute instants with an injected clock and time zone; reject anything outside the list with a message the apps can show.
- Compare each supported expression with Taskwarrior's result in an isolated profile, across a DST change.

### T04 — Urgency

- Port Taskwarrior's default urgency coefficients and terms for the fields Momentum stores.
- Build fixtures in an isolated Taskwarrior profile and assert equality with the exported `urgency` values.

### T05 — API contract

- Define request and response types for every operation in the interfaces above, a single error shape with field-level messages (matching the bridge's field errors), and `/v1` versioning.
- Mutations carry a client-generated idempotency key so retries after a timeout are safe.

### T06 — Backend skeleton

- `backend/cmd/momentum-server`: configuration from environment variables, `/healthz`, structured logs that never include task contents or tokens, graceful shutdown.
- Container image and a CI job with a PostgreSQL service container.

### T07 — Schema, migrations, and store

- Tables: `users` (including the time zone), `identities`, `sessions`, `device_authorizations`, `tasks` (with an `extra jsonb` column for imported fields), and `task_events` for history and undo.
- `ent` schema with privacy rules that limit every task query and mutation to the signed-in user, and Atlas migration files generated from it and reviewed.
- A test guard that refuses any connection string that is not a local disposable database.

### T08 — Sign-in, sessions, and per-user access

- OpenID Connect sign-in and the device authorization flow (D1), the email allowlist (D10), one hashed token per device, revocation, and expiry.
- Middleware resolves the token to a user. Tests prove one user cannot read or change another user's tasks.
- Run the security review before T17.

### T09 — Task API

- Endpoints: pending tasks, recurring templates, completed since a time, projects, tags, create, modify (with `TaskDiff` semantics), complete, delete, start, stop.
- Each mutation validates with `core/domain` and writes its `task_events` row in the same transaction.

### T10 — Undo

- Revert the device's latest mutation when the task has not changed since; otherwise explain why undo is unavailable.

### T11 — Recurrence

- Templates and instances per D5, stop-series (today's `until:today` behavior), and DST-correct anchors in the user's time zone.

### T12 — Project migration

- A preview that returns counts, and an apply step in one transaction that keeps ADR 0001's rules other than contexts (D6).

### T13 — Backend client and storage-mode switch

- A client in `core/api` that implements the TUI's and the bridge's interfaces.
- The `[backend]` config section selects it; `momentum doctor` checks that the backend is reachable and the token is valid.

### T14 — TUI

- `momentum login` (device flow) and `momentum logout`; the token is stored in a mode `0600` file under the XDG config directory.
- In backend mode: skip the sync state machine, keep the undo toast, and show an unreachable-backend error with retry.

### T15 — macOS

- A sign-in sheet that shows the device code and opens the browser; the token is kept in the Keychain and passed to the bridge.
- Backend-mode states mirror T14.

### T16 — Taskwarrior import

- `momentum import` reads `task export` for every status, including recurring templates, and uploads it with UUIDs preserved.
- A dry run reports counts and any fields kept only in `extra`. Re-running the import changes nothing. Taskwarrior data is never modified.

### T17 — Deployment

- Cloud Run service, Neon project, Secret Manager secrets, and a migration step that uses the direct endpoint.
- A deploy guide in `backend/` alongside the existing `tui/docs/sync.md`. No resources are created without the user's go-ahead.

### T18 — Parity check

- Walk through the DESIGN requirements (VIEW, ROW, ADD, MUT, SEARCH, and the planner) in both modes with imported data, and record any differences.

### T19 — Cutover

- Make the backend the default. Removing the Taskwarrior adapter, the sync documentation, and the TaskChampion service each needs separate approval.

## Risks

| Risk | Mitigation |
|---|---|
| Behavior Taskwarrior provided silently changes (dates, urgency, recurrence) | T03, T04, and T11 compare with Taskwarrior in isolated profiles; T18 checks both modes side by side |
| Slow first request after idle (P1 with scale-to-zero hosting) | Measured in T01; D8 lists the paid alternative |
| Sign-in or session flaws | Maintained OIDC libraries, hashed tokens, short-lived device codes, rate limits, and a security review before T17 |
| Data loss during import | Dry run, idempotent import, Taskwarrior data left untouched until T19, Neon point-in-time restore |
| Scope growth from accounts | Sharing, teams, a web app, and account self-service beyond sign-in and sign-out are out of scope |

## Out of scope

Offline use (P1), sharing tasks between users, a web or mobile app, end-to-end encryption (P3), and Taskwarrior contexts and hooks in backend mode (D6).

## T01 feasibility record

Not started.

## Execution log

| Date | Task | Commit | Evidence |
|---|---|---|---|
| 2026-10-14 | T00 | — | D1–D10 approved in conversation; D9 revised from `pgx` + `sqlc` + `goose` to `ent` with Atlas. |
| 2026-10-14 | T02 | `71db93a` | `git mv tui/internal/domain core/domain`; 78 files changed one import line each. `go test -count=1 -json` passed the same 754 tests and subtests before and after (Taskwarrior 3.5.0 integration tests included). gofmt, race tests, and vet are clean in `core` and `tui`; the four `cmd/momentum` cross-builds, the bridge build, and `GOWORK=off` builds pass; `go mod tidy` with `GOPROXY=off` leaves `go.mod` and `go.sum` unchanged. The CI matrix (`core`, `tui`) has not run on GitHub yet. |
