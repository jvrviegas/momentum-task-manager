# Implementation plan — Settings window in the macOS app

**Created:** 2026-10-22  
**Status:** S1–S7 are approved (2026-10-22). C01–C04 are done (2026-10-28).  
**Baseline:** `0d69645` on `main`. Recheck HEAD and the working tree before starting.  
**Design:** the canvas has no Settings board; its `project/momentum.css` "Settings window" rules (`.settings`, `.tabs`, `.group`, `.switch`, `.status`, `.theme-cards`, `.ptree`, `.wd-grid`, `.migration`) set the look: a 760×600 window with toolbar tabs and grouped rows. Its `.log` panel was for Taskwarrior commands and does not apply.  
**Related:** [Plan Today](./macos-plan-today.md). Its calendar step (T06) comes before C05 here.

## Next-agent instructions

1. Read this plan, [`CONTRIBUTING.md`](../../CONTRIBUTING.md), [`macos/README.md`](../../macos/README.md), the Projects requirements in [`tui/docs/spec/settings-projects.md`](../../tui/docs/spec/settings-projects.md) (SP-01–SP-16), and [`tui/docs/plans/calendar-awareness.md`](../../tui/docs/plans/calendar-awareness.md).
2. S1–S7 are settled; do not reopen them.
3. Mark a task complete only after its tests and gates pass, and record the commit in the tracker.
4. Commit as [`AGENTS.md`](../../AGENTS.md) describes: one commit per file. Do not push without instruction.
5. Tests never write the user's `config.toml`, reach a real server, or read real calendars: use temporary config files, the fake server, `.ics` fixtures, and a fake event source in place of EventKit.

## Current state (verified at planning time)

| Area | Where | What it means |
|---|---|---|
| App scene | `macos/Momentum/App/MomentumApp.swift` | One `Window` scene; no `Settings` scene. The design's sidebar shows "Settings ⌘,"; the app's sidebar has no such row. |
| Config writer | `tui/internal/config/projects_store.go:114` (`save` with a `tomledit` patch), `theme_store.go` | Keeps comments and unrelated keys, refuses to write when the file changed since it was read, preserves symlinks, replaces atomically. New planning and calendar patches can reuse it. |
| Config reading | `tui/internal/bridge/ops.go:17` | The bridge reads `config.toml` once at start; the README says to restart after editing it. |
| Project rules | `core/domain/project_edit.go` (`PlanAddDraft`, `PlanUpdateDraft`, `PlanRemoveProject`, `ComposeProjectValue`), `core/domain/project_migration.go`, `core/api/project.go` | Validation, hierarchy and migration mapping are shared Go. The server previews and applies a migration in one transaction. The TUI's ordering (catalog first, then tasks) lives in `tui/internal/app/project_migration.go` and `rename_workflow.go`, which import Bubble Tea, so the bridge repeats the ordering rather than importing them. |
| Planning and calendar config | `tui/internal/config/planning.go` | `[planning]` `enabled`, `daily_capacity`, `buffer`, `weekday_capacity`; `[calendar]` `enabled`, `paths`, `include_all_day`, `include_transparent`, `working_start`, `working_end`, `stale_after`. `[calendar] enabled` without `paths` fails validation. |
| Diagnostics | `tui/internal/diagnostics/doctor.go:67` | `Run` checks the server, sign-in, time zone, config validity, calendar files, terminal and state directory, read-only. The bridge writes no log. |
| Build | `macos/project.yml` | The Info.plist is generated (`INFOPLIST_KEY_…`); hardened runtime is off, so EventKit needs only a usage description for now. |

## Decisions

| ID | Decision | Alternatives rejected | Status |
|---|---|---|---|
| S1 | Five tabs: Projects, Planning, Calendar, Appearance, Diagnostics. | Fewer tabs; Diagnostics left out | Approved 2026-10-22 |
| S2 | Appearance (System, Light, Dark) is a Mac-only preference, applied at once. The TUI's `theme` key is not read or written. | Sharing `config.toml`'s `theme` | Approved 2026-10-22 |
| S3 | Each change applies at once: switches, steppers and pickers save immediately; text fields save on Return or when they lose focus; an invalid value shows inline and is not saved. In Projects, each add, edit or remove is saved from its own sheet, and a value change that moves tasks goes through a preview-and-confirm sheet. | A draft per tab with Save and a discard prompt, as the TUI | Approved 2026-10-22 |
| S4 | The Calendar tab offers two sources, chosen per Mac: calendar files (`.ics`, as the TUI) or macOS Calendar (EventKit). | `.ics` only; EventKit only | Approved 2026-10-22 |
| S5 | Storage is split by what is shared. Mac-only preferences: the source and the chosen macOS calendars. `config.toml` `[calendar]`, shared with the TUI: the files switch and paths, plus working hours, all-day and transparent, which apply to both sources. `stale_after` applies to files only. Choosing macOS Calendar never changes `[calendar] enabled` or `paths`. | Everything in `config.toml`; a separate Mac copy of all calendar settings | Approved 2026-10-22 |
| S6 | The bridge reloads `config.toml` when its modification time changes, checked on each load and before Settings reads it, so edits from the TUI or an editor apply without a restart. A file that no longer validates keeps the last good settings and shows the error in Settings and Diagnostics. A changed `[backend] url` still needs a restart. | Reading only after a refused save; a restart for every edit | Approved 2026-10-22 |
| S7 | Projects lists catalog entries and values found only on tasks in one tree, each tagged "Catalog" or "From tasks". A "From tasks" row offers "Add to Catalog…", which opens the add sheet prefilled; nothing is added until that sheet is saved. | Catalog entries only, as the TUI | Approved 2026-10-22 |

## Consequences already settled

