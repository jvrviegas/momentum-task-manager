# Recurrence flow
> Native Taskwarrior template/instance lifecycle

Entry: `internal/domain/recurrence.go:ParseRecurrencePhrase()`
Flow: quick-add explicit `^` or local `quickadd.Interpret()` phrase → validated expression + due anchor → `task add ... due:... recur:...` → Taskwarrior template + generated pending instance.

Export: `domain.Task.Parent`, `MaskIndex`, `RecurrenceType`, and `Recurrence` distinguish generated instances. Details labels the parent UUID.

Mutation policy:
- Recurrence-only edit on an instance targets `RecurrenceTargetUUID()` and adds `rc.recurrence.confirmation=no`.
- Stop uses `CommandClient.StopRecurrence()` → `until:today`; direct `recur:` clearing is rejected by Taskwarrior 3.5 templates.
- Momentum never generates recurrence in the background; the recurrence-primary device policy remains Taskwarrior configuration.

Updated: 2026-09-21
