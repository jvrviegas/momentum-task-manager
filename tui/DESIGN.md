# Momentum Design

**Status:** Implemented; see `docs/UAT.md` for acceptance evidence  
**Application:** Momentum  
**Module path:** `github.com/jvrviegas/momentum-task-manager/tui`  
**Target directory:** `/home/joaovvr/Projects/Personal/momentum`  
**Last updated:** 2026-10-19 (Taskwarrior replaced by the Momentum server, backend plan T19)

## 1. Product Summary

Momentum is a polished, keyboard-first terminal task manager. It gives tasks a focused two-view workflow, fast structured editing, and a quick-add command bar with contextual suggestions.

Tasks live on a Momentum server ([ADR 0002](../docs/adr/0002-go-postgres-backend.md)): a Go service with PostgreSQL that keeps Taskwarrior's task model, urgency, and recurrence presets, and that the TUI and the macOS app reach over HTTPS. The apps keep no local task store; the server owns persistence, validation, recurrence, and undo history. Taskwarrior is used only by `momentum import`, which copies its tasks to the server. Momentum may store a local project suggestion catalog in its configuration; these entries are not tasks and never change existing task assignments.

### Delivered extension: Settings → Projects

Momentum includes in-app catalog management and an explicit opt-in to migrate changed project values on eligible pending tasks while leaving historical values intact. The narrow exception to the bulk-operations non-goal is recorded in [ADR 0001](docs/adr/0001-project-settings-and-pending-task-renames.md); detailed [requirements](docs/spec/settings-projects.md), [implementation evidence](docs/plans/settings-projects.md), and [UAT evidence](docs/UAT.md) govern the shipped behavior. Child-removal prompts, duplicate rejection, catalog-first outcomes, no batch undo, and symlink preservation are implemented as recorded; since the server replaced Taskwarrior, it applies a confirmed migration in one transaction and contexts no longer scope it.

## 2. Goals

- Present pending tasks in two disjoint smart views, Inbox and Today, plus a recent read-only Completed view.
- Make task capture fast through a command bar and compact trigger syntax.
- Make common field edits reachable with one key.
- Provide a visually polished, responsive Bubble Tea interface.
- Keep every device on the same tasks through one server, and say plainly when it is unreachable.
- Support macOS and Linux without requiring a Nerd Font.
- Need nothing but a `[backend] url` and a sign-in.

## 3. Non-goals for v1

- Offline use or a local task store ([ADR 0002](../docs/adr/0002-go-postgres-backend.md), P1).
- Custom or user-defined views.
- A Kanban board.
- General bulk task operations. The explicit Settings → Projects pending-only value migration is the narrow documented exception; it is not a general bulk editor.
- Editing annotations, dependencies, or arbitrary UDAs. Native recurrence presets and daily-plan tags are supported extensions documented below.
- Contexts and hooks ([ADR 0002](../docs/adr/0002-go-postgres-backend.md), D6).
- Desktop notifications or background execution while Momentum is closed.
- Native Windows support.
- Automatic update checks, telemetry, or crash reporting.
- A Cloud Run/Neon provisioning tool or Terraform configuration.
- Fully configurable keybindings.

## 4. Functional Requirements

### Views

- **VIEW-01:** Inbox contains pending tasks that are not overdue, due today, or scheduled today.
- **VIEW-02:** Today contains unique pending tasks that are overdue, due today, scheduled today, or explicitly planned for today.
- **VIEW-03:** Today groups tasks in this order: Overdue, Due Today, Scheduled Today, Planned Today.
- **VIEW-04:** A task matching more than one Today group appears once in its highest-priority group.
- **VIEW-05:** Inbox sorts by urgency descending. Urgency is Taskwarrior's default formula, computed in `core/domain`.
- **VIEW-06:** Today sorts by group and then urgency descending within each group.
- **VIEW-07:** Date classification uses the machine's local calendar date.
- **VIEW-08:** Startup opens Today when it is non-empty and Inbox otherwise.
- **VIEW-09:** Sidebar counts show the unique number of tasks in each view.
- **VIEW-10:** Selection is preserved by task UUID across refreshes when possible.
- **VIEW-11:** Completed contains tasks completed in the last 30 local calendar days, including today.
- **VIEW-12:** Completed groups tasks into Today, Yesterday, and Earlier, with newest completion first in each group.
- **VIEW-13:** Completed tasks support navigation, search, details, and refresh but not task-targeted mutations; quick capture and global undo remain available.
- **VIEW-14:** Automatic startup never selects Completed.