| Source | Consequence |
|---|---|
| Convention | Settings is a SwiftUI `Settings` scene with toolbar tabs, opened with ⌘, and from a "Settings" row in the sidebar footer. The design's "Context" row is not added back. |
| Code | Every config write goes through the bridge and the existing writer: comments are kept, a symlinked config stays a symlink, and a save is refused when the file changed since it was read. After a refused save, the tab reloads from disk, says the file changed outside Momentum, and asks you to make the change again. Nothing is overwritten silently (SP-13). |
| Code | After its own save, the bridge updates the config it holds, so suggestions, the sidebar meter and Plan Today use the new values at once. |
| SP-01–SP-16 | The TUI's Projects requirements govern the Projects tab, except SP-06's draft and discard prompt, which S3 replaces with per-action sheets. Catalog-only edits never change tasks. A value change offers "Catalog only" (default) or "Catalog + pending tasks", with "Include subprojects", counts, a merge warning, and the note that history keeps its values. The catalog saves before the migration, the server moves all tasks or none, a migration has no undo and closes the undo window, and a stale preview needs a fresh one. |
| S3 | The Planning tab's durations are typed as `8h` or `7h30m` and checked with the config's own validation before they are saved. An empty weekday field means the daily capacity. Turning planning off hides Plan Today everywhere. |
| S4 | Choosing macOS Calendar asks for calendar access. When access is denied, the tab says so and offers to open System Settings › Privacy & Security › Calendars, and Plan Today shows "Calendar unavailable: no access". The Info.plist gains `NSCalendarsFullAccessUsageDescription`. |
| S4 | With macOS Calendar, Swift reads today's events and passes their start, end, all-day and free/busy state to the bridge on each `load` and `plan`. The bridge merges and clips them with the same functions as files. Event titles are sent only with `plan`. |
| S5 | With the files source, per-file status (available, stale, missing, error) comes from loading each file on its own; how Plan Today treats an unreadable file is unchanged (Plan Today D6). |
| Code | Diagnostics runs `diagnostics.Run` through the bridge without the Terminal check, and adds the app and bridge versions, the config path with "Show in Finder", and "Copy Report". |

## Bridge contract

- `settings` returns the config path, a revision, the config error if the file no longer validates, the project tree (value, name, tag, task count), the `[planning]` values, the `[calendar]` values, and each calendar file's status.
- `savePlanning` and `saveCalendar` take the revision and the changed keys, validate the whole config, and return the new revision or a conflict.
- `planProject` takes an add, edit or remove with its draft, `includeSubprojects` and `removeChildren`, and returns what the catalog would become, whether the value changes, and the descendant counts. `previewMigration` returns the server's preview. `saveProject` takes the revision, the same edit and, when tasks move, the confirmed preview; it saves the catalog first, then asks the server to apply the migration.
- `doctor` returns the checks.
- `load` and `plan` take optional `events` when the source is macOS Calendar.

## Tracker

| Status | ID | Task | Depends on | Verification |
|---|---|---|---|---|
| [x] | C01 | Bridge: reload on change (S6), `settings`, `savePlanning`, `saveCalendar` | — | Go tests on temporary configs: comments and unrelated keys kept, a symlink kept, a conflict refused, an invalid file keeps the last good config, a changed `[backend] url` ignored until restart, `[calendar] enabled` without paths refused. Go gates in `tui/`. Done: `e5818ec`–`da17973` (reload, status), `c61fc7e`–`4a1a885` (settings, saves, per-file status). |
| [x] | C02 | Settings scene, sidebar row, Appearance tab (S2) | — | Swift test that the preference maps to the color scheme. Run the app: ⌘, and the sidebar row open it; System, Light and Dark switch at once. Done: `f3bb93f`–`20aa25d`, `508a6a6`. |
| [x] | C03 | Planning tab | C01 | The enabled switch, daily capacity, buffer and the weekday grid. Swift tests with a fake bridge: a valid value saves, an invalid one stays inline and unsaved, a conflict reloads. The sidebar meter changes on the next load. Done: `167784a`–`34741f9`. |
| [x] | C04 | Calendar tab, files source | C01, Plan Today T06 | Files added with an open panel (stored with `~/` under home), removed, and shown with their status; working hours, all-day, transparent and staleness. Go tests for per-file status with fixtures. Done: the switch, the file list, **Add…**, **Remove** and each file's status (`167784a`–`34741f9`, story S-045), and working hours, all-day, transparent and staleness (story S-046, `73423d0`–`5eabe1e`). |
| [ ] | C05 | Calendar tab, macOS Calendar source (S4, S5) | C04 | The source picker, the permission request and the denied state, the calendar list by account, Mac-only preferences, and events passed on `load` and `plan`. Go tests: passed events give the same busy minutes as the same events from a file. Swift tests with a fake event source. Run the app with access granted and denied. |
| [ ] | C06 | Bridge project operations | C01 | Go tests mirroring SP-16: invalid drafts write nothing, removing with and without children, `workshop` not matched by `work`, catalog saved before the migration, a refused migration keeps the saved catalog, a stale preview refused. |
| [ ] | C07 | Projects tab | C06 | The tree with "Catalog" and "From tasks" tags (S7), the add and edit sheet with parent picker and full value, the remove prompt with the child count, the rename sheet with its options, counts and merge warning, and "Add to Catalog…". Swift tests with a fake bridge for each flow; the undo window closes after a migration. |
| [ ] | C08 | Diagnostics tab | C01 | The checks, versions, config path, "Show in Finder" and "Copy Report". Go test for `doctor` against the fake server; the copied report holds no token. |
| [ ] | C09 | Docs | C01–C08 | `macos/README.md`: describe Settings and its tabs, say that config edits apply without restart except `[backend] url`, and remove Settings window and Project migration from "Not yet built". |
