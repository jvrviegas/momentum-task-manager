# Momentum

Momentum is a keyboard-first terminal frontend for [Taskwarrior 3.x](https://taskwarrior.org/). Taskwarrior remains the only source of truth: Momentum reads `task ... export`, sends normal `task` commands, and derives Inbox/Today in memory.

## Requirements

- Go 1.27 or a compatible current Go toolchain for source builds
- Taskwarrior 3.x on macOS or Linux
- A terminal with UTF-8 support (no Nerd Font is required)

## Install

Install the latest published command:

```sh
go install github.com/jvrviegas/momentum/cmd/momentum@latest
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
momentum doctor
momentum --config PATH
momentum --debug
momentum --version
```

Momentum never writes Taskwarrior data files directly. It invokes `task` with an argument vector, so task descriptions and metadata are never passed through a shell.

## Views

- **Inbox** contains every pending task in the active Taskwarrior context; context-scoped machine-readable exports must apply that context filter explicitly (see the [Settings → Projects feasibility record](docs/plans/settings-projects.md#t00-feasibility-record--2026-09-16)).
- **Today** contains unique pending tasks classified as Overdue, Due Today, or Scheduled Today, in that order.
- Inbox groups tasks by project alphabetically, with oldest-created tasks first within each group. Unassigned tasks appear under **No project** at the end; missing creation dates sort last within a group.
- Today rows sort by Taskwarrior urgency; raw urgency is not shown in the normal row.

## Quick add

Press `Ctrl+K`, type a description, and press Enter. Metadata triggers work at token boundaries:

```text
Prepare proposal #work.client !high @tomorrow >monday +planning
```

| Trigger | Field | Example |
|---|---|---|
| `#` | project | `#work.client` |
| `!` | priority | `!high`, `!medium`, `!low`, `!none` |
| `@` | due | `@today`, `@tomorrow`, `@2026-09-20` |
| `>` | scheduled | `>today`, `>monday` |
| `+` | tag | `+planning` |

Use a leading backslash for a literal trigger, for example `\#launch`. Email addresses remain description text. Projects and tags may be new; Taskwarrior performs final validation.

## Controls

| Key | Action |
|---|---|
| `j`/`k`, arrows | move selection |
| `h`/`l`, `Tab` | switch focus |
| `g`/`G` | first/last task |
| `1`/`2`/`3` | Inbox/Today/Settings → Projects |
| `Enter` | details |
| `Ctrl+K` | quick add |
| `/` | local fuzzy search; use `project:name` for an exact project or `project:none` for unassigned tasks |
| `Space` | complete |
| `e`, `p`, `!`, `D`, `S`, `t` | edit Description, Project, Priority, Due, Scheduled, Tags |
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

Momentum does not implement or provision synchronization. It runs Taskwarrior's native `task sync` when Taskwarrior sync settings are present. Local work remains available when sync is unavailable. After a mutation, sync waits 15 seconds and shows an undo grace countdown. See [`docs/sync.md`](./docs/sync.md) for the manual TaskChampion/Cloud Run/Neon setup.

For local-only use, set `[sync].enabled = false`.

## Dotfiles and privacy

A shared `.taskrc` may include a non-secret file:

```text
include ~/.config/taskwarrior/secrets.rc
```

Keep `secrets.rc` untracked, create it with mode `0600`, and transfer it through a password manager. Never commit a sync URL, client UUID, encryption secret, or database credential. Momentum has no telemetry, crash reporting, automatic update checks, or network activity outside a user-initiated/native Taskwarrior sync.

`momentum doctor` checks availability, export decoding, configuration, sync readiness, terminal support, and writable state paths without mutating tasks or printing secret values. `--debug` writes redacted structured logs under the XDG state directory.

## Development

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/momentum
```

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for isolation and release rules.
