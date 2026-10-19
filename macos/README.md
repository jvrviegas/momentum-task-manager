# Momentum for macOS

A native SwiftUI frontend for Momentum, built as **Momentum Task Manager.app** so it is not confused with an unrelated app called Momentum. It shares the TUI's Go code through `momentum-bridge`, a child process the app starts and talks to over newline-delimited JSON. Taskwarrior stays the source of truth; the app never reads or writes Taskwarrior's data files. In [backend mode](#backend-mode), the bridge keeps tasks on a Momentum server instead.

```
Momentum Task Manager.app ──JSON lines──▶ momentum-bridge (Go) ──argv──▶ task
                                           internal/bridge, taskwarrior,      or, in backend mode,
                                           domain, quickadd, config     ──HTTPS──▶ momentum-server
```

## Requirements

- macOS 15 or later
- Xcode 16 or later, [XcodeGen](https://github.com/yonaskolb/XcodeGen) and Go (to build the bundled bridge)
- Taskwarrior 3.x, found on `PATH`, `/opt/homebrew/bin` or `/usr/local/bin`, unless the app is in backend mode

## Build and run

```sh
cd macos
xcodegen generate          # Momentum.xcodeproj is generated, not committed
open Momentum.xcodeproj    # or:
xcodebuild -scheme Momentum -configuration Debug build
```

The `Build momentum-bridge` phase compiles `tui/cmd/momentum-bridge` for each target architecture and places it in `Momentum Task Manager.app/Contents/MacOS`. The app is not sandboxed because it runs `task`, which reads `~/.taskrc`.

To try the app against a throwaway database, launch the executable directly so the bridge inherits the environment:

```sh
TASKRC=/tmp/demo/taskrc TASKDATA=/tmp/demo/data \
  "path/to/Momentum Task Manager.app/Contents/MacOS/Momentum Task Manager"
```

## Tests

```sh
xcodebuild -scheme Momentum test                                  # Swift: store, sync reducer, search, wire format, backend mode
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

## Backend mode

A `[backend]` section with a `url` in Momentum's `config.toml`, the file the TUI reads, puts the app in backend mode, as it does the TUI (see [Backend mode](../tui/README.md#backend-mode-preview)):

```toml
[backend]
url = "https://momentum.example.com"
```

- **Sign in:** the window shows **Sign In…** until the app is signed in, and **Momentum Task Manager ▸ Sign In…** signs in again. The sheet shows a short code and opens the browser, where you enter it and sign in with Google or GitHub; the app checks every few seconds until you finish. Signing in again replaces the sign-in and revokes the old one.
- **Where the sign-in lives:** in the login Keychain, as a generic password under the service `dev.jvrviegas.momentum.sign-in` and the server's address. The app hands the token to the bridge over its standard input, and again whenever the bridge restarts; it is never in arguments, the environment, or a file. The app does not use the TUI's `credentials.json` from `momentum login`, so the app and the TUI are separate sessions, each with its own undo. Builds signed ad hoc, such as the Debug build, may ask for Keychain access again after a rebuild, because the Keychain recognizes the app by its signature.
- **Sign out:** **Momentum Task Manager ▸ Sign Out** revokes the sign-in on the server and removes it from the Keychain. When the server cannot revoke it, the app still removes it and says the token works until it expires.
- **States:** the toolbar reads **Online**, **Saved · undo 12s** after a change, **Server unreachable** (click to try again), or **Signed out** (click to sign in). Nothing is queued while the server is unreachable.
- **Undo and sync:** a change can be undone for `[sync].mutation_delay` (15 seconds by default), unless the task changed since. Nothing syncs, so there is no sync countdown, no **Sync Now**, and no prompt when quitting. Backend mode has no Taskwarrior contexts; the sidebar shows the account and the server instead.

## Not yet built

Plan Today and its capacity meter, the Settings window (projects catalog, appearance, planning, calendar, diagnostics), project migration, calendar awareness, and a signed release build.