Today classification precedence:

1. `due` local date is before today: Overdue.
2. `due` local date equals today: Due Today.
3. `scheduled` local date equals today: Scheduled Today.
4. Otherwise the task is not in Today.

A scheduled date before today does not make a task overdue. Overdue is based on `due` only.

### Task rows

- **ROW-01:** A row shows completion state, description, project badge, relevant due/scheduled value, priority badge, and active-task indicator; comfortable/wide pending rows also show an optional Estimate metadata value.
- **ROW-02:** UUID, numeric ID, tags, raw urgency, recurrence, dependencies, and annotations are hidden from the default row.
- **ROW-03:** Color is never the only indication of state.
- **ROW-04:** Selected rows use three cues: a `▌` gutter on both lines (ASCII `>` on the first), the Selection fill, and a bold title.
- **ROW-05:** Comfortable rows are two lines. Line 1 is gutter, state glyph, title, and a right-edge 6-cell slot (priority, or completion time in Completed). Line 2 starts at the title column with `Active · #project · due or scheduled · Estimate`, joined by ` · `, and drops parts in a fixed order (Estimate, then project, then Active/scheduled, then the overdue phrase last) instead of wrapping. Completed rows never show an estimate. Below 50 columns a row is one line with a trailing `!` when overdue.
- **ROW-06:** Daily-plan tags remain ordinary task tags and are not a separate plan store.

Example:

```text
▌ ▶ Send Q3 invoice to Acme Corp                      ↑ High
▌   Active · #work.client.acme · Due Today 17:00
  □ Renew the TLS certificate                          ↑ High
    #ops.infra · ! Overdue · Due Sep 28 10:00
```

### Quick add

- **ADD-01:** `Ctrl+K` opens a bottom command bar.
- **ADD-02:** Plain text becomes the task description.
- **ADD-03:** Metadata triggers activate only at the start of a whitespace-delimited token.
- **ADD-04:** A leading backslash escapes a trigger and is removed from the final description.
- **ADD-05:** Capture reaches the server as structured fields; no typed text is executed.
- **ADD-06:** The app resolves date expressions to instants in the device's time zone ([ADR 0002](../docs/adr/0002-go-postgres-backend.md), D3), and the server validates every change.

Trigger grammar:

| Trigger | Field | Examples | Stored as |
|---|---|---|---|
| `#` | Project | `#work.client` | project `work.client` |
| `!` | Priority | `!high`, `!medium`, `!low`, `!none` | priority `H`, `M`, `L`, or none |
| `@` | Due | `@today`, `@tomorrow`, `@2026-09-20` | due instant |
| `>` | Scheduled | `>today`, `>monday` | scheduled instant |
| `+` | Tag | `+planning` | tag `planning` |
| `~` | Estimate | `~15m`, `~1h30m` | estimate in minutes |
| `^` | Recurrence | `^daily`, `^weekly`, `^2wks` | validated recurrence rule |

Rules:

- One project token is permitted.
- One priority, due, and scheduled token is permitted; the parser rejects duplicate scalar metadata with a useful error.
- Multiple unique tags are permitted.
- One estimate token is permitted. It accepts positive whole minutes from 1 minute through 24 hours (`15m`, `90m`, `1h`, `1.5h`, or `1h30m`) and is removed from the final description.
- Unknown project and tag values are accepted, allowing inline creation.
- The description must not be empty after metadata extraction.
- Email addresses such as `bob@example.com` remain description text because `@` is not at a token boundary.
- `\#launch` and `\~1h` become literal description text `#launch` and `~1h`.

Autocomplete:

