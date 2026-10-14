# Momentum for macOS

A native SwiftUI frontend for Momentum. It shares the TUI's Go code through `momentum-bridge`, a child process the app starts and talks to over newline-delimited JSON. Taskwarrior stays the source of truth; the app never reads or writes Taskwarrior's data files.

```
Momentum.app (SwiftUI) ──JSON lines──▶ momentum-bridge (Go) ──argv──▶ task
                                        internal/bridge, taskwarrior,
                                        domain, quickadd, config
```

## Requirements

- macOS 15 or later
- Xcode 16 or later, [XcodeGen](https://github.com/yonaskolb/XcodeGen) and Go (to build the bundled bridge)
- Taskwarrior 3.x, found on `PATH`, `/opt/homebrew/bin` or `/usr/local/bin`

## Build and run

```sh
cd macos
xcodegen generate          # Momentum.xcodeproj is generated, not committed
open Momentum.xcodeproj    # or:
xcodebuild -scheme Momentum -configuration Debug build
```

The `Build momentum-bridge` phase compiles `tui/cmd/momentum-bridge` for each target architecture and places it in `Momentum.app/Contents/MacOS`. The app is not sandboxed because it runs `task`, which reads `~/.taskrc`.

To try the app against a throwaway database, launch the executable directly so the bridge inherits the environment:

```sh
TASKRC=/tmp/demo/taskrc TASKDATA=/tmp/demo/data \
  path/to/Momentum.app/Contents/MacOS/Momentum
```

## Tests

```sh
xcodebuild -scheme Momentum test                                  # Swift: store, sync reducer, search, wire format
(cd ../tui && go test ./internal/bridge/ ./internal/quickadd/)    # Go: protocol, views, edits, capture review
```

The bridge's integration test runs real Taskwarrior against a temporary `TASKRC`/`TASKDATA` and refuses the real `~/.taskrc` and `~/.task`.

## What it does

- **Views:** Today (Overdue, Due Today, Scheduled Today, Planned Today), Inbox grouped by project, and Completed for the last 30 days. Startup opens Today when it has work, otherwise Inbox.
- **Capture (⌘N or `c`):** natural language and `# ! @ > + ~ ^` triggers are highlighted live by the Go interpreter, with suggestions (↑ ↓, ⇥). Return adds the task; ambiguous input opens a correction review whose rules come from `internal/quickadd`.
- **Edit (`e E p ! D S t R` or ⌘E):** only changed fields are sent, validated by the bridge with the TUI editor's rules.
- **Actions:** complete (Space, ⌘↩), start/stop (`s`, ⌘T), delete (`d`), stop repeating (`x`), refresh (`r`), search (`/`, ⌘F, `project:name`, `project:none`).
- **Sync and undo:** the TUI's sync state machine, ported to Swift: a 15-second undo window after each change, sync every 5 minutes with retry backoff, local-only mode when Taskwarrior sync is not configured, and a prompt to sync before quitting with unsynced changes.

Press `?` in the app for every shortcut.

## Not yet built

Plan Today and its capacity meter, the Settings window (projects catalog, appearance, planning, calendar, diagnostics), project migration, calendar awareness, and a signed release build.
