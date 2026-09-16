# Sync Flow
> Native Taskwarrior sync with local grace/retry state

Entry: `internal/app/sync_state.go:SyncState.Apply()`
Flow: local export visible → optional readiness check → startup `task sync` → refresh; mutations set a 15s grace timer → manual sync closes undo → success refreshes and resets retry → failure schedules 15s/30s/1m/2m/5m.

`internal/app/sync.go` owns Bubble Tea timer commands; no timer/service is installed outside the process.
- Missing Taskwarrior sync settings disable Momentum sync while preserving local work.
- `MutationUndo` applies the `SyncUndo` transition; it must not create a new dirty grace window.
- Shutdown with app-originated unsynced work routes through the generated quit modal.

Updated: 2026-09-08