- Suggestions appear below the input and replace the idle syntax guide, keeping the capture path uncluttered.
- `Up`/`Down` and `Ctrl+P`/`Ctrl+N` move through suggestions.
- `Tab` accepts the highlighted suggestion.
- `Enter` submits the whole task; `Ctrl+S` is an alternative. In correction review, either key confirms the current draft after validation.
- First `Esc` dismisses suggestions; a second `Esc` closes quick add.
- With the cursor on or directly after a highlighted natural-language phrase, `Esc` keeps that phrase as description text instead of closing; editing the phrase makes it recognizable again.
- Projects and tags use fuzzy matching.
- Priority uses a fixed list.
- Due and scheduled suggestions include today, tomorrow, upcoming weekdays, `next-week` (Friday of the next calendar week), and ISO-date guidance.
- A date need not be suggested to be accepted: Taskwarrior's named dates (`sod`, `eow`, `eom`, `soy`, `eoq`, `later`, `1st`, …), weekdays, ISO dates, and `YYYY-MM-DD HH:MM` resolve on the device. Date arithmetic, bare durations, and holidays (`today+3d`, `2wks`, `easter`) are refused with a message (D3).

### Structured editing

- **EDIT-01:** `e` opens a structured modal focused on Description.
- **EDIT-02:** Direct task-list shortcuts open the same modal focused on a specific field.
- **EDIT-03:** `Tab` accepts a visible suggestion without changing fields; otherwise it moves to the next field. After accepting, another `Tab` moves forward. `Shift+Tab` always moves to the previous field without accepting suggestions.
- **EDIT-04:** `Up`/`Down` and `Ctrl+P`/`Ctrl+N` navigate field suggestions only; `Tab` accepts the highlighted suggestion.
- **EDIT-05:** `Enter` saves only changed supported fields using the current input, never the highlighted suggestion. `Ctrl+S` remains an alternative; validation failures retain the editor.
- **EDIT-06:** `Esc` cancels without mutation.
- **EDIT-07:** Unsupported exported fields remain untouched.

Fields and direct shortcuts:

| Key | Initial field |
|---|---|
| `e` | Description |
| `E` | Estimate |
| `R` | Recurrence |
| `p` | Project |
| `!` | Priority |
| `D` | Due |
| `S` | Scheduled |
| `t` | Tags |

Editable fields are Description, Project, Priority, Due, Scheduled, Tags, Estimate, and Recurrence. Project/tag/estimate fields autocomplete, priority uses a selection list, and date fields show existing timestamps in local `YYYY-MM-DD HH:MM` form while still accepting the date expressions quick add does. Estimate values use the shared positive whole-minute parser and display compactly (`15m`, `1h`, `1h 30m`); `E` focuses Estimate while lowercase `e` remains Description. On a date field, `Ctrl+Left`/`Ctrl+Right` step one calendar day and `Ctrl+Up`/`Ctrl+Down` step 30 minutes. Removing a field value clears it on the server.

### Details

- **DETAIL-01:** `Enter` opens a centered, primarily read-only details modal.
- **DETAIL-02:** Details include all available supported information: description, status, project, priority, Estimate, dates, tags, annotations, dependencies, recurrence, urgency, UUID, and unknown exported properties where practical.
- **DETAIL-03:** `e` moves from details to the structured edit modal.
- **DETAIL-04:** `Esc` or `Enter` closes details.

### Mutations

- **MUT-01:** `Space` completes the selected task immediately.
- **MUT-02:** `s` starts or stops the selected task.
- **MUT-03:** `d` requests deletion and requires `y/n` confirmation.
- **MUT-04:** `u` asks the server to undo this device's latest change while the undo window is open (`mutation_delay`; ADR 0002, D7).
- **MUT-05:** Only one mutation runs at a time.
- **MUT-06:** Every successful mutation is followed by a fresh load from the server.
- **MUT-07:** Hooks are not supported (ADR 0002, D6).
- **MUT-08:** Momentum owns confirmations; the server never prompts.
- **MUT-09:** Success and failure messages appear inside the TUI.
- **MUT-10:** After completion or deletion, selection moves predictably to the next task, then the previous task if no next task exists.

### Search

- **SEARCH-01:** `/` opens a local fuzzy filter for the current view.
- **SEARCH-02:** Search covers description, project, and tags.
- **SEARCH-03:** Search filters the in-memory view and does not reach the server.
- **SEARCH-04:** `project:<name>` restricts results to an exact case-insensitive project match and can be combined with fuzzy text; `project:none` selects unassigned tasks.
- **SEARCH-04:** `Enter` keeps the filter active; `Esc` clears it.
- **SEARCH-05:** The UI displays match count while filtering.

