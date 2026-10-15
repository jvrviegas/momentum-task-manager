# Implementation plan — Go backend replacing Taskwarrior

**Created:** 2026-10-14  
**Status:** P1–P4 and D1–D10 are approved (2026-10-14). T02–T09 are complete. T08 was built before T01, at the user's request (2026-10-14). T01 still needs the user's cloud accounts, and T17 waits on it.  
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
backend/         module …/backend: cmd/{momentum-server,migration-diff}, internal/{auth,config,httpapi,logging,store,tasks}, migrations/;
                 internal/recur (T11) to come
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
| [x] | T03 | Date resolution in `core` | T00 (D3), T02 | Table tests with an injected clock and zone, DST cases, comparison with Taskwarrior |
| [x] | T04 | Urgency in `core` | T00 (D4), T02 | Matches Taskwarrior's exported urgency on fixture tasks |
| [x] | T05 | API contract in `core/api` | T00 (D2), T02 | JSON round-trip tests for every type and the error format |
| [x] | T06 | Backend skeleton | T05 | Health check, config, shutdown, and log redaction tests; CI job |
| [x] | T07 | Schema, migrations, and store | T06, T00 (D9) | Migrations apply to an empty database and match the `ent` schema; privacy-rule and store tests on disposable PostgreSQL |
| [x] | T08 | Sign-in, sessions, and per-user access | T07, T01 | Device-flow, token-revocation, allowlist, and cross-user isolation tests |
| [x] | T09 | Task API | T03, T07, T08 | Handler and store tests per endpoint, including idempotent retries |
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

Next: T10, T11, T12, and T13 can start. T01 needs the user's go-ahead and access to Neon, a Google Cloud project, and the Google and GitHub OAuth apps; T17 waits on it, and it still owes T08 an end-to-end check with the real providers.

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
- Done 2026-10-14: `domain.ResolveDate(expression, now)` in `core/domain/dates.go`, where `now` carries both the clock and the zone. Nothing calls it yet; T13 wires it into the backend client.
- Inventory of what Momentum suggests or sends:
  - Capture suggestions (`quickadd/suggest.go`): `today`, `tomorrow`, the next seven weekday names, `next-week`, and the ISO date a week ahead.
  - TUI edit form (`ui/edit.go`): `today`, `tomorrow`, `monday`–`friday`, `next-week`, today's ISO date. The field shows and steps `YYYY-MM-DD HH:MM`.
  - macOS edit field (`EditTaskView.swift`): `today`, `tomorrow`, `monday`, `friday`, `next-week`, and the picker's `YYYY-MM-DD HH:MM`.
  - Natural-language capture (`quickadd/interpret.go`) resolves its own phrases and sends local `YYYYMMDDTHHMMSS`. `^weekday` sends the weekday name, and a recurrence without a due date sends `today` (`quickadd/parser.go`).
  - Stop-series sends `until:today` (`taskwarrior/mutations.go`). In backend mode the server sets it (T11), so the apps do not resolve it.
- Supported, per D3 and the user's choices on 2026-10-14. Names ignore case.
  - Momentum's forms: `today`, `tomorrow`, weekday names, `next-week` (Friday of the next Monday-to-Sunday week), `YYYY-MM-DD`, `YYYY-MM-DD HH:MM`, `YYYY-MM-DDTHH:MM[:SS]`, `YYYYMMDDTHHMMSS`, and the ISO forms with `Z` or an offset.
  - Taskwarrior's documented named dates: `now`, `today`/`sod`, `eod`, `yesterday`, `tomorrow`, weekday and month names (or any prefix of three or more letters), `later`/`someday`, the start and end of the current, previous, and next week, month, quarter, and year (`sow`, `eopm`, `sonq`, …), the previous and next work week (`sopww`, `eonww`), and the ordinals `1st`–`31st`.
- Rejected with a message:
  - Holidays (`easter`, `goodfriday`, `eastermonday`, `ascension`, `pentecost`, `midsommar`, `midsommarafton`), by the user's choice. Taskwarrior 3.5's `midsommarafton` for 2027 is June 18, a week early.
  - Prefixes of other names (`tom`, `yes`, `lat`, `some`), by the user's choice.
  - Names that work in Taskwarrior 3.5 but are undocumented: `soww`, `eoww`, `sopd`, `eopd`, `sond`, `eond`, `juhannus`, and informal times such as `9am`.
  - Arithmetic and durations (`eom-1d`, `3d`), epoch seconds, and ISO week or ordinal dates (`2026-W43`, `2026-293`).
