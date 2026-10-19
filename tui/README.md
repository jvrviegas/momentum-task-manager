# Momentum

Momentum is a keyboard-first terminal task manager. Tasks live on a Momentum server ([ADR 0002](../docs/adr/0002-go-postgres-backend.md)) that the TUI and the [macOS app](../macos/README.md) share; Momentum derives Inbox/Today in memory from what it loads.

## Requirements

- Go 1.27 or a compatible current Go toolchain for source builds
- A Momentum server and an account on it ([`backend/`](../backend/README.md))
- macOS or Linux, and a terminal with UTF-8 support (no Nerd Font is required)
- Taskwarrior 3.x only to bring existing tasks over with `momentum import`

## Install

Install the latest published command:

```sh
go install github.com/jvrviegas/momentum-task-manager/tui/cmd/momentum@latest
```

Or build locally from `tui/`:

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
momentum login
momentum logout
momentum import [--dry-run]
momentum --config PATH
momentum --debug
momentum --version
```

## Server and sign-in

Name the server in the config:

```toml
[backend]
url = "https://momentum.example"
```

The URL is the server's address, without a path, query, or credentials, over HTTPS, or over HTTP to `localhost` or a loopback address. Without it, `momentum` prints these steps and exits.

`momentum login` signs in: it shows a code, opens the browser, and waits while you enter the code and sign in with Google or GitHub. The sign-in lasts 90 days and is saved in `credentials.json` in the default config directory (`$XDG_CONFIG_HOME/momentum` or `~/.config/momentum`, whatever `--config` says), readable only by you. Signing in again replaces it and revokes the old one. `momentum logout` revokes the sign-in on the server and deletes the file; when the server cannot revoke it, the file is still deleted, and the token keeps working until it expires. The macOS app does not use this file: it signs in on its own and keeps its sign-in in the Keychain (see [`macos/README.md`](../macos/README.md#server-and-sign-in)).

- A change is saved once the server answers. `u` undoes the latest change for `[sync].mutation_delay` (15 seconds by default), unless the task changed since.
- The status bar shows `✓ Online`, `• Saved · u to undo`, `! Server unreachable · r to retry`, or `! Signed out · run momentum login`. Nothing is queued while the server is unreachable: press `r` to load again, and repeat a change once it answers.
- Dates are resolved on this device, in its time zone; expressions Momentum does not support are refused instead of sent. There are no contexts or hooks, and a project migration moves all of its tasks or none.

## Bringing tasks over from Taskwarrior

`momentum import` copies your Taskwarrior tasks to the server: every status, recurring series with their instances, UUIDs, timestamps, annotations, dependencies, and any field of your own, which the server keeps unchanged. It reads `task export` with garbage collection, recurrence, and hooks turned off, so Taskwarrior's data is never changed, then reports what it found and how many tasks the server already has. It only creates tasks: one already on the server is left as it is, even if it differs, so running it again changes nothing. `momentum import --dry-run` stops after the report. A series repeating by a rule Momentum does not support stops the import before anything is sent; change it in Taskwarrior first. An import cannot be undone with `u`. This is the only command that runs `task`.

## Views

- **Inbox** contains pending tasks that are not currently classified into Today.
- **Today** contains unique pending tasks classified as Overdue, Due Today, Scheduled Today, or Planned Today, in that order. Tasks in Today are excluded from Inbox.
- **Completed** contains tasks completed in the last 30 local calendar days, grouped into Today, Yesterday, and Earlier and sorted newest first. Completed tasks are read-only.
- Press `P` to open the daily planning ritual. Obligations, rollover work, and voluntary candidates stay distinct; confirming uses inspectable `momentum-plan-YYYY-MM-DD` tags and never changes due dates.
- Inbox groups tasks by project alphabetically, with oldest-created tasks first within each group. Unassigned tasks appear under **No project** at the end; missing creation dates sort last within a group.
- Today rows sort by urgency, Taskwarrior's default formula; raw urgency is not shown in the normal row.

## Quick add

Press `Ctrl+K`, type a description, and press Enter. In both quick capture and task editing, `Enter` submits the current input (`Ctrl+S` also works), arrows select suggestions, and `Tab` accepts a visible suggestion. In the editor, `Tab` otherwise moves to the next field; after accepting a suggestion, press `Tab` again to move on. `Shift+Tab` always moves to the previous field. Capture correction review also uses `Enter` to confirm after validation. Metadata triggers work at token boundaries:

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

Use a leading backslash for a literal trigger, for example `\#launch` or `\~1h`. Recognized natural-language phrases are highlighted while you type; press `Esc` with the cursor on or right after one (for example `tomorrow`) to keep it as plain text. Email addresses remain description text. Estimates accept positive whole minutes from `1m` through `24h`; accepted forms include `15m`, `90m`, `1h`, `1.5h`, and `1h30m`. Momentum displays them as `15m`, `1h`, or `1h 30m`, opens them for editing in the compact form (`1h30m`), and clears one by removing the field in the editor. Imported values over 24 hours remain readable; unsupported calendar durations appear as a details warning. A recurring series is a template with one pending instance; the next is created when it is completed or deleted. Valid natural-language forms such as `every Friday`, `every weekday`, and `every 2 weeks` create immediately on Enter, just like explicit `^` forms. Invalid or conflicting interpretations open review for correction instead. `R` edits a recurrence; `X` or `x` in details stops a series, keeping its pending instance and completed history. Projects and tags may be new; the server performs final validation.

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
| `u` | undo the latest change while the undo window is open |
| `r` | reload from the server |
| `?` | help |
| `q` | quit |
| `Esc` | close/cancel the active overlay |