### Refresh

- **REFRESH-01:** `r` refreshes tasks manually.
- **REFRESH-02:** Background task refresh defaults to 60 seconds.
- **REFRESH-03:** The refresh interval is configurable and `0s` disables it.
- **REFRESH-04:** Refresh pauses while quick add, search editing, or an edit/confirmation modal is active.
- **REFRESH-05:** Existing task content remains visible during non-initial refreshes.

### Empty states

Empty states sit at the title column: what would appear here and the keys that get you somewhere.

Inbox:

```text
▱ No pending tasks
  Tasks that are not due or scheduled today land here.
  [c] capture a task
```

Today:

```text
◷ Nothing due today
  Overdue, due and scheduled tasks land here.
  [c] capture a task    [1] open Inbox · N waiting
```

Completed:

```text
✓ No recently completed tasks
  Tasks completed in the last 30 days appear here.
  [c] capture a task    [2] open Today
```

Loading draws `░` skeleton rows at the real row positions and omits counts until they are known. A load failure shows `! Could not load tasks`, the error, that no data changed, and `r` to retry.

## 5. Keyboard and Mouse Map

| Input | Action |
|---|---|
| `j` / `k`, `Down` / `Up` | Move task selection |
| `h` / `l`, `Tab` | Switch sidebar/list focus |
| `g` / `G` | First/last task |
| `1` / `2` / `3` / `4` | Inbox/Today/Completed/Settings → Projects |
| `Enter` | Open/close details as context permits |
| `Ctrl+K` | Quick add |
| `c` / `Ctrl+K` | Create a task with quick capture |
| `/` | Search current view |
| `Space` | Complete task |
| `e`, `E`, `R`, `p`, `!`, `D`, `S`, `t` | Structured edit with initial field |
| `P` | Open the daily planning ritual |
| `X` | Stop the selected recurrence template |
| `s` | Start/stop task |
| `d` | Delete confirmation |
| `u` | Undo the latest change while the undo window is open |
| `r` | Reload tasks from the server |
| `?` | Help |
| `q` | Quit |
| `Esc` | Close/cancel the active overlay |

Mouse support is intentionally limited:

- Click sidebar views.
- Click tasks to select them.
- Mouse wheel scrolls the task list.
- No drag-and-drop or hover-only actions.

Keybindings are fixed in v1. The help screen must be generated from the application's keybinding definitions so it cannot drift from behavior. It lays them out as Navigate, Tasks and App columns plus a Mouse section.

## 6. Visual Design

Momentum's visual language is built only from what a terminal can draw: foreground, background, bold, underline and reverse on a monospace grid. The source of truth is the "Momentum TUI redesign" canvas; this section summarizes it.

- **Hierarchy comes from column position first** (section at column 0, state glyph at 2, title and metadata at 4), weight second, color third.
- **Two edges, two jobs.** The left edge reads as an outline; the right edge is a fixed 6-cell slot (`↑ High`, `→ Med`, `↓ Low`, or the completion time in Completed).
- **Section headers** are `Title  count  ─────`: Overdue in bold Red, Inbox projects in bold Cyan with `#`, No project in bold Muted, otherwise bold Text. One blank row precedes each header except the first, removed below 50 columns. A list that overflows reports `↓ n more` on its last row.
- **Modals are framed, not listed.** Title and context sit in the top border, keys in the bottom border, a scroll thumb `┃` and `a-b of n` show position, every cell has a Panel fill, and the view behind is redrawn in one Dim color with its fills removed. Destructive frames use a Red border and a `! Title`.
- **Forms** use a 12-cell label column: `•` marks a changed field with a Muted `was …`, the focused field gets `▌`, a bold label and a filled input with a reverse-video cursor cell, and an invalid value swaps `•` for a bold Red `!` with the message under the value column.
- **Quick capture** underlines parsed trigger tokens and recognized natural-language phrases in their field color (project cyan, priority High, due Accent, scheduled Medium, tag Green, estimate Purple, recurrence Teal) as you type (phrases that would block the capture in red), and shows a trigger guide that suggestions replace while a trigger is active. It sits at row 5 so the list stays visible.
- **Footer** is a hints row of key chips plus a full-width Panel status bar: the message on the left (with an undo countdown bar while the undo window is open) and the server state on the right. Each server state has a glyph and a word (`✓` online, `•` saved with undo, `!` unreachable or signed out, `↻` waiting for the server), so nothing relies on color.
- **Faint text, strikethrough, drop shadows and translucent backdrops are not used**; Border and Dim are explicit colors.

