# Implementation plan — Plan Today in the macOS app

**Created:** 2026-10-22  
**Status:** D1–D7 are approved (2026-10-22). T01–T07 are done (2026-10-23).  
**Baseline:** `0d69645` on `main`. Recheck HEAD and the working tree before starting.  
**Design:** the "Plan Today" board of the [Momentum for macOS canvas](https://claude.ai/artifact/Sm68ju5s8c5CjKuSVSPye3), and the `.plan`, `.prow`, `.cap` and `.cal-strip` rules in its `project/momentum.css`. The canvas predates the server: ignore its Taskwarrior wording.

## Next-agent instructions

1. Read this plan, [`CONTRIBUTING.md`](../../CONTRIBUTING.md), [`macos/README.md`](../../macos/README.md), and the planning rules in [`tui/DESIGN.md`](../../tui/DESIGN.md) (VIEW-02, ROW-06, criteria 15 and 18).
2. D1–D7 are settled; do not reopen them.
3. Mark a task complete only after its tests and gates pass, and record the commit in the tracker.
4. Commit as [`AGENTS.md`](../../AGENTS.md) describes: one commit per file. Do not push without instruction.
5. Tests never reach a real server or read real calendars: bridge tests use the fake server, calendar tests write `.ics` fixtures to a temporary directory.

## Current state (verified at planning time)

| Area | Where | What it means |
|---|---|---|
| Planning rules | `core/domain/planner.go`: `BuildDailyPlan`, `BuildPlanMutations`, `DailyPlanTagFor`, `IsPlannedFor` | Obligations (overdue, due or scheduled today) are fixed; rollover (planned on an earlier day, still pending) starts selected; everything else is a candidate. Saving changes only `momentum-plan-YYYY-MM-DD` tags, never dates, and repeating it changes nothing. |
| TUI flow | `tui/internal/app/planner.go` (`OpenPlanner`, `CalendarCommand`, `recalculatePlannerSummaryFromUI`), `commands.go:118`, `ui/planner.go:105` | The TUI sums selected estimates itself while you tick, applies one `Modify` per task, stops at the first error with "applied k of n; retry is safe", and says "Daily plan unchanged" when nothing changes. Over capacity warns and never blocks. |
| Config | `tui/internal/config/planning.go` | `[planning]`: on by default, `daily_capacity` 8h, `buffer` 1h, optional `weekday_capacity`. `[calendar]`: off by default, local `.ics` `paths`, working hours 09:00–17:00, `stale_after` 24h. The bridge already loads this config (`tui/internal/bridge/ops.go:17`), once, at start. |
| Calendar | `tui/internal/calendar/calendar.go:54` | `Load` fails as a whole when any file can't be read or parsed. `BusyIntervalsWithOptions` clips to working hours and merges overlaps; `BusyMinutes` totals them. |
| Bridge | `tui/internal/bridge/server.go:193` (dispatch), `ops.go:28` (`load`), `wire.go` | No planning operations yet. `load` already derives every view with the domain functions. |
| Swift | `Bridge/ProcessBridge.swift:16`, `Model/AppStore.swift` | `hasOverlay` (`:124`) pauses auto-refresh (`:255`). `didMutate` (`:288`) reloads, opens the undo window and offers Undo. |
| Swift entry points | `Views/SidebarView.swift`, `Views/ContentView.swift:33` (toolbar), `Views/TaskKeyCommands.swift`, `App/MomentumCommands.swift`, `ShortcutsView` in `Views/ToastView.swift:84` | Where the sidebar box, the toolbar button, the `P` key, the menu item and the shortcut list go. |
| Server undo | `backend/README.md:58` | Undo reverts only the latest change made with the same token. A plan is several changes. |

## Decisions

| ID | Decision | Alternatives rejected | Status |
|---|---|---|---|
| D1 | Calendar awareness is in scope, built after the task-only sheet (T06). T06 reads `.ics` files; the macOS Calendar source chosen in Settings is [Settings C05](./macos-settings.md). | Tasks-only, calendar as its own plan | Approved 2026-10-22; source revised by Settings S4 |
| D2 | A saved plan offers no undo. The toast reads "Daily plan saved · N tasks" without an Undo button, and saving closes any open undo window so ⌘Z cannot revert one task's tag. Reopening Plan Today is how a plan changes. | A server batch endpoint undone as one change; offering Undo as the TUI does | Approved 2026-10-22 |
| D3 | The sidebar's "Today's plan" box reads the calendar on every `load`, so it and the sheet show the same numbers. Only busy minutes reach the sidebar. | Calendar from the last sheet open; a button with no meter | Approved 2026-10-22 |
| D4 | When a save stops partway, the sheet stays open with the choices kept, reloads tasks, and says "Saved k of n changes. Confirm again to finish — nothing is applied twice." Confirm Plan retries. | The design's result screen; closing with an error toast | Approved 2026-10-22 |
| D5 | Keys: `P` (⇧P) on the list or sidebar, the toolbar Plan button, the sidebar button, and a "Plan Today…" menu item with ⇧⌘P open the sheet. In it: ↑ ↓ or `j` `k` move over rows you can choose (obligations are skipped), Space ticks, `g` `G` go to the first or last, `/` or ⌘F focus the candidate filter, ↩ confirms (↩ in the filter returns to the list), Esc cancels (in the filter it clears the filter first). | TUI keys without the filter; standard macOS focus only | Approved 2026-10-22 |
| D6 | An unreadable `.ics` file makes the whole calendar unavailable, as in the TUI: the sheet says "Calendar unavailable: …", calendar time counts as zero, and planning continues with tasks. `calendar.Load` does not change. | Counting the readable files and noting "1 file missing", as the design shows | Approved 2026-10-22 |
| D7 | The sidebar counts only tasks tagged for today, the same tasks as Today's Planned Today section. Rollover counts once it is confirmed. "N of tasks" is obligations plus chosen estimates; "free" is capacity − obligations − calendar − buffer − chosen. | Counting pending rollover as chosen | Approved 2026-10-22 |

## Consequences already settled

| Source | Consequence |
|---|---|
| Code | The planning rules stay in Go. The bridge builds the plan with `BuildDailyPlan` and saves it with `BuildPlanMutations`; Swift only sums the ticked estimates while the sheet is open, as the TUI does. |
| Code | `applyPlan` exports pending tasks again and builds the changes from that, with the date the sheet was opened for, so a task completed elsewhere is skipped and a sheet left open past midnight still writes the date it showed. Tag changes are add/remove, so a retry is safe. |
| Code | The sheet counts as an overlay, so auto-refresh pauses while it is open and the single-key shortcuts stay off. |
| Code | `[planning] enabled = false` hides the sidebar box, the toolbar button, the menu item and `P`. The entry points are disabled while signed out, as New Task is. |
| TUI | Obligations can't be unticked. Over capacity warns and never blocks. Unestimated tasks are counted separately, never as zero. Saving an unchanged plan closes the sheet with "Daily plan unchanged". |
| D3 | Event titles are sent only in the `plan` result, for the sheet's strip, and are never stored or logged. |
| Settings S4, S5 | Each Mac picks its calendar source: `.ics` files (`[calendar]`, shared with the TUI) or macOS Calendar. With macOS Calendar, Swift reads today's events on each load and passes them to the bridge; D6 applies to files only. Working hours, all-day and transparent apply to both sources. |
| Config | The bridge reads `config.toml` once at start, so capacity and calendar settings changed in the file apply after the app restarts. The Settings plan may change this. |
| D2 | The TUI also offers Undo after a plan, which reverts only the last task's tag. That is a separate fix, out of scope here. |

## Implementation notes

Calls made during implementation where this plan left room:

- `applyPlan` receives the obligations the bridge reported as selected, that is, already tagged for today. Obligations are not newly tagged, as in the TUI, so an untouched plan answers `total` 0.
- The toast's "N tasks" counts the ticked rollover tasks and candidates, the same count as the sheet's "voluntary tasks selected".
- The toolbar button, the menu item and `P` show until a load says planning is off, and are disabled while signed out. The sidebar box needs a load's numbers, so it is hidden while signed out.
- The candidate filter matches as the main search does (`TaskSearch`). The sheet's list is not lazy, so the filter can take focus when it is scrolled out of view.
- ↑ ↓ and `j` `k` stop at the first and last rows, as the main list does; the TUI's planner wraps.
- The calendar header shows "Up to date", "Stale" or "Unavailable".

## Bridge contract

- `plan` (no params) returns `date` (`YYYY-MM-DD`), `tag`, `capacityMinutes` (for that weekday), `bufferMinutes`, and three lists, `obligations`, `rollover` and `candidates`. Each item holds the task as `taskWire` plus `selected`; a rollover item also holds `plannedDate`. From T06 it also returns `calendar`: `enabled`, `available`, `stale`, `error`, `busyMinutes`, `eventCount`, `workingStart`, `workingEnd`, and the merged busy blocks, each with its start, end and event titles.
- `applyPlan` takes `date` and `uuids` (every selected task, obligations included) and returns `applied` and `total`, plus `error` when it stopped partway. A `total` of 0 means the plan is unchanged.
- `load` gains `plan`: `enabled`, `chosenCount`, `chosenMinutes`, `unestimatedChosen`, `fixedMinutes`, `unestimatedFixed`, `capacityMinutes`, `bufferMinutes`, and from T06 `calendarMinutes`, `calendarAvailable` and `calendarError`.

## Tracker

| Status | ID | Task | Depends on | Verification |
|---|---|---|---|---|
| [x] | T01 | Bridge `plan` and `applyPlan`, tasks only | — | Bridge tests against the fake server: grouping and pre-selection, an unchanged plan answers `total` 0, a failure on the 2nd of 3 answers `applied` 1 and a retry finishes, a task completed between `plan` and `applyPlan` is skipped, the given date is the one written. Go gates in `tui/`. Done: `0a9d6e4`–`cb3fea8`. |
| [x] | T02 | Plan summary in `load` (D7) | T01 | Tests: rollover not counted until tagged, obligation minutes and unestimated counts, weekday capacity, `enabled` false. Done: `9b4c162`–`e7e565c`. |
| [x] | T03 | Swift wire types, bridge calls and store | T01, T02 | Swift tests with a fake bridge: ticking updates selected minutes and over-capacity, confirm closes the sheet and shows the toast without Undo and with the undo window closed (D2), a partial save keeps the sheet and the ticks (D4), "Daily plan unchanged". `xcodebuild -scheme Momentum test`. Done: `ba68301`–`a1ca351`. |
| [x] | T04 | Plan Today sheet | T03 | The obligations, rollover and candidates groups with their reasons ("Overdue 2d", "Due today 3:00 PM", "Planned yesterday"), the candidate filter with its count, the capacity column (the big remaining or over figure, the bar, the table, the unknown-time and over-capacity notes), and the header "Writes +momentum-plan-… ; never changes dates". 1040×720, stacked under 900 pt. Swift tests for the sheet's keys (D5), written like `TaskKeyCommands.handle`. Run the app in light and dark mode and compare with the board. Done: `7fe1b87`–`5a20ee5` with T05. Run against a fake server in light and dark mode: wide and stacked layouts, keys, the filter, over capacity, a partial save and its retry, an unchanged plan. |
| [x] | T05 | Entry points | T03 | The sidebar box (chosen count, meter, "N of tasks", "free", Plan Today ⇧P button), the toolbar Plan button, the "Plan Today…" menu item with ⇧⌘P, `P` in `TaskKeyCommands`, and entries in `ShortcutsView`. Swift test for `P`; everything hidden when planning is off. Done: `7fe1b87`–`5a20ee5` with T04. |
| [x] | T06 | Calendar (D1, D3, D6) | T04, T05 | The bridge reads calendars in `plan` and `load` as `app.CalendarCommand` does. The sheet gets "Calendar commitments": the strip of merged blocks over working hours with titles on hover, a now marker, the Stale badge and the error line. The capacity table gets "Calendar · N events" and the sidebar meter its calendar segment. Go tests with temporary `.ics` fixtures: busy minutes, a missing file makes the calendar unavailable, a stale file, calendar off. Done: `65005c8`–`23feb7e`, `0cf358c`. Run with a stale fixture (strip, now marker, Stale badge, calendar row and segment) and a missing file (error line, zero calendar time). Titles on hover were not checked on screen. |
| [x] | T07 | Docs | T06 | `macos/README.md`: describe Plan Today and its keys, and remove Plan Today and Calendar awareness from "Not yet built". Done: `52141c7`. |
