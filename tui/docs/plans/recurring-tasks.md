# Implementation plan — native recurring tasks

**Goal:** [G2 — Recurring-task management](task-first-daily-planner-goals.md#g2--recurring-task-management)
**Status:** Implemented; automated isolated Taskwarrior evidence present; live UAT pending

## Decisions

- Taskwarrior remains the recurrence engine. Momentum creates native `recur:` templates with a first `due:` anchor and never runs a background scheduler.
- Supported presets are daily, weekdays, weekly, selected weekday, monthly, and intervals such as every two weeks. Momentum normalizes interval aliases to Taskwarrior-safe expressions (`2wks`, `3mo`, etc.).
- Natural-language recurrence always opens review. Explicit `^` recurrence syntax is available for fast capture.
- Exported instances retain `parent`, `imask`, `recur`, and `rtype`; Details labels generated instances and their template UUID.
- Editing recurrence alone on a generated instance targets its template with recurrence confirmation disabled. Mixed recurrence plus ordinary-field edits are rejected to avoid an accidental propagation policy.
- Stopping a series uses the template's `until:today` with `rc.recurrence.confirmation=no`. It preserves generated and completed history; clearing `recur:` directly is not used because Taskwarrior rejects that operation for templates.
- The device-level Taskwarrior recurrence-generation setting remains outside Momentum. The existing single recurrence-primary deployment policy applies; Momentum does not change it.

## Acceptance evidence

- `internal/domain/recurrence.go` validates presets, interval aliases, anchors, and Taskwarrior-equivalent next-occurrence previews; isolated differential tests cover time-of-day, month-end/leap-year clamping, intervals, and DST transitions.
- `internal/quickadd/interpret.go` recognizes `every day`, `every weekday`, `every week`, selected weekdays, monthly, and positive intervals.
- `internal/ui/edit.go` adds the Recurrence field and `R` focus shortcut; Details offers `x` to stop a series.
- `internal/taskwarrior/mutations.go` builds recurrence-safe add/modify/stop argv.
- `internal/taskwarrior/integration_recurrence_test.go` creates a recurring template, observes a generated instance/parent, and expires the template in an isolated temporary Taskwarrior profile.

## Independent demo

Capture `Review release every Friday about an hour`, review the Friday anchor, recurrence, and estimate, confirm, inspect the generated pending instance, press `Enter` for Details, then press `x` and confirm. Verify the template has an `until` value and completed history is untouched.
