# Momentum Design

**Status:** Implemented; see `docs/UAT.md` for acceptance evidence  
**Application:** Momentum  
**Module path:** `github.com/jvrviegas/momentum`  
**Target directory:** `/home/joaovvr/Projects/Personal/momentum`  
**Last updated:** 2026-09-16

## 1. Product Summary

Momentum is a polished, keyboard-first terminal frontend for Taskwarrior 3.x. It gives Taskwarrior a focused two-view workflow, fast structured editing, a quick-add command bar with contextual suggestions, and optional synchronization through Taskwarrior's native sync command.

Momentum is a stateless wrapper for task data. Taskwarrior remains the only source of truth for tasks and owns their persistence, recurrence, hooks, validation, contexts, synchronization, and conflict handling. Momentum may store a local project suggestion catalog in its configuration; these entries are not Taskwarrior entities and never change existing task assignments.

### Delivered extension: Settings → Projects

Momentum includes in-app catalog management and an explicit opt-in to migrate changed project values on eligible pending tasks while leaving historical values intact. The narrow exception to the bulk-operations non-goal is recorded in [ADR 0001](docs/adr/0001-project-settings-and-pending-task-renames.md); detailed [requirements](docs/spec/settings-projects.md), [implementation evidence](docs/plans/settings-projects.md), and [UAT evidence](docs/UAT.md) govern the shipped behavior. Active-context scope, child-removal prompts, duplicate rejection, catalog-first partial outcomes, no batch undo, and symlink preservation are implemented as recorded.

## 2. Goals

- Present pending tasks in two disjoint smart views: Inbox and Today.
- Make task capture fast through a command bar and compact trigger syntax.
- Make common field edits reachable with one key.
- Provide a visually polished, responsive Bubble Tea interface.
- Preserve full interoperability with the regular `task` CLI.
- Work offline and synchronize safely when configured.
- Support macOS and Linux without requiring a Nerd Font.
- Remain usable with no Momentum config and no sync configuration.

## 3. Non-goals for v1

- Replacing Taskwarrior's storage or synchronization protocol.
- Writing directly to `~/.task` or any Taskwarrior database file.
- Custom or user-defined views.
- A Kanban board.
- General bulk task operations. The explicit Settings → Projects pending-only value migration is the narrow documented exception; it is not a general bulk editor.
- Editing annotations, dependencies, recurrence, or arbitrary UDAs.
- Changing Taskwarrior contexts from Momentum.
- Desktop notifications or background execution while Momentum is closed.
- Native Windows support.
- Automatic update checks, telemetry, or crash reporting.
- A Cloud Run/Neon provisioning tool or Terraform configuration.
- Fully configurable keybindings.

## 4. Functional Requirements

### Views

- **VIEW-01:** Inbox contains pending tasks visible in the active Taskwarrior context that are not overdue, due today, or scheduled today.
- **VIEW-02:** Today contains unique pending tasks that are overdue, due today, or scheduled today.
- **VIEW-03:** Today groups tasks in this order: Overdue, Due Today, Scheduled Today.
- **VIEW-04:** A task matching more than one Today group appears once in its highest-priority group.
- **VIEW-05:** Inbox sorts by Taskwarrior urgency descending.
- **VIEW-06:** Today sorts by group and then Taskwarrior urgency descending within each group.
- **VIEW-07:** Date classification uses the machine's local calendar date.
- **VIEW-08:** Startup opens Today when it is non-empty and Inbox otherwise.
- **VIEW-09:** Sidebar counts show the unique number of tasks in each view.
- **VIEW-10:** Selection is preserved by task UUID across refreshes when possible.

Today classification precedence:

1. `due` local date is before today: Overdue.
2. `due` local date equals today: Due Today.
3. `scheduled` local date equals today: Scheduled Today.
4. Otherwise the task is not in Today.

A scheduled date before today does not make a task overdue. Overdue is based on `due` only.

### Task rows

- **ROW-01:** A row shows completion state, description, project badge, relevant due/scheduled value, priority badge, and active-task indicator.
- **ROW-02:** UUID, numeric ID, tags, raw urgency, recurrence, dependencies, and annotations are hidden from the default row.
- **ROW-03:** Color is never the only indication of state.
- **ROW-04:** Selected rows use a full-row background treatment.

Example:

