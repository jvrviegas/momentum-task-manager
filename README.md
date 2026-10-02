# Momentum

Momentum is a keyboard-first terminal frontend for [Taskwarrior 3.x](https://taskwarrior.org/). Taskwarrior remains the only source of truth: Momentum reads `task ... export`, sends normal `task` commands, and derives Inbox/Today in memory.

## Requirements

- Go 1.27 or a compatible current Go toolchain for source builds
- Taskwarrior 3.x on macOS or Linux
- A terminal with UTF-8 support (no Nerd Font is required)

## Install

Install the latest published command:

```sh
go install github.com/jvrviegas/momentum-task-manager/cmd/momentum@latest
```

Or build locally:

```sh
go build -o momentum ./cmd/momentum
```

Run the automatic startup view, or choose a view explicitly:

```sh
momentum
momentum inbox
momentum today
momentum completed
momentum doctor
momentum --config PATH
momentum --debug
momentum --version
```

Momentum never writes Taskwarrior data files directly. It invokes `task` with an argument vector, so task descriptions and metadata are never passed through a shell.

## Optional task estimates

Estimates are stored in Taskwarrior as a duration UDA. Before using `~estimate` or the Estimate editor, add this definition to `~/.taskrc` on every device that edits or synchronizes the tasks:

```text
uda.estimate.type=duration
uda.estimate.label=Estimate
```

Momentum never installs or changes this configuration. Estimate values sync as task data, but the UDA definition is per-device; `momentum doctor` reports whether the active client is ready and estimate writes are blocked when it is missing or has another type.

## Views

- **Inbox** contains pending tasks in the active Taskwarrior context that are not currently classified into Today; context-scoped machine-readable exports must apply that context filter explicitly (see the [Settings → Projects feasibility record](docs/plans/settings-projects.md#t00-feasibility-record--2026-09-16)).
- **Today** contains unique pending tasks classified as Overdue, Due Today, Scheduled Today, or Planned Today, in that order. Tasks in Today are excluded from Inbox.
- **Completed** contains context-scoped tasks completed in the last 30 local calendar days, grouped into Today, Yesterday, and Earlier and sorted newest first. Completed tasks are read-only.
- Press `P` to open the daily planning ritual. Obligations, rollover work, and voluntary candidates stay distinct; confirming uses inspectable `momentum-plan-YYYY-MM-DD` Taskwarrior tags and never changes due dates.
- Inbox groups tasks by project alphabetically, with oldest-created tasks first within each group. Unassigned tasks appear under **No project** at the end; missing creation dates sort last within a group.
- Today rows sort by Taskwarrior urgency; raw urgency is not shown in the normal row.

## Quick add

Press `Ctrl+K`, type a description, and press Enter. Metadata triggers work at token boundaries:

```text
Prepare proposal tomorrow at 3pm, p1, every Friday, about 1h #work.client
```

| Trigger | Field | Example |
|---|---|---|
| `#` | project | `#work.client` |
| `!` | priority | `!high`, `!medium`, `!low`, `!none` |
| `@` | due | `@today`, `@tomorrow`, `@2026-09-20` |
| `>` | scheduled | `>today`, `>monday` |
| `+` | tag | `+planning` |
| `~` | estimate | `~15m`, `~1h`, `~1h30m` |
| `^` | recurrence | `^daily`, `^weekdays`, `^weekly`, `^monthly`, `^2wks` |

Use a leading backslash for a literal trigger, for example `\#launch` or `\~1h`. Email addresses remain description text. Estimates accept positive whole minutes from `1m` through `24h`; accepted forms include `15m`, `90m`, `1h`, `1.5h`, and `1h30m`. Momentum displays them as `15m`, `1h`, or `1h 30m` and clears one by removing the field in the editor. Outgoing Taskwarrior values use `<minutes>min` because Taskwarrior's `m` suffix means months. Precise external values over 24 hours remain readable; unsupported calendar durations appear as a details warning. Recurrence uses Taskwarrior's native template/instance behavior: natural-language forms such as `every Friday`, `every weekday`, and `every 2 weeks` require review before submission; explicit `^` forms retain fast capture. `R` edits a recurrence; `X` or `x` in details stops a series by expiring its template, preserving generated and completed history. Projects and tags may be new; Taskwarrior performs final validation.

## Controls

| Key | Action |
|---|---|
| `j`/`k`, arrows | move selection |
| `h`/`l`, `Tab` | switch focus |
| `g`/`G` | first/last task |
| `1`/`2`/`3`/`4` | Inbox/Today/Completed/Settings → Projects |
| `Enter` | details |
| `c` / `Ctrl+K` | create a task with quick capture |
| `/` | local fuzzy search; use `project:name` for an exact project or `project:none` for unassigned tasks |
| `Space` | complete |
| `e`, `E`, `R`, `p`, `!`, `D`, `S`, `t` | edit Description, Estimate, Recurrence, Project, Priority, Due, Scheduled, Tags |
| `P` | open the daily planning ritual |
| `X` | stop the selected recurrence template |
| `s` | start/stop |
| `d` | delete, then confirm `y`/`n` |
| `u` | undo during the sync grace period |
| `r` | refresh |
| `Ctrl+R` | sync now |
| `?` | help |
| `q` | quit or choose how to handle unsynced changes |
| `Esc` | close/cancel the active overlay |

Mouse support is limited to view/task clicks and task-list wheel scrolling.

## Configuration

Momentum uses `$XDG_CONFIG_HOME/momentum/config.toml`, or `~/.config/momentum/config.toml`. `--config PATH` wins. Missing configuration uses safe defaults. Unknown keys are errors.

Copy [`config.example.toml`](./config.example.toml) to get started. `MOMENTUM_ICONS=unicode|nerd|ascii` overrides the icon setting. `0s` disables background task refresh.

Themes are `auto`, `dark`, and `light`; icons are `unicode`, `nerd`, and `ascii`.

### Daily planning and calendar awareness

`[planning]` defaults to 8 hours of focus capacity with a 1-hour buffer. Set `daily_capacity`, `buffer`, and optional `weekday_capacity` overrides in Momentum config. The ritual counts estimates and reports unestimated work separately; exceeding capacity is a warning, never a block. Canceling makes no task mutation.

Calendar awareness is optional and currently supports local, read-only ICS files only. Configure `[calendar].enabled = true` and `paths`, plus working hours. Multiple files are merged, overlapping events are counted once, all-day and transparent events are configurable, and stale/missing files leave task-only planning usable. Momentum never fetches, edits, or persists event details; subscriptions must be refreshed by an external tool. See the [daily planning](docs/plans/daily-planning.md), [recurrence](docs/plans/recurring-tasks.md), and [calendar](docs/plans/calendar-awareness.md) implementation records.
### Project suggestion catalog

Store frequently used projects in your Momentum config, even before they have tasks:

```toml
[[projects]]
name = "Work"
value = "work"

[[projects]]
name = "Client"
value = "work.client"
```

Each entry has a readable `name` and a unique Taskwarrior `value`. Values use lowercase letters/numbers, hyphens, and dots for hierarchy. Parent entries are optional and selectable. The example displays **Work → Client** and inserts `#work.client`; neither spaces nor display labels are sent as project values.

- Type `#` in quick capture to see suggestions; type part of a name or value to filter, use arrows to select, and Tab to complete.
- Configured projects appear first for an empty query and merge with discovered Taskwarrior projects without duplicate values. They are also available in the project editor.
- The catalog is available even with no tasks or when project discovery fails. It is local to Momentum, not restricted by Taskwarrior contexts or synchronized by `task sync`.
- Use **Settings → Projects** to add, edit, reparent, or remove entries; successful saves update suggestions immediately and persist to the active config path. Manual TOML edits still take effect on restart. Display-name edits affect suggestions **only**, never existing tasks. Discovered projects can still appear after removal from the catalog.
- Unknown project values remain valid in quick capture. New task assignments are not automatically saved to the catalog.

Settings → Projects is a catalog-management screen, not a separate project database; the catalog still lives entirely in configuration. Value edits default to Catalog only. The user may explicitly preview and confirm a pending-only migration in the active Taskwarrior context, optionally including dotted descendants; completed, deleted, waiting, recurring, and historical values are retained. Config and task outcomes are reported separately, with no batch undo or automatic rollback. See the [implementation tracker](docs/plans/settings-projects.md), [requirements](docs/spec/settings-projects.md), [UAT evidence](docs/UAT.md), and [decision record](docs/adr/0001-project-settings-and-pending-task-renames.md).

## Synchronization

Momentum does not implement or provision synchronization. It runs Taskwarrior's native `task sync` when Taskwarrior sync settings are present. Local work remains available when sync is unavailable. After a mutation, sync waits 15 seconds and shows an undo grace countdown. Daily-plan tags and native recurrence data participate in the same refresh/sync/undo lifecycle. See [`docs/sync.md`](./docs/sync.md) for the manual TaskChampion/Cloud Run/Neon setup.

For local-only use, set `[sync].enabled = false`. Local-only tasks still support `u` to undo the latest in-app mutation; with no sync timer, that undo remains available until another mutation or exit.

## Dotfiles and privacy

A shared `.taskrc` may include a non-secret file:

```text
include ~/.config/taskwarrior/secrets.rc
```

Keep `secrets.rc` untracked, create it with mode `0600`, and transfer it through a password manager. Never commit a sync URL, client UUID, encryption secret, or database credential. Momentum has no telemetry, crash reporting, automatic update checks, or network activity outside a user-initiated/native Taskwarrior sync.

`momentum doctor` checks availability, export decoding, configuration, sync readiness, Estimate UDA readiness, terminal support, and writable state paths without mutating tasks or printing secret values. `--debug` writes redacted structured logs under the XDG state directory.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/momentum
```

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for isolation and release rules.