The default palette is Tokyo Night-inspired. Tokens are Surface (never painted, so transparent terminals work), Panel, Selection, Text, Muted, Muted-on-fill, Accent, Cyan, Red, High, Medium, Low, Green, Purple, Teal, Border and Dim. Light Muted, Cyan, Red, High, Medium, Green, Purple and Teal are darkened to clear 4.5:1 on Panel and Selection.

Lip Gloss background detection selects a dark or light variant when `theme = "auto"`. Users can override it.

`theme = "terminal"` maps every token to the terminal's 16 ANSI colors, so the user's color scheme decides the hues: Accent blue, Cyan cyan, Red red, High bright red, Medium yellow, Green green, Purple magenta, Teal bright cyan. Text keeps the default foreground, so the input cursor uses reverse video. Background detection picks the neutrals: on dark, Muted, Low, Border and Muted-on-fill are white, Panel is black, and Selection and Dim are bright black; on light, Muted, Low and Border are bright black, Panel is bright white, Selection white, Muted-on-fill black and Dim white. Dark schemes often set bright black close to the background, so it is never a dark foreground. Contrast then depends on the scheme, so the 4.5:1 guarantee does not apply.

Settings → Appearance is the second Settings section (`Tab` from Projects). `←`/`→` cycles auto, dark, light and terminal and repaints the whole app as a live preview; `Enter` saves the top-level `theme` key with the same conflict-checked, comment-preserving replacement as Projects saves, and `Esc` restores the saved theme. With an unsaved or saving draft, `Tab`, `1`–`3`, `q` and closing are refused with a status message until the draft is saved or discarded. Leaving Settings resets it to Projects.

Icon modes:

- `unicode` is the default and requires no Nerd Font.
- `nerd` is an explicit opt-in via config or `MOMENTUM_ICONS=nerd`. It swaps only icons (Font Awesome codepoints from Nerd Fonts; use a "Mono" variant); chrome such as gutters, rules and borders stays box-drawing.
- `ascii` supports restricted terminals with printable ASCII only: `[ ]`/`[x]`/`[>]` state, `>` selection, `^ = v` priority, `+ - |` chrome, and no navigation icons.

Automatic Nerd Font detection is not attempted because terminals do not reliably expose the active font.

Responsive behavior:

- **Wide+ (≥ 140 columns):** Wide plus a 44-column read-only details pane for the selected task.
- **Wide (104–139):** 24-column rail, `│` divider and a 2-column gutter; the main column starts at column 27, keeps a 2-column right margin, and has a title row and rule.
- **Compact (50–103):** a tab row with `━` under the active tab replaces the rail; two-line rows.
- **Narrow (28–49):** inactive tabs collapse to icon and count; one-line rows, no section gaps, and a single status row. Below 40 columns only the active tab shows, with `• · · ·` position dots.
- Modals are min(spec, width − 4) wide (1-cell margins below 50 columns) and scroll rather than shrink.
- Below 28 × 8 Momentum shows the current and needed size and how to quit.

Exact breakpoints should be centralized constants and adjusted during manual UAT.

## 7. CLI Surface

```text
momentum                  Launch with automatic startup view
momentum inbox            Launch in Inbox
momentum today            Launch in Today
momentum completed        Launch in Completed
momentum doctor           Run diagnostics
momentum login            Sign in to the [backend] server (backend plan T14)
momentum logout           Sign out of the [backend] server
momentum import           Copy every Taskwarrior task to the [backend] server (backend plan T16)
momentum import --dry-run Report what import would do, and import nothing
momentum --config PATH    Use another config file
momentum --debug          Enable local diagnostic logging
momentum --version        Print version
momentum --help           Show help
```

