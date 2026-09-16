# Taskwarrior Flow
> Direct argv adapter with export-derived views

Entry: `internal/taskwarrior/client.go:CommandClient.ExportPending()`
Flow: app command → `CommandClient.run()` → injectable `Runner`/`exec.CommandContext` → JSON export → `domain.Task` → local Inbox/Today derivation.

Commands: `status:pending export`; `_projects`; `_get rc.context`; Settings migrations use `_get rc.context.<name>.read`, `status:pending recur.none:`, `project.is:<old>`, and one UUID per guarded modify. Mutations use argv first and never a shell.
- `task export` deliberately ignores the active context in Taskwarrior 3.5.0. Context-scoped migration must read `_get rc.context`, then `_get rc.context.<name>.read`, wrap that filter in parentheses as one argv element, and pass it explicitly.
- Pending-only migration guards use `status:pending recur.none:` plus `project.is:<old>`; successful commands can be silent and no-match returns exit 1, so reconcile the UUID export after every command.
- `on-modify` hooks receive original/modified JSON and may reject or change additional fields; keep hooks enabled and report reconciled outcomes.
- `task undo` reverses only the latest Taskwarrior action, not a config save or a multi-command migration.
- `task _get sync.server.url` exits non-zero when sync is absent; app treats that as local-only.
- `_projects` failure is tolerated when pending export can supply project values.
- `task.tags` supplies user tags; virtual tags are not exported in that field.

Integration guard: `internal/taskwarrior/integration_test.go:newIsolatedEnvironment()` sets temporary `TASKRC`/`TASKDATA` before any process and rejects `~/.taskrc`/`~/.task`. Detailed probes and config-store constraints: `docs/plans/settings-projects.md#t00-feasibility-record--2026-09-16`.

Updated: 2026-09-16
