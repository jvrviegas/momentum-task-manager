# ADR 0002 — Go and PostgreSQL backend to replace Taskwarrior

- **Date:** 2026-10-14
- **Status:** Accepted: backend language, storage, and product policies P1–P4. Technical choices D1–D10 were approved on 2026-10-14 and are recorded in the [implementation plan](../plans/backend.md#decisions). Implemented: since 2026-10-19 (T19) the backend is the apps' only storage, and Taskwarrior is used only by `momentum import`.
- **Source:** User conversation following the monorepo reorganization (`99bc675`)
- **Delivery:** [Backend implementation plan](../plans/backend.md)
- **Numbering:** ADR numbers are shared across the repository. [ADR 0001](../../tui/docs/adr/0001-project-settings-and-pending-task-renames.md) predates the monorepo layout and lives with the TUI.

## Context

Momentum is a stateless frontend over Taskwarrior 3.x. Taskwarrior owns persistence, recurrence generation, urgency, date expressions, contexts, hooks, undo, and synchronization. Sync runs through a TaskChampion server on Cloud Run backed by Neon PostgreSQL, end-to-end encrypted (`tui/docs/sync.md`, removed in T19; see its history). There are two apps: the Go TUI and the SwiftUI macOS app, which reaches the same Go code through `momentum-bridge`.

The user wants Momentum to own its task storage: a backend with PostgreSQL on Neon instead of Taskwarrior. The choice was between NestJS (TypeScript) and Go.

Forces:

- The task rules are already Go: `domain` (task model, view classification, sorting, edit diffs, recurrence and estimate parsing) and `quickadd` (capture parser, natural-language interpreter, review rules), about 5,000 lines including tests.
- Both apps need Go locally whatever the backend language: capture highlighting runs the interpreter on every keystroke, which cannot wait on a network round trip.
- Cloud Run and Neon are already in use for the TaskChampion server.
- One maintainer works across the repository.

## Decision

1. Build the backend in Go as its own module in `backend/`, storing data in PostgreSQL on Neon.
2. Move the Go code that both the backend and the apps need into a new `core/` module, joined to `tui/` and `backend/` by a root `go.work`. The server enforces task rules with the same code the apps use. `quickadd` stays with the apps.
3. The macOS app keeps talking to `momentum-bridge`; the bridge talks to the backend. Swift needs new sign-in screens, not a new API client.
4. Product policies, chosen by the user on 2026-10-14:

| ID | Policy |
|---|---|
| P1 | **Always online.** No offline mode, now or later. The apps show a clear error when the backend is unreachable. There is no local task store, sync engine, or conflict handling. |
| P2 | **Accounts with sign-in.** Several users, each seeing only their own data, signing in through an identity provider. Provider, sign-in flow, and who may sign up are D1 and D10 in the plan. |
| P3 | **Readable by the server.** Tasks are ordinary rows in Neon, protected by TLS and Neon's encryption at rest. The server can generate recurrences, sort, filter, and validate. The end-to-end encryption TaskChampion provides today is given up. |
| P4 | **Import, then run both for a while.** A one-off import copies `task export` data into PostgreSQL. Until the backend matches what Momentum needs from Taskwarrior, the TUI and the bridge use either Taskwarrior or the backend, chosen in config. Taskwarrior support is removed after parity is confirmed. |

5. Once backend mode ships, this supersedes the [DESIGN](../../tui/DESIGN.md) premise that Taskwarrior is the only source of truth and the §3 non-goal "Replacing Taskwarrior's storage or synchronization protocol". Until then, the shipped TUI keeps its Taskwarrior behavior.

## Options considered

| Dimension | Go | NestJS (TypeScript) | Keep Taskwarrior + TaskChampion |
|---|---|---|---|
| Reuse of existing rules | Direct, through `core/` | Port about 5,000 lines to TypeScript, or let the server trust the apps | Not applicable; Taskwarrior owns the rules |
| Languages in the repository | Go, Swift | Go, Swift, TypeScript | Go, Swift |
| Cloud Run fit | One static binary; fast cold start, low memory | Node runtime; slower cold start, more memory | Already running |
| Built-in structure | Standard library plus small libraries; more wiring by hand | Dependency injection, modules, validation, OpenAPI | Not applicable |
| Owns storage, undo, and sync | Yes | Yes | No |

**Go.** Selected. One set of rules for the apps and the server, no third language, and a small service that suits scale-to-zero hosting. The cost is wiring that NestJS provides out of the box: routing, validation, and auth middleware are written with the standard library and a few focused libraries.

**NestJS.** Not selected. It would be the better choice if a TypeScript web app were next (the `momentum-desktop` React prototype could become one) and could share types with the server. Today it would duplicate the Go rules or leave them only in the apps.

**Keep Taskwarrior.** Not selected. It does not give Momentum ownership of its storage.

Policy alternatives not selected: online first with offline added later, offline from the start, a single user with device tokens, end-to-end encryption with the server as a relay, an immediate switch without a fallback, and starting fresh without an import.

## Consequences

Easier:

- The server validates every write with the shared rules, regardless of which app sent it.
- Operations on several tasks become one transaction. The ADR 0001 project migration, today a sequence of guarded single-task commands with partial outcomes, can be applied atomically.
- Undo becomes a server-side history of mutations instead of Taskwarrior's single-step native undo.
- Reads are SQL queries instead of `task export` subprocesses.

Harder, or new work:

- Everything Taskwarrior did for Momentum must be rebuilt and checked against Taskwarrior: date expressions, urgency, recurrence generation, undo, and the import.
- Running a service: sign-in and session security, database migrations, backups (Neon point-in-time restore), monitoring, and hosting cost.
- No capture or editing without a connection or while the backend is down (P1). Cloud Run and Neon both scale to zero, so the first request after idle is slower.
- The maintainer and the hosting providers (Google Cloud, Neon) can read task contents (P3). Task contents must never be logged.
- In backend mode, Taskwarrior hooks, contexts, arbitrary filters, and the `task` command line no longer apply to Momentum's data.
- In backend mode, the sync state machine and the unsynced-changes quit prompt do not apply. A change is saved when its request succeeds.

To revisit:

- Offline use is closed by P1. Adding it later needs a new ADR.
- The TaskChampion server and its Neon database are retired only after cutover, with separate approval.
