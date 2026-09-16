# Taskwarrior Flow
> Direct argv adapter with export-derived views

Entry: `internal/taskwarrior/client.go:CommandClient.ExportPending()`
Flow: app command → `CommandClient.run()` → injectable `Runner`/`exec.CommandContext` → JSON export → `domain.Task` → local Inbox/Today derivation.

Commands: `status:pending export`; `_projects`; `_get rc.context`; mutations use UUID first and never a shell.
- `task _get sync.server.url` exits non-zero when sync is absent; app treats that as local-only.
- `_projects` failure is tolerated when pending export can supply project values.
- `task.tags` supplies user tags; virtual tags are not exported in that field.

Integration guard: `internal/taskwarrior/integration_test.go:newIsolatedEnvironment()` sets temporary `TASKRC`/`TASKDATA` before any process and rejects `~/.taskrc`/`~/.task`.

Updated: 2026-09-08