Custom filters and views are deferred.

## 8. Configuration

Location resolution:

1. `--config PATH`
2. `$XDG_CONFIG_HOME/momentum/config.toml`
3. `~/.config/momentum/config.toml`

Every setting has a default except `[backend] url`, the server's address, which the apps need; without it, `momentum` prints setup steps and exits, and the macOS app shows them.

```toml
refresh_interval = "60s"
theme = "auto"       # auto, dark, light, terminal
icons = "unicode"    # unicode, nerd, ascii

[backend]
url = "https://momentum.example.com"

[sync]
mutation_delay = "15s"   # how long u can undo the latest change

[planning]
enabled = true
daily_capacity = "8h"
buffer = "1h"
# Optional: [planning.weekday_capacity] friday = "4h"

[calendar]
enabled = false
# paths = ["~/Calendars/work.ics"]
include_all_day = false
include_transparent = false
working_start = "09:00"
working_end = "17:00"
stale_after = "24h"
```

Calendar paths are local ICS files. Momentum does not fetch remote URLs; an
external calendar client may refresh a subscription before Momentum reads it.

Environment overrides in v1:

```text
MOMENTUM_ICONS=nerd|unicode|ascii
```

Invalid configuration fails with a concise actionable message. Unknown keys should be rejected to detect typos. `[sync]`'s `enabled`, `interval`, `startup`, and `shutdown` drove Taskwarrior sync; a config that still sets one fails with a message saying so.

## 9. Architecture

### Package structure

```text
cmd/momentum/          CLI parsing, startup, version metadata
internal/app/          Root Bubble Tea model, update loop, orchestration
internal/ui/           Components, styles, responsive rendering
internal/backend/      Server client, sign-in, saved credentials, import plan
internal/taskwarrior/  Reads Taskwarrior's tasks for momentum import
internal/config/       TOML loading, XDG paths, defaults, validation
../core/domain/        Task, view, grouping, sorting, edit-diff models (shared core module)
internal/quickadd/     Trigger parser and autocomplete context
internal/diagnostics/  Doctor checks and debug logging
internal/calendar/     Read-only local ICS parsing and busy-interval merging
```

Recommended dependencies:

- Go current stable release at implementation time.
- Bubble Tea for the Elm-style terminal update loop.
- Bubbles for text input, viewport/list primitives, and spinner behavior.
- Lip Gloss for layout and styling.
- A small TOML library for strict configuration decoding.
- Prefer the standard library for CLI parsing, HTTP, logging, time, and JSON.
- Use a small fuzzy matcher only if a straightforward internal implementation is insufficient.

The implementing agent must confirm current stable Charm APIs in official documentation before pinning dependency versions.

### Runtime flow

```text
User input
  -> Bubble Tea Update
  -> pure state transition or asynchronous command
  -> backend client sends HTTPS + JSON to the Momentum server
  -> typed result message
  -> reload pending and recent completed tasks
  -> derive Inbox/Today and recent Completed sections in Go
  -> render
```

### Core interfaces

Illustrative contracts; implementation may refine names without changing responsibilities:

```go
type Client interface {
    ExportPending(ctx context.Context) ([]domain.Task, error)
    Add(ctx context.Context, input domain.NewTask) error
    Modify(ctx context.Context, uuid string, diff domain.TaskDiff) error
    Complete(ctx context.Context, uuid string) error
    Delete(ctx context.Context, uuid string) error
    Start(ctx context.Context, uuid string) error
    Stop(ctx context.Context, uuid string) error
    Undo(ctx context.Context) error
    Projects(ctx context.Context) ([]string, error)
}
```

The UI depends on this interface and uses a fake implementation in tests; `internal/backend` implements it. The explicit project migration uses a separate injected seam: it exports pending tasks for the preview, and the server applies the confirmed UUID mappings in one transaction that refuses them all if any task changed since (backend plan T12). It is not a general bulk-edit interface.

### Task data model

The domain task should include at least:

```text
UUID, ID, Description, Status, Project, Priority, Estimate,
Due, Scheduled, Start, Wait, Until, Tags, Annotations,
Dependencies, Recurrence, recurrence parent/mask metadata, Urgency, RawFields
```

