# Implementation plan — read-only calendar awareness

**Goal:** [G4 — Read-only calendar awareness](task-first-daily-planner-goals.md#g4--read-only-calendar-awareness)
**Status:** Implemented; local ICS provider selected; automated fixture evidence present; live UAT pending

## Feasibility decision

The first provider is explicitly configured local iCalendar (`.ics`) files. Momentum does not fetch URLs, authenticate to a provider, write events, or keep an event cache. A separate calendar client or sync job may refresh a subscribed file before planning is opened. This preserves offline behavior on Linux/macOS and avoids adding provider credentials or application network traffic.

Supported configuration:

```toml
[calendar]
enabled = true
paths = ["~/Calendars/work.ics", "~/Calendars/personal.ics"]
include_all_day = false
include_transparent = false
working_start = "09:00"
working_end = "17:00"
stale_after = "24h"
```

## Behavior

- Local ICS parsing handles timed events, all-day events, transparency, timezone parameters, common RRULE daily/weekly/monthly/yearly expansion, duplicate UID/start/end instances, and folded lines.
- Busy intervals are clipped to configured work hours and merged before minutes are calculated.
- All-day and transparent treatment is explicit configuration. Events outside work hours do not consume focus capacity.
- Missing, malformed, or stale files produce an unavailable/stale summary; task-only planning remains usable.
- While the planner is open, the app holds a short-lived event summary with start/end times for the read-only ritual. Event summaries/attendees are not persisted, sent to Taskwarrior, or written to logs; closing/reopening reloads the source.

## Evidence

- `internal/calendar/calendar_test.go` covers overlap merging, all-day policy, transparent policy, recurrence expansion, deduplication, and missing-file degradation.
- `internal/config/planning_test.go` covers local-only source validation and defaults.
- `internal/app/planner.go` loads the source asynchronously and merges its summary into the existing ritual without blocking or changing task data.
- `internal/diagnostics/doctor.go` reports only source count/readability when calendar is enabled.

## Independent demo

Use two disposable ICS fixtures with overlapping meetings, a recurring event, an all-day event, and a transparent event. Open `P` with calendar enabled, compare merged busy minutes at working hours, then remove one file and reopen: the ritual must show a clear unavailable state while still permitting task selection and confirmation.