```text
□ Finish API proposal    #work.client    Today 17:00    H
▶ Review pull request    #work
```

### Quick add

- **ADD-01:** `Ctrl+K` opens a bottom command bar.
- **ADD-02:** Plain text becomes the task description.
- **ADD-03:** Metadata triggers activate only at the start of a whitespace-delimited token.
- **ADD-04:** A leading backslash escapes a trigger and is removed from the final description.
- **ADD-05:** Commands are converted to an argument vector and never executed through a shell.
- **ADD-06:** Taskwarrior performs final date and mutation validation.

Trigger grammar:

| Trigger | Field | Examples | Taskwarrior translation |
|---|---|---|---|
| `#` | Project | `#work.client` | `project:work.client` |
| `!` | Priority | `!high`, `!medium`, `!low`, `!none` | `priority:H/M/L/` |
| `@` | Due | `@today`, `@tomorrow`, `@2026-09-20` | `due:<value>` |
| `>` | Scheduled | `>today`, `>monday` | `scheduled:<value>` |
| `+` | Tag | `+planning` | `+planning` |

Rules:

- One project token is permitted.
- One priority, due, and scheduled token is permitted; the parser rejects duplicate scalar metadata with a useful error.
- Multiple unique tags are permitted.
- Unknown project and tag values are accepted, allowing inline creation.
- The description must not be empty after metadata extraction.
- Email addresses such as `bob@example.com` remain description text because `@` is not at a token boundary.
- `\#launch` becomes literal description text `#launch`.

Autocomplete:

- Suggestions appear above the command bar.
- `Up`/`Down` and `Ctrl+P`/`Ctrl+N` move through suggestions.
- `Tab` accepts the highlighted suggestion.
- `Enter` submits the whole task.
- First `Esc` dismisses suggestions; a second `Esc` closes quick add.
- Projects and tags use fuzzy matching.
- Priority uses a fixed list.
- Due and scheduled suggestions include today, tomorrow, upcoming weekdays, next-week, and ISO-date guidance.
- Any Taskwarrior-compatible date expression may be submitted even if it was not suggested.

### Structured editing

- **EDIT-01:** `e` opens a structured modal focused on Description.
- **EDIT-02:** Direct task-list shortcuts open the same modal focused on a specific field.
- **EDIT-03:** `Up`/`Down`, `Tab`, and `Shift+Tab` move between fields.
- **EDIT-04:** When a field suggestion menu is open, `Up`/`Down` navigate suggestions; closing or accepting it restores field navigation.
- **EDIT-05:** `Ctrl+S` saves only changed supported fields.
- **EDIT-06:** `Esc` cancels without mutation.
- **EDIT-07:** Unsupported exported fields remain untouched.

Fields and direct shortcuts:

| Key | Initial field |
|---|---|
| `e` | Description |
| `p` | Project |
| `!` | Priority |
| `D` | Due |
| `S` | Scheduled |
| `t` | Tags |

Editable fields are Description, Project, Priority, Due, Scheduled, and Tags. Project/tag fields autocomplete, priority uses a selection list, and date fields show existing timestamps in local `YYYY-MM-DD HH:MM` form while still accepting Taskwarrior expressions. On a date field, `Ctrl+Left`/`Ctrl+Right` step one calendar day and `Ctrl+Up`/`Ctrl+Down` step 30 minutes. Removing a field value clears it through a Taskwarrior `modify` command.

### Details

- **DETAIL-01:** `Enter` opens a centered, primarily read-only details modal.
- **DETAIL-02:** Details include all available supported information: description, status, project, priority, dates, tags, annotations, dependencies, recurrence, urgency, UUID, and unknown exported properties where practical.
- **DETAIL-03:** `e` moves from details to the structured edit modal.
- **DETAIL-04:** `Esc` or `Enter` closes details.

### Mutations

- **MUT-01:** `Space` completes the selected task immediately.
- **MUT-02:** `s` starts or stops the selected task.
- **MUT-03:** `d` requests deletion and requires `y/n` confirmation.
- **MUT-04:** `u` invokes Taskwarrior undo while undo remains available.
- **MUT-05:** Only one mutation runs at a time.
- **MUT-06:** Every successful mutation is followed by a fresh Taskwarrior export.
- **MUT-07:** Taskwarrior hooks run normally.
- **MUT-08:** Momentum owns interactive confirmations and invokes Taskwarrior non-interactively afterward.
- **MUT-09:** Success and failure messages appear inside the TUI.
- **MUT-10:** After completion or deletion, selection moves predictably to the next task, then the previous task if no next task exists.