Mouse support is limited to view/task clicks and task-list wheel scrolling.

## Configuration

Momentum uses `$XDG_CONFIG_HOME/momentum/config.toml`, or `~/.config/momentum/config.toml`. `--config PATH` wins. Every setting but `[backend] url` has a default. Unknown keys are errors, and `[sync]`'s old Taskwarrior keys (`enabled`, `interval`, `startup`, `shutdown`) fail with a message saying they were removed.

Copy [`config.example.toml`](./config.example.toml) to get started. `MOMENTUM_ICONS=unicode|nerd|ascii` overrides the icon setting. `0s` disables background task refresh.

Themes are `auto`, `dark`, `light`, and `terminal` (your terminal's 16 ANSI colors); icons are `unicode`, `nerd`, and `ascii`. In Settings, `Tab` switches to **Appearance**, where `←`/`→` previews each theme live, `Enter` saves it to the active config file, and `Esc` discards the preview.

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

Each entry has a readable `name` and a unique project `value`. Values use lowercase letters/numbers, hyphens, and dots for hierarchy. Parent entries are optional and selectable. The example displays **Work → Client** and inserts `#work.client`; neither spaces nor display labels are sent as project values.

- Type `#` in quick capture to see suggestions; type part of a name or value to filter, use arrows to select, and Tab to complete.
- Configured projects appear first for an empty query and merge with the projects of pending tasks without duplicate values. They are also available in the project editor.
- The catalog is available even with no tasks or when project discovery fails. It is local to this device's config, not stored on the server.
- Use **Settings → Projects** to add, edit, reparent, or remove entries; successful saves update suggestions immediately and persist to the active config path. Manual TOML edits still take effect on restart. Display-name edits affect suggestions **only**, never existing tasks. Discovered projects can still appear after removal from the catalog.
- Unknown project values remain valid in quick capture. New task assignments are not automatically saved to the catalog.

Settings → Projects is a catalog-management screen, not a separate project database; the catalog still lives entirely in configuration. Value edits default to Catalog only. The user may explicitly preview and confirm a pending-only migration, optionally including dotted descendants; completed, deleted, waiting, recurring, and historical values are retained. The server moves every confirmed task in one transaction, or none if any changed since the preview. Config and task outcomes are reported separately, with no batch undo. See the [implementation tracker](docs/plans/settings-projects.md), [requirements](docs/spec/settings-projects.md), [UAT evidence](docs/UAT.md), and [decision record](docs/adr/0001-project-settings-and-pending-task-renames.md).

## Dotfiles and privacy

`config.toml` may be tracked in dotfiles; `credentials.json`, which holds the sign-in, must not be. Momentum has no telemetry, crash reporting, automatic update checks, or network activity outside the configured server and the browser sign-in it starts.

`momentum doctor` checks that the server answers, that the saved sign-in is valid there, the device's time zone, configuration, terminal support, and writable state paths, without mutating tasks or printing secret values. `--debug` writes redacted structured logs under the XDG state directory.

## macOS app (preview)

[`macos/`](../macos/README.md) contains a native SwiftUI frontend that shares this repository's Go code through `cmd/momentum-bridge`. It covers the Today, Inbox and Completed views, capture with correction review, editing, task actions, sign-in, and undo; Plan Today and Settings come next.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/momentum
```

See [`CONTRIBUTING.md`](../CONTRIBUTING.md) for isolation and release rules.