- Where the result differs from Taskwarrior 3.5:
  - `next-week` stays on Friday midnight across a DST change. Taskwarrior adds the TUI's `sow+1w+4d` as seconds, so Taskwarrior mode lands on Thursday 23:00 or Friday 01:00 in those weeks. That Taskwarrior-mode bug is unchanged.
  - An ordinal missing from its month is an error, as Taskwarrior's documentation says. For example, `31st` on 31 October, or `30th` in February. Taskwarrior 3.5 rolls it into the next month.
  - A typed time that a DST change skips or repeats is rejected, by the user's choice. This matches capture review. Taskwarrior moves a skipped time forward and picks one occurrence of a repeated time.
  - `Today` resolves; Taskwarrior accepts `Monday` but not `Today`.
- Day boundaries: if a DST change skips midnight, the day starts at the change, as in Taskwarrior. If 23:59:59 happens twice, the end of day is the later one. A day that is skipped entirely (Pacific/Apia, 2011-12-30) is an error.

### T04 — Urgency

- Port Taskwarrior's default urgency coefficients and terms for the fields Momentum stores.
- Build fixtures in an isolated Taskwarrior profile and assert equality with the exported `urgency` values.
- Done 2026-10-14: `domain.SetUrgency(tasks, now)` in `core/domain/urgency.go`. Nothing calls it yet; T09 does.
  - It uses Taskwarrior 3.5's default coefficients, as `task show urgency` lists them. User coefficients from a `.taskrc` do not apply in backend mode, so a user who changed them sees a different order there.
  - Blocking: a dependency counts only when both tasks are in the list and neither is completed or deleted. T09 must pass every unfinished task, including waiting ones. `status:pending` hides waiting tasks, but Taskwarrior still counts them as blockers.
  - Precision: the result is identical to the exported number. Taskwarrior adds float32 terms and prints six significant digits, so `SetUrgency` does the same. Tasks that tie in Taskwarrior mode tie in backend mode too, and the views break the tie the same way. On arm64, Go fuses a multiply into the following add unless the product is converted explicitly. That changed the sixth digit of one recorded value, so the code converts it.
  - Not compared with Taskwarrior: a task without `entry` scores the full age term, as in Taskwarrior's source. Taskwarrior adds `entry` on import, so it cannot be compared.

### T05 — API contract