### Search

- **SEARCH-01:** `/` opens a local fuzzy filter for the current view.
- **SEARCH-02:** Search covers description, project, and tags.
- **SEARCH-03:** Search filters the in-memory view and does not invoke Taskwarrior.
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

Inbox:

```text
No pending tasks
c or Ctrl+K to capture something
```

Today:

```text
Nothing scheduled for today
Your day is clear.
Ctrl+K to add a task
```

## 5. Keyboard and Mouse Map

| Input | Action |
|---|---|
| `j` / `k`, `Down` / `Up` | Move task selection |
| `h` / `l`, `Tab` | Switch sidebar/list focus |
| `g` / `G` | First/last task |
| `1` / `2` / `3` | Inbox/Today/Settings → Projects |
| `Enter` | Open/close details as context permits |
| `Ctrl+K` | Quick add |
| `c` / `Ctrl+K` | Create a task with quick capture |
| `/` | Search current view |
| `Space` | Complete task |
| `e`, `p`, `!`, `D`, `S`, `t` | Structured edit with initial field |
| `s` | Start/stop task |
| `d` | Delete confirmation |
| `u` | Taskwarrior undo |
| `r` | Refresh local tasks |
| `Ctrl+R` | Synchronize now |
| `?` | Help |
| `q` | Quit or open unsynced-changes prompt |
| `Esc` | Close/cancel the active overlay |

Mouse support is intentionally limited:

- Click sidebar views.
- Click tasks to select them.
- Mouse wheel scrolls the task list.
- No drag-and-drop or hover-only actions.

Keybindings are fixed in v1. The help screen must be generated from the application's keybinding definitions so it cannot drift from behavior.

## 6. Visual Design

Momentum uses a minimal task-app aesthetic instead of a dense terminal table:

- Borderless sidebar with highlighted active view.
- Flexible task list with generous horizontal spacing.
- Subtle modal and panel borders.
- Colored project badges.
- Restrained priority colors.
- Dimmed secondary metadata.
- Full-row selection background.
- No visible raw urgency score in rows.

The default palette is Tokyo Night-inspired:

- dark navy surfaces
- blue selection
- cyan project badges
- red overdue state
- orange high priority
- muted slate metadata

Lip Gloss background detection selects a dark or light variant when `theme = "auto"`. Users can override it.

Icon modes:

- `unicode` is the default and requires no Nerd Font.
- `nerd` is an explicit opt-in via config or `MOMENTUM_ICONS=nerd`.
- `ascii` supports restricted terminals.

Automatic Nerd Font detection is not attempted because terminals do not reliably expose the active font.

Responsive behavior:

- At normal widths, render a fixed-width sidebar and flexible task list.
- Below approximately 80 columns, replace the sidebar with a compact top tab bar.
- Below approximately 50 columns, hide nonessential row metadata before truncating descriptions.
- Modals size relative to the terminal.
- Show a minimum-size warning only when no usable layout can be rendered.

Exact breakpoints should be centralized constants and adjusted during manual UAT.

## 7. CLI Surface

```text
momentum                  Launch with automatic startup view
momentum inbox            Launch in Inbox
momentum today            Launch in Today
momentum doctor           Run diagnostics
momentum --config PATH    Use another config file
momentum --debug          Enable local diagnostic logging
momentum --version        Print version
momentum --help           Show help
```

Arbitrary Taskwarrior filters and custom views are deferred.

## 8. Configuration

Location resolution:

1. `--config PATH`
2. `$XDG_CONFIG_HOME/momentum/config.toml`
3. `~/.config/momentum/config.toml`

Momentum works with defaults when the file does not exist.

```toml
refresh_interval = "60s"
theme = "auto"       # auto, dark, light
icons = "unicode"    # unicode, nerd, ascii

[sync]
enabled = true
interval = "5m"
mutation_delay = "15s"
startup = true
shutdown = true
```

Environment overrides in v1:

```text
MOMENTUM_ICONS=nerd|unicode|ascii
```

Invalid configuration fails with a concise actionable message. Unknown keys should be rejected to detect typos.

