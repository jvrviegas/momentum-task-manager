# Calendar awareness flow
> Local, read-only ICS planning input

Entry: `internal/calendar/calendar.go:Load()`
Flow: configured local paths → parse VEVENT/recurrence → filter requested local date → `BusyIntervalsWithOptions()` clips to work hours and merges overlaps → app stores an ephemeral count/minutes/event-summary/stale/error projection in `CalendarState` while the planner is open.

Policy: no URL fetching, credentials, event cache, Taskwarrior event fields, or event-content logging. All-day and transparent entries are explicit config choices. Missing/malformed/stale sources never block task-only planning.

Updated: 2026-09-21