- UUID is the stable selection and mutation identity.
- Optional timestamps retain absence distinctly from zero values.
- Timestamps are parsed and converted to local time for classification/display.
- Unknown JSON properties are retained when useful for details/debugging but never rewritten.
- A daily commitment is the inspectable tag `momentum-plan-YYYY-MM-DD`; planned tasks appear in a distinct Planned Today group without changing due dates.
- Recurrence keeps Taskwarrior's template-and-instance model on the server: one pending instance at a time, and the next when it is completed or deleted (ADR 0002, D5). Stopping a series ends it and keeps its pending instance.

### Asynchronous execution

- All server requests run through Bubble Tea commands and never block `Update`.
- Mutations are serialized.
- Initial load shows a subtle loading state.
- Refreshes retain old content.
- Operations use contexts and finite timeouts.
- Results return typed messages, not shared mutable callbacks.

## 10. Server

The apps keep tasks on a Momentum server ([ADR 0002](../docs/adr/0002-go-postgres-backend.md), [backend README](../backend/README.md)): JSON over HTTPS under `/v1`, with request and response types in `core/api`. Every request is scoped to the signed-in user.

- **Sign-in:** `momentum login` runs the device flow (D1): it shows a code and opens the browser, where the person signs in with Google or GitHub. The token is saved in `credentials.json` next to `config.toml`, with mode `0600`; `momentum logout` revokes it. The macOS app signs in on its own and keeps its token in the Keychain.
- **Changes:** each change carries an idempotency key, so a retried request applies once, and is on the server once it answers.
- **Dates:** the app resolves date expressions to instants in the device's time zone and sends the zone with the first change (D3).
- **Estimates** are a task field on the server; no configuration is needed.
- **Import:** `momentum import` reads `task export` with recurrence, garbage collection, and hooks off, previews what the server would create, and creates only tasks the user does not have, keeping their UUIDs (backend plan T16). Taskwarrior's data is never changed.

Project suggestions combine the server's list of projects in pending tasks with optional Momentum `[[projects]]` configuration entries, which supply a readable `name` and unique lowercase dotted `value`. Configured entries precede discovered values for an empty query, remain available without tasks or successful discovery, and supply breadcrumbs in quick capture, the project editor, and Settings → Projects. Matching uses labels and values; completion inserts only the value. Settings catalog saves take effect immediately without restart and never mutate tasks by themselves. An explicitly confirmed value migration is the narrow pending-only exception: it preserves historical values, and the server applies it in one transaction without batch undo. Tag suggestions come from the tags of pending tasks. Unknown project/tag values remain valid.

## 11. Undo and Connectivity

Nothing syncs: a change is on the server once it answers, and every device reads the same tasks.

- After a change, `u` undoes it for `[sync] mutation_delay` (15 seconds by default), unless the task changed since (D7). A new change restarts the window; an undo or a project migration closes it.
- The footer reads `✓ Online`, `• Saved · u to undo` with a countdown bar, `! Server unreachable · r to retry`, or `! Signed out · run momentum login`.
- When the server is unreachable, loaded tasks stay visible and nothing is queued (P1); `r` and the refresh timer try again.
- `q` quits immediately; there is nothing left to send.

## 12. Hosting

The server runs on Cloud Run with Neon PostgreSQL; [`backend/DEPLOY.md`](../backend/DEPLOY.md) is the deploy guide. Infrastructure is configured by hand and is not provisioned by this repository.

## 13. Dotfiles and Secret Management

The dotfiles repository may track:

```text
~/.config/momentum/config.toml
```

`credentials.json`, which holds the TUI's sign-in, must not be tracked.

## 14. Diagnostics, Privacy, and Errors

### Error handling

| Scenario | Behavior |
|---|---|
| No `[backend] url` | Exit startup with setup steps; `doctor` fails the Backend server check |
| Not signed in | Exit startup asking for `momentum login`; a refused token shows `Signed out` |
| Unexpected server answer | Keep UI safe, show the error, log redacted details |
| Mutation rejected | Preserve current view and show the server's message, on the field it names |
| Refresh failure | Keep stale tasks visible and show degraded status |
| Server unreachable | Keep loaded tasks visible, say so, and retry on `r`; nothing is queued |
| Invalid config | Exit with exact key/value validation error |
| Terminal too small | Render minimum-size message |