## 9. Architecture

### Package structure

```text
cmd/momentum/          CLI parsing, startup, version metadata
internal/app/          Root Bubble Tea model, update loop, orchestration
internal/ui/           Components, styles, responsive rendering
internal/taskwarrior/  Process adapter, JSON decoding, mutations, sync
internal/config/       TOML loading, XDG paths, defaults, validation
internal/domain/       Task, view, grouping, sorting, edit-diff models
internal/quickadd/     Trigger parser and autocomplete context
internal/diagnostics/  Doctor checks and debug logging
```

Recommended dependencies:

- Go current stable release at implementation time.
- Bubble Tea for the Elm-style terminal update loop.
- Bubbles for text input, viewport/list primitives, and spinner behavior.
- Lip Gloss for layout and styling.
- A small TOML library for strict configuration decoding.
- Prefer the standard library for CLI parsing, process execution, logging, time, and JSON.
- Use a small fuzzy matcher only if a straightforward internal implementation is insufficient.

The implementing agent must confirm current stable Charm APIs in official documentation before pinning dependency versions.

### Runtime flow

```text
User input
  -> Bubble Tea Update
  -> pure state transition or asynchronous command
  -> Taskwarrior adapter executes `task` with argv (never a shell)
  -> typed result message
  -> refresh from `task ... export`
  -> derive Inbox/Today in Go
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
    Sync(ctx context.Context) (SyncResult, error)
    Projects(ctx context.Context) ([]string, error)
}
```

The UI depends on this interface and uses a fake implementation in tests. The explicit project migration uses a separate injected seam: it captures the active context read filter, exports `status:pending recur.none:`, applies one guarded UUID assignment with `project.is:<old>`, and reconciles the UUID afterward. It is not a general bulk-edit interface.

### Task data model

The domain task should include at least:

```text
UUID, ID, Description, Status, Project, Priority,
Due, Scheduled, Start, Wait, Tags, Annotations,
Dependencies, Recurrence, Urgency, RawFields
```

- UUID is the stable selection and mutation identity.
- Optional timestamps retain absence distinctly from zero values.
- Export timestamps are parsed and converted to local time for classification/display.
- Unknown JSON properties are retained when useful for details/debugging but never rewritten.
- Numeric task IDs are display-only and never used for mutations.

### Asynchronous execution

- All subprocesses run through Bubble Tea commands and never block `Update`.
- Mutations are serialized.
- Initial load shows a subtle loading state.
- Refreshes retain old content.
- Operations use contexts and finite timeouts.
- Results return typed messages, not shared mutable callbacks.

## 10. Taskwarrior Integration

Momentum targets Taskwarrior 3.x on macOS and Linux. Compatibility with 2.6 may be retained when inexpensive but is not guaranteed.

Read path:

```sh
task status:pending export
```