- Define request and response types for every operation in the interfaces above, a single error shape with field-level messages (matching the bridge's field errors), and `/v1` versioning.
- Mutations carry a client-generated idempotency key so retries after a timeout are safe.
- Done 2026-10-14: `core/api` has the routes as ServeMux patterns, the types, and the `Error` shape. Choices made here, which T09 and T12 implement:
  - Routes: list tasks by `status` (`pending`, `recurring`, or `completed` with `ended_after`), create, `PATCH` to modify, `DELETE`, complete, start, stop, stop-recurrence, undo, projects, tags, version, and project-migration preview and apply.
  - The same `status:pending` meaning as Taskwarrior: pending excludes tasks that are waiting.
  - Modify works like a JSON merge patch: a field left out stays, `null` clears it, and a value sets it. `Change[T]` carries `domain.ChangeKind`. Tags are `add` and `remove` lists, as in `TaskDiff`, so a modify never replaces tags it did not touch.
  - Every task mutation, and undo, answers with the task as it is afterwards.
  - Idempotency uses the `Idempotency-Key` header, not a body field, so one middleware handles every mutation. A reused key with a different request is a conflict.
  - Errors: the body is `{"code","message","field"}`. The codes are `invalid`, `unauthorized`, `not_found`, `conflict`, and `internal`. `field` is the request's JSON field name; the bridge maps `estimateMinutes` back to its `estimate` field.
  - The task leaves out Taskwarrior's numeric `id`, `mask`, `imask`, and `rtype`; T16 keeps imported values in `extra`. Estimates are whole minutes.
  - The project-migration preview returns the task mappings, not only counts. Apply sends them back, and the server refuses with `conflict`, changing nothing, if the mappings no longer match.
  - Sign-in types are left to T08, because T01 can still change D1. `Sync`, `SyncConfigured`, `Context`, `EstimateUDAReadiness`, and `RecurrenceGeneration` have no routes; T13's client answers them locally.

### T06 — Backend skeleton

- `backend/cmd/momentum-server`: configuration from environment variables, `/healthz`, structured logs that never include task contents or tokens, graceful shutdown.
- Container image and a CI job with a PostgreSQL service container.
- Done 2026-10-14, with one departure. The health check is `GET /health`, not `/healthz`, because Cloud Run's known issues reserve "some paths ending with `z`" and recommend avoiding all of them.
  - Config: `PORT` only, defaulting to 8080. Later tasks add their own variables.
  - Logs: JSON with Cloud Logging's `severity` and `message` keys. Only allowlisted attribute keys keep their values (`addr`, `duration_ms`, `method`, `route`, `status`); every other value becomes `[redacted]`. Requests are logged by route pattern, and a panic is logged without its value.
  - Errors: unknown routes answer with an `api.Error`. Each error code maps to one HTTP status in `httpapi`.
  - Shutdown: SIGINT or SIGTERM stops accepting connections and gives requests in flight 8 seconds, inside Cloud Run's 10.
  - Image: a static `CGO_ENABLED=0` binary on `distroless/static-debian12:nonroot`, built from the repository root because of `replace ../core`.
  - CI: a `backend` job runs gofmt, tests, race tests, vet, and the image build next to a `postgres:17-alpine` service. T17 should match that major version to the Neon project's.

### T07 — Schema, migrations, and store

- Tables: `users` (including the time zone), `identities`, `sessions`, `device_authorizations`, `tasks` (with an `extra jsonb` column for imported fields), and `task_events` for history and undo.
- `ent` schema with privacy rules that limit every task query and mutation to the signed-in user, and Atlas migration files generated from it and reviewed.
- A test guard that refuses any connection string that is not a local disposable database.
- Done 2026-10-14. **The first migration, `backend/migrations/20261014175612_init.sql`, waits for the user's review (D9).** It has only been applied to disposable local databases.
  - Tables: the six listed. Users and tasks have UUID ids, so an imported task keeps its Taskwarrior UUID; the other tables use bigint identities. Users, identities, sessions, device authorizations, tasks, and task events reference their user by foreign key.
  - Task columns: `tags`, `depends`, `annotations`, and `extra` are `jsonb`. `status` is a varchar that ent validates, with no CHECK constraint. `parent` (an instance's template) is not a foreign key.
  - Task events: `task_id` is not a foreign key, so history outlives the task. `before` and `after` are `jsonb`, and `session_id` is set to null if its session is deleted.
  - Privacy: Task and TaskEvent use `rule.FilterByViewer`, which limits queries, updates, and deletes to the viewer and denies them without one, and `rule.CreateAsViewer`. The auth tables have no policy, because only the sign-in code (T08) uses them.
  - Migrations: `cmd/migration-diff` runs ent's `NamedDiff` in replay mode on an empty local database and writes Atlas-format files with `atlas.sum`. `store.Migrate` applies the embedded files with Atlas's Go executor, one transaction per file, and records revisions in `momentum_migrations.revisions`, a schema of its own. No Atlas CLI is needed. T17 wires `store.Migrate` to a command that uses Neon's direct endpoint.
  - Tooling: the ent CLI is pinned as a Go tool (`go tool ent`). `golang.org/x/tools` is raised to v0.52.0, because the version ent requires fails on Go 1.27 with `package "context" without types`. CI fails if `go generate` changes the committed code.
  - Tests: `storetest` creates a fresh database per test on the server `MOMENTUM_TEST_DATABASE_URL` names, and drops it afterwards. It fails on any host other than this machine, and skips when the variable is unset.
  - Left for later tasks: T08 may change `sessions` and `device_authorizations` with a new migration. T09 needs a table for idempotency keys. T10 may need to mark an event as undone.

### T08 — Sign-in, sessions, and per-user access

- OpenID Connect sign-in and the device authorization flow (D1), the email allowlist (D10), one hashed token per device, revocation, and expiry.
- Middleware resolves the token to a user. Tests prove one user cannot read or change another user's tasks.
- Run the security review before T17.
- Done 2026-10-14, before T01, at the user's request. T01 still has to run sign-in end to end with the real providers.
  - **Finding for D1:** GitHub's documentation for OAuth apps describes only an access token, with no ID token and no `openid` scope. GitHub therefore signs in through OAuth 2.0: the account is `GET /user`, and the email is the primary, verified one from `GET /user/emails`. Google uses OpenID Connect as D1 says. D1 is unchanged; this changes only how GitHub is implemented.
  - Device flow: `POST /v1/auth/device` returns an eight-letter code (no vowels) and a device code that lasts 10 minutes. The person enters the code at `/device`, picks a provider, and comes back to `/auth/callback/{provider}`. The app polls `POST /v1/auth/token` every 5 seconds and gets RFC 8628's `authorization_pending`, `slow_down`, `access_denied`, or `expired_token` until it receives the token, once.
  - Tokens: `mtk_` plus 32 random bytes. Sessions store only the SHA-256 and last 90 days with no renewal. `POST /v1/auth/sign-out` revokes the current one. Middleware turns `Authorization: Bearer` into the user (`viewer`) and the session (`auth.FromContext`, for T10's per-device undo), or answers 401 with `WWW-Authenticate`.
  - Allowlist (D10): `ALLOWED_EMAILS` is checked at every sign-in, not only the first. An account outside it is refused, and no user is created. A new identity joins the user with the same verified address.
  - Browser protections: PKCE (S256) and, for Google, a nonce; a state cookie that ties the callback to the browser that started it, valid once; a CSRF cookie on the `/device` form; and `prompt=select_account`. The page warns the person to continue only for a code from their own device, against RFC 8628's remote phishing. Pages send CSP `frame-ancestors 'none'`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, and `Cache-Control: no-store`.
  - Logs: a failure logs only `error_type`, the Go type of the innermost error.
  - Configuration: `DATABASE_URL` is now required, and `PUBLIC_URL`, `ALLOWED_EMAILS`, and the provider credentials are added. `momentum-server migrate` applies migrations; Compose runs it before the server, and T17 can run it with Neon's direct endpoint.
  - Schema: migration `20261014214921_device_sign_in.sql` adds the sign-in state to `device_authorizations`. It waits for review, like the first.
  - For the security review: there are no rate limits on the unauthenticated routes yet (starting sign-ins, polling, the form). On Cloud Run they need the client address from the right `X-Forwarded-For` entry. Sessions cannot be listed or revoked from another device.

### T09 — Task API

- Endpoints: pending tasks, recurring templates, completed since a time, projects, tags, create, modify (with `TaskDiff` semantics), complete, delete, start, stop.
- Each mutation validates with `core/domain` and writes its `task_events` row in the same transaction.
- Done 2026-10-15. **Migration `20261015213017_idempotency_keys.sql` waits for the user's review (D9),** like the first two. It has only been applied to disposable local databases.
  - Package: `internal/tasks` reads and changes tasks; `httpapi` decodes requests and maps errors. Create answers 201 and the other mutations 200, each with the task as it is afterwards. Version, undo, stop-recurrence, and project migration are left to T13, T10, T11, and T12.
  - Idempotency: the header is required, 1 to 255 characters. The `idempotency_keys` table, unique per user and key, holds a SHA-256 of the action, the task, and the decoded request, and the response. A mutation claims its key first, in its own transaction, so a retry sent while the first attempt runs waits for it, then gets its response. A failed mutation claims nothing. Keys last 24 hours and are pruned at the user's next mutation.
  - Locking and history: each mutation locks its task with `SELECT … FOR UPDATE` (ent's `sql/lock` feature), so concurrent changes apply one after another. It writes a `task_events` row with the session and the task before and after, as `api.Task` JSON with urgency zero; before is absent for a create.
  - Rules: only pending tasks change; anything else is `conflict`, as are starting a started task and stopping one that is not. Taskwarrior 3.5 lets you change completed tasks, but both apps treat them as read-only. In an isolated 3.5.0 profile, `done` set `end` and cleared `start`, `delete` set `end` and kept `start`, and starting a waiting task kept its wait date; the backend does the same.
  - Validation follows the bridge's editor: description not blank; priority `H`, `M`, `L`, or empty; estimate 1 to 1440 minutes when set, while an imported larger estimate stays if a modify leaves it out; tags not empty and without whitespace; and a modify must change something. Tags keep their order without repeats, and a modify removes before it adds.
  - Recurrence: `recurrence` on create or modify is `invalid` until T11 generates instances.
  - Times are stored in UTC to the second, as Taskwarrior keeps them.
  - Urgency: reads and responses compute it with `domain.SetUrgency` over every pending task, waiting ones included, as T04 requires. Projects and tags come from pending tasks that are not waiting, sorted.
  - JSON type errors name the top-level field when `encoding/json` reports one. It reports none for `api.Change` fields or times, which get a general message.

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
| 2026-10-14 | T03 | `b01b50f`, `b8f15f9`, `4d6dbb3` | The `core/domain` table tests use `task calc` output from Taskwarrior 3.5.0 at 2026-10-14 17:08:27 Europe/Lisbon as expected values. They also cover DST changes in Lisbon and Santiago (a skipped midnight, a repeated hour, and the spring and autumn changes), month, year, and ordinal edges, and every rejected form. `TestIntegrationResolveDateMatchesTaskwarrior` adds 84 expressions through the Taskwarrior adapter in an isolated profile, in Europe/Lisbon and in two POSIX-rule zones whose DST ends or starts three days after the run date. Every result equals `ResolveDate` except `next-week`, which equals Taskwarrior's seconds arithmetic as described in T03. Two deliberate breaks each failed it: removing that `next-week` allowance, and resolving today's weekday name to today. gofmt, tests, race tests, and vet pass in `core` and `tui`, and `cmd/momentum` builds with and without `go.work`. The comparison skips where `task` is not installed, as in CI. |
| 2026-10-14 | T04 | `f4eba68`, `48cd185`, `2ab1f83` | `TestSetUrgencyMatchesTaskwarrior` has 48 fixtures, imported into an isolated Taskwarrior 3.5.0 profile with offsets from one second and exported in that second (2026-10-14 17:37:29 UTC). They cover every term, the 14-day and 7-day due limits, `scheduled` equal to now, blocking chains, blockers that are waiting, completed, or deleted, age limits, and six due dates where float64 sums or fused products change the sixth digit. `TestIntegrationSetUrgencyMatchesTaskwarrior` checks 35 fixtures live through `task import` and `task export`, at every second the export could have used. Deliberate breaks each failed a test: `scheduled` at or before now, no rounding, a fused or float64 product, float64 day counts, age rounded up, finished dependencies blocking, and waiting tasks not blocking. gofmt, tests, race tests, and vet pass in `core` and `tui`, and `cmd/momentum` builds with and without `go.work`. |
| 2026-10-14 | T05 | `fe63472`–`a053780` (7 commits) | `core/api` tests encode every type to golden JSON and decode it back to an equal value: a fully populated and a minimal task, task list, create, modify (set, clear, set to empty, unchanged), projects, tags, project migration and its apply request, result, version, and errors with and without a field. `TestRoutesAreDistinctV1Patterns` registers every route in one ServeMux, which panics on a bad or conflicting pattern. Deliberate breaks each failed a test: dropping `omitzero`, decoding `null` as a set, and a wildcard other than `{uuid}`. gofmt, tests, race tests, and vet pass in `core` and `tui`. |
| 2026-10-14 | T06 | `3d245fe`–`06bc383` (16 commits) | `backend/internal` tests cover the config (default, valid, invalid `PORT`), logging (Cloud Logging fields, `WARNING`, redaction of task text, a token, an email, an error, and a nested group), the handler (`/health`; 404 errors for `/healthz`, a wrong method, and unknown paths; a request log without path, query, `Authorization`, or body; a panic answering 500 without logging its value), and shutdown (finishes a request in flight while refusing new connections; gives up after the drain time). Deliberate breaks each failed a test: no allowlist, logging the URL, and `Close` instead of `Shutdown`. Logging a panic value under an unlisted key stayed hidden, because the allowlist redacted it. The image built locally (11.6 MB) and its container answered `/health`, logged a request with a token and query string without either, and exited 0 on SIGTERM. gofmt, tests, race tests, and vet pass in `core`, `tui`, and `backend`; `GOWORK=off` builds pass and `go mod tidy` leaves `backend/go.mod` unchanged. The CI job has not run on GitHub yet. |
| 2026-10-14 | T07 | `e39c006`–`d62ca21` (80 commits; 55 are generated ent files) | On PostgreSQL 17.11 in Docker: `TestMigrationsBuildTheEntSchemaOnAnEmptyDatabase` applies the migrations to an empty database, finds nothing left between it and the ent schema (`ErrNoPlan`, drops included), sees every file recorded, runs `Migrate` again with no change, and sees the comparison report a dropped column. `TestTaskKeepsEveryField` round-trips every task column. `TestUsersCannotReachEachOthersTasks` covers listing, get, reach through the user edge, update, bulk update, delete, bulk delete, and create for another user, for tasks and task events. `TestTasksNeedASignedInUser` checks that queries, updates, deletes, and creates are denied without a user. `TestCheckLocalAcceptsOnlyThisMachine` covers Neon direct and pooled URLs, other hosts, and a remote fallback host. Deliberate breaks each failed a test: no user filter, no create check, allowing a missing user, a guard that accepts any host, and an edited migration (checksum mismatch). Without the variable the store tests skip, and with a Neon URL they fail before connecting. `go generate` gives the same output twice. gofmt, tests, race tests, and vet pass in `core`, `tui`, and `backend`; `GOWORK=off` builds and the image build pass; no test database is left behind. |
| 2026-10-14 | T08 | `672d162`–`e7e64bf` (36 commits; 9 regenerate ent files) | `internal/auth` tests on PostgreSQL 17 cover the whole device flow with an injected clock (pending, sign-in, token, hash-only storage, issued once), `slow_down`, allowlist denial with no user created, unknown and expired codes, forged and cross-provider state, state used once, identities joined by email, and revoked, expired, and malformed tokens. Provider tests run against a fake OpenID Connect issuer, which signs RS256 ID tokens and checks PKCE, and a fake GitHub: verified account, another sign-in's nonce, unverified email, wrong audience, expired token, missing PKCE verifier, and GitHub's primary-verified email rule. `internal/httpapi` tests cover the flow over HTTP with cookies (security headers, sign-out then 401), the CSRF cookie, the callback from another browser and replayed, cancelled, unknown-code, and unlisted-account pages, poll validation, bearer header variants, and a test showing two tokens each reaching only their own user's tasks. The log of the full flow contains no token, device code, user code, or email. Deliberate breaks each failed a test: no CSRF check, no state cookie check, no allowlist, revocation ignored, no viewer, reusable state, no `slow_down`, and no `Referrer-Policy`. In Compose, `migrate` applied both migrations, the server started, `POST /v1/auth/device` and `/device` answered, polling gave `authorization_pending`, and no code reached the logs. gofmt, tests, race tests, and vet pass in `core`, `tui`, and `backend`; generated code is current, and `GOWORK=off` builds and the image build pass. |
| 2026-10-15 | T09 | `d7b3ba5`–`9ba835b` (44 commits; 28 are generated ent files) | `internal/tasks` tests on PostgreSQL 17 cover create with every field and its event (session, after state, and urgency 36.119 from its terms), each refused field on create and modify, a modify of every field kind that keeps fields left out (an over-limit estimate included), start, stop, complete, and delete as Taskwarrior 3.5 does them, refusals for finished tasks, and not found for unknown and other users' tasks. They also cover retries (the first answer after the task changed, conflict for another request or route, keys per user, failed requests claiming nothing, expiry after 24 hours, key length), eight concurrent retries creating one task, eight concurrent tag additions all kept, listing by status with a waiting blocker (urgency −5) and an exclusive `ended_after`, and projects and tags. `internal/httpapi` tests cover every route over HTTP with a signed-in token: statuses, retried create and modify, a missing key on every mutation route, field errors, malformed JSON, unknown and other users' tasks, 401 without a valid token, and a log without task text, ids, or keys. Deliberate breaks each failed a test: no row lock, no replay, no fingerprint check, keys that never expire, complete keeping `start`, delete clearing it, changing finished tasks, no session on events, waiting tasks not blocking, unnormalized times, repeated tags, start without sign-in, create answering 200, and handlers ignoring the key. The server, built from source against a throwaway local database, ran `migrate` and answered every route with curl. Four mutations wrote four events and four keys, while a retry, a reused key, and a refused delete wrote none, and the log held route patterns only. gofmt, tests, race tests, and vet pass in `core`, `tui`, and `backend`; generated code is current, `GOWORK=off` builds pass, `go mod tidy` leaves `go.mod` unchanged, and the image builds. |
