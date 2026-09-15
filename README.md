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

- **Inbox** contains every pending task in the active Taskwarrior context.
- **Today** contains unique pending tasks classified as Overdue, Due Today, or Scheduled Today, in that order.
- Taskwarrior urgency sorts rows; raw urgency is not shown in the normal row.

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
| `1`/`2` | Inbox/Today |
| `Enter` | details |
| `Ctrl+K` | quick add |
| `/` | local fuzzy search |
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