Momentum computes disjoint Inbox/Today views locally from one consistent export. Taskwarrior 3.x deliberately leaves machine-readable `export` unencumbered by the active context, so a context-scoped operation must explicitly read `_get rc.context` and `_get rc.context.<name>.read`, then add that filter to its export/modify argv. The existing generic export path must not be treated as context-safe for the pending-only migration; this correction is tracked in the [Settings → Projects plan](docs/plans/settings-projects.md#t00-feasibility-record--2026-09-16).

Mutation examples:

```sh
task add "Prepare proposal" project:work.client due:tomorrow +planning priority:H
task <uuid> modify project:work.client
task <uuid> done
task <uuid> start
task <uuid> stop
task <uuid> delete
task undo
task sync
```

Actual invocation uses `exec.CommandContext` with separate arguments. It must never use `sh -c` or interpolate user text into a command string.

The adapter must:

- force machine-readable output where applicable
- suppress Taskwarrior's interactive confirmation only after Momentum has confirmed
- capture stdout, stderr, exit code, and duration
- preserve hooks and Taskwarrior validation
- redact secrets and task descriptions from debug logs where practical
- return typed errors suitable for concise UI display and detailed logs

Project suggestions use Taskwarrior's script-oriented project listing when supported and supplement it with projects found in the pending export. Optional Momentum `[[projects]]` configuration entries supply a readable `name` and unique lowercase dotted `value`. Configured entries precede discovered values for an empty query, remain available without tasks or successful discovery, and supply breadcrumbs in quick capture, the project editor, and Settings → Projects. Matching uses labels and values; completion inserts only the Taskwarrior value. Settings catalog saves take effect immediately without restart and never mutate tasks by themselves. An explicitly confirmed value migration is the narrow pending-only exception: it uses active context scope, preserves historical values, and reports partial results without batch undo. Tag suggestions should come from actual exported user tags to avoid suggesting Taskwarrior virtual tags. Unknown project/tag values remain valid.

## 11. Synchronization Design

Momentum does not implement sync. It invokes `task sync` and relies on TaskChampion for merge behavior.

Taskwarrior owns these settings:

```text
sync.server.url
sync.server.client_id
sync.encryption_secret
```

Momentum owns only sync timing and UI state.

### Sync behavior

- Load local tasks before startup sync so startup is not blocked by network cold starts.
- Start sync asynchronously after local data is visible.
- Refresh tasks after successful sync.
- Sync every five minutes while Momentum is running.
- Do not install systemd timers, LaunchAgents, or any background service.
- Continue all local task operations while offline.
- `Ctrl+R` triggers immediate sync.
- Failed sync retries after 15s, 30s, 1m, 2m, then every 5m.
- Successful sync resets retry backoff.

### Undo grace period

Taskwarrior cannot undo changes once they have been synchronized. Therefore:

- After a mutation, mark local changes unsynced.
- Delay sync for 15 seconds after the latest mutation.
- Reset the delay after each new mutation.
- Show a countdown such as `Syncing in 12s · u to undo`.
- `u` invokes Taskwarrior undo during the grace period and reloads tasks.
- Manual sync ends the undo window immediately.
- After successful sync, disable undo and report `Changes synced`.

### Shutdown

When no app-originated unsynced changes exist, `q` quits immediately. Otherwise display:

```text
Unsynced changes
[s] Sync and quit
[q] Quit without syncing
[Esc] Cancel
```

Sync-and-quit has a finite timeout. Quitting without sync leaves changes in Taskwarrior and uploads them on the next Momentum launch. Forced process termination cannot guarantee sync.

### Local-only behavior

When Taskwarrior sync is not configured:

- Momentum remains fully functional.
- Footer shows `Local only`.
- `momentum doctor` reports missing settings without printing secret values.
- `[sync].enabled = false` disables checks and suppresses the status warning.

## 12. Self-hosted Sync Deployment Notes

Infrastructure is configured manually and is not part of the Momentum repository.

Chosen topology:

```text
macOS Taskwarrior ----\
                       -> Cloud Run TaskChampion Sync Server -> Neon PostgreSQL
Linux Taskwarrior ----/
```

Decisions:

- Use the official TaskChampion PostgreSQL container image, pinned to an explicit release tag.
- Use Cloud Run's generated `run.app` HTTPS URL.
- Cloud Run permits unauthenticated HTTP access because Taskwarrior does not automatically mint Google IAM tokens.
- Restrict TaskChampion to one high-entropy client UUID.
- Disable automatic client creation after provisioning, or pre-create the allowed client row in Neon.
- Store the Neon direct TLS connection string in Google Secret Manager.
- Prefer Neon's direct endpoint over its pooler because TaskChampion already pools connections and uses serializable transactions.
- Configure Cloud Run with minimum instances `0` and maximum instances `1` for this two-device workload.
- Expect occasional combined Cloud Run and Neon cold-start latency.
- Apply the `postgres/schema.sql` matching the deployed TaskChampion server release before startup.
- Keep the Taskwarrior encryption secret only on clients; it is never sent to or stored by the server.

Both machines share:

```text
sync.server.url          same Cloud Run URL
sync.server.client_id    same generated UUID
sync.encryption_secret   same random 256-bit secret
```

Linux is the designated recurrence primary:

```sh
task config recurrence on
```

macOS must use:

```sh
task config recurrence off
```

Initial provisioning starts on Linux with an empty collection, performs the first sync, then configures macOS as a second empty replica. Never copy `~/.task` between machines or synchronize it with rsync/Syncthing.

## 13. Dotfiles and Secret Management

Preserve Taskwarrior's current default paths:

```text
~/.taskrc   configuration
~/.task     task data
```

The dotfiles repository may track a common `.taskrc` containing non-secret preferences and:

```text
include ~/.config/taskwarrior/secrets.rc
```

The untracked `secrets.rc` contains the sync URL, client ID, and encryption secret. It must be transferred through a password manager and created with mode `0600`.

The dotfiles repository may also track:

```text
~/.config/momentum/config.toml
```

A non-secret bootstrap script may prompt without echo, create `secrets.rc`, set permissions, run `task sync`, and verify configuration without printing secrets.

## 14. Diagnostics, Privacy, and Errors

### Error handling

| Scenario | Behavior |
|---|---|
| `task` missing | Exit startup with installation guidance; `doctor` fails the check |
| Unsupported export JSON | Keep UI safe, show parse error, log redacted details |
| Mutation rejected | Preserve current view and show Taskwarrior stderr concisely |
| Hook failure | Surface hook error; do not pretend mutation succeeded |
| Refresh failure | Keep stale tasks visible and show degraded status |
| Sync unavailable | Continue offline and apply retry backoff |
| Invalid config | Exit with exact key/value validation error |
| Terminal too small | Render minimum-size message |

### Debug logging

`--debug` writes structured logs to:

1. `$XDG_STATE_HOME/momentum/momentum.log`
2. `~/.local/state/momentum/momentum.log`

Logs include command kind, duration, exit status, state transitions, and redacted error details. They should avoid task descriptions and never contain Taskwarrior sync secrets or database credentials.

### Doctor

`momentum doctor` checks:

- Taskwarrior executable and version
- export command and JSON decoding
- config path, syntax, and values
- sync setting presence without printing values
- a safe sync readiness check where possible
- terminal capabilities
- writable state/log directories

Doctor must not mutate tasks or run an irreversible sync unless explicitly documented and confirmed. Prefer configuration validation over active synchronization.

### Privacy

- No telemetry.
- No crash reporting.
- No automatic update checks.
- No application network traffic except Taskwarrior sync initiated through `task sync`.

## 15. Testing Strategy

- Unit tests for task JSON decoding, date conversion, Today classification, grouping, deterministic sorting, edit diffs, configuration, quick-add parsing, and autocomplete context.
- Update-loop tests use typed messages and a fake Taskwarrior client.
- Rendering tests cover normal, narrow, very narrow, empty, loading, modal, suggestion, offline, and error states.
- Integration tests use isolated temporary Taskwarrior configuration/data and never touch the user's `~/.task` or `~/.taskrc`.
- Manual UAT runs on Linux and macOS with small and large terminals.
- Baseline gate: `go test ./...`.
- Additional gates: `gofmt`, `go vet ./...`, and cross-platform build checks.

## 16. Distribution

Initial development uses local Go builds. Releases support:

```sh
go install github.com/jvrviegas/momentum/cmd/momentum@latest
```

First published releases should provide binaries for:

- Linux AMD64
- Linux ARM64
- macOS AMD64
- macOS ARM64

GitHub Actions runs tests, vet, and build checks. GoReleaser builds tagged releases after the application stabilizes. A Homebrew tap is deferred.

The project uses the MIT license and should include a README, CONTRIBUTING guide, example config, screenshots, quick-add grammar, keybinding reference, and sync setup notes.

## 17. Acceptance Criteria

Momentum v1 is ready when:

1. It launches against Taskwarrior 3.x on Linux and macOS.
2. Inbox and Today exactly follow the agreed disjoint semantics.
3. Today correctly groups overdue, due-today, and scheduled-today tasks without duplicates.
4. Quick add safely creates tasks with all five trigger types and autocomplete.
5. Structured edit can directly focus and change every supported field without altering unsupported fields.
6. Complete, start/stop, delete, and undo work through UUID-based Taskwarrior commands.
7. Search, keyboard navigation, limited mouse interaction, details, help, and responsive layouts work.
8. Refresh and synchronization remain asynchronous and never freeze the update loop.
9. Offline operation, retry backoff, undo delay, and unsynced-quit behavior work as designed.
10. Local-only mode works without sync configuration.
11. No operation reads or writes Taskwarrior's database files directly.
12. Debug logs and doctor checks do not disclose secrets.
13. Automated tests never access real user task data and `go test ./...` plus `go vet ./...` pass.
14. Documentation explains installation, configuration, controls, and manual Cloud Run/Neon synchronization setup.