### Debug logging

`--debug` writes structured logs to:

1. `$XDG_STATE_HOME/momentum/momentum.log`
2. `~/.local/state/momentum/momentum.log`

Logs include state transitions and redacted error details. They should avoid task descriptions and never contain tokens or other credentials.

### Doctor

`momentum doctor` checks:

- the `[backend]` server answers, and its version
- the saved sign-in is valid there
- the device's time zone is known
- config path, syntax, and values
- calendar sources, when enabled
- terminal capabilities
- writable state/log directories

Doctor must not mutate tasks.

### Privacy

- No telemetry.
- No crash reporting.
- No automatic update checks.
- No network traffic except requests to the configured Momentum server and the browser sign-in it starts; calendar awareness reads configured local ICS files only.

## 15. Testing Strategy

- Unit tests for task JSON decoding, date conversion, Today/planned classification, grouping, deterministic sorting, edit diffs, recurrence/plan projections, configuration, quick-add interpretation, and autocomplete context.
- Calendar tests use fixture ICS files and assert overlap merging, recurrence expansion, all-day/transparency policy, stale/missing degradation, and no event persistence.
- Update-loop tests use typed messages and a fake client; backend client tests use a fake HTTP server.
- Rendering tests cover normal, narrow, very narrow, empty, loading, modal, suggestion, unreachable, and error states.
- Taskwarrior is read only by `momentum import`; its tests use isolated temporary Taskwarrior configuration/data and never touch the user's `~/.task` or `~/.taskrc`.
- Manual UAT runs on Linux and macOS with small and large terminals.
- Baseline gate: `go test ./...`.
- Additional gates: `gofmt`, `go vet ./...`, and cross-platform build checks.

## 16. Distribution

Initial development uses local Go builds. Releases support:

```sh
go install github.com/jvrviegas/momentum-task-manager/tui/cmd/momentum@latest
```

First published releases should provide binaries for:

- Linux AMD64
- Linux ARM64
- macOS AMD64
- macOS ARM64

GitHub Actions runs tests, vet, and build checks. GoReleaser builds tagged releases after the application stabilizes. A Homebrew tap is deferred.

The project uses the MIT license and should include a README, CONTRIBUTING guide, example config, screenshots, quick-add grammar, keybinding reference, and server setup notes.

## 17. Acceptance Criteria

Momentum v1 is ready when:

1. It launches against a Momentum server on Linux and macOS.
2. Inbox and Today exactly follow the agreed disjoint semantics.
3. Today correctly groups overdue, due-today, and scheduled-today tasks without duplicates.
4. Quick add safely creates tasks with all supported explicit trigger types and autocomplete, including optional estimates and native recurrence.
5. Structured edit can directly focus and change every supported field without altering unsupported fields.
6. Complete, start/stop, delete, and undo work through UUID-based server requests.
7. Search, keyboard navigation, limited mouse interaction, details, help, and responsive layouts work.
8. Refresh and server requests remain asynchronous and never freeze the update loop.
9. An unreachable server, the undo window, and quitting work as designed.
10. Without a `[backend] url`, Momentum exits with setup steps.
11. No operation reads or writes Taskwarrior's database files; `momentum import` only runs `task export`.
12. Debug logs and doctor checks do not disclose secrets.
13. Automated tests never access real user task data and `go test ./...` plus `go vet ./...` pass.
14. Documentation explains installation, configuration, controls, and the server's manual Cloud Run/Neon setup.
15. The daily ritual can be canceled/reopened, records selected commitments as inspectable tags, distinguishes obligations/rollover/candidates, reports capacity and unestimated work, and warns without blocking over-capacity plans.
16. Recurring tasks support the approved presets and interval, show template/instance metadata, keep one pending instance at a time (ADR 0002, D5), and stop without rewriting history.
17. Natural-language capture supports the documented local date/time/effort/priority/recurrence whitelist, opens review before inferred mutation, preserves prose, and retains explicit-trigger precedence.
18. Optional local ICS awareness merges overlapping in-hours events, treats all-day/transparent events by configuration, and degrades to task-only planning when unavailable or stale.
