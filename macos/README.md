# Momentum for macOS

A native SwiftUI frontend for Momentum, built as **Momentum Task Manager.app** so it is not confused with an unrelated app called Momentum. It shares the TUI's Go code through `momentum-bridge`, a child process the app starts and talks to over newline-delimited JSON. Tasks live on a [Momentum server](#server-and-sign-in), which the bridge reaches over HTTPS.

```
Momentum Task Manager.app ──JSON lines──▶ momentum-bridge (Go) ──HTTPS──▶ momentum-server
                                           internal/bridge, backend,
                                           domain, quickadd, config
```

## Requirements

- macOS 15 or later
- Xcode 16 or later, [XcodeGen](https://github.com/yonaskolb/XcodeGen) and Go (to build the bundled bridge)
- A Momentum server named in `config.toml` (see [Server and sign-in](#server-and-sign-in))

## Build and run

```sh
cd macos
xcodegen generate          # Momentum.xcodeproj is generated, not committed
open Momentum.xcodeproj    # or:
xcodebuild -scheme Momentum -configuration Debug build
```

The `Build momentum-bridge` phase compiles `tui/cmd/momentum-bridge` for each target architecture and places it in `Momentum Task Manager.app/Contents/MacOS`. The app is not sandboxed because the bridge reads Momentum's `config.toml`.

To try the app with another config, such as a local server's, launch the executable directly so the bridge inherits the environment:

```sh
XDG_CONFIG_HOME=/tmp/demo/config \
  "path/to/Momentum Task Manager.app/Contents/MacOS/Momentum Task Manager"
```

## Tests

```sh
xcodebuild -scheme Momentum test                                  # Swift: store, undo window, search, wire format, sign-in, Plan Today
(cd ../tui && go test ./internal/bridge/ ./internal/quickadd/)    # Go: protocol, views, edits, capture review, plans, calendar
```

The bridge's tests reach a fake server over HTTP, and its calendar tests read `.ics` files from a temporary directory.

To see the app, `tools/snap.sh` builds the Debug app and saves a window image of each page in light and dark to `/tmp/Momentum-shots`, one `<page>-light.png` and `<page>-dark.png` per page. It runs on a fake server and an in-memory sign-in, under its own bundle identifier, so it never touches your server, Keychain, `config.toml` or the real app's preferences. `MOMENTUM_SNAPSHOT_PAGES=today,inbox` limits it to some pages. Run it from a terminal that has Screen Recording permission, and leave the screen alone while it runs, since the app's window comes to the front for each image.

## What it does

- **Views:** Today (Overdue, Due Today, Scheduled Today, Planned Today), Inbox grouped by project, and Completed for the last 30 days. Startup opens Today when it has work, otherwise Inbox.
- **Capture (⌘N or `c`):** natural language and `# ! @ > + ~ ^` triggers are highlighted live by the Go interpreter, with suggestions (↑ ↓, ⇥). Return adds the task; ambiguous input opens a correction review whose rules come from `internal/quickadd`.
- **Edit (`e E p ! D S t R` or ⌘E):** only changed fields are sent, validated by the bridge with the TUI editor's rules.
- **Actions:** complete (Space, ⌘↩), start/stop (`s`, ⌘T), delete (`d`), stop repeating (`x`), refresh (`r`), search (`/`, ⌘F, `project:name`, `project:none`).
- **Undo:** a change can be undone for `[sync].mutation_delay` (15 seconds by default) after it, unless the task changed since. The toast counts it down.
- **Plan Today (`P`, ⇧⌘P, the toolbar's Plan, or the sidebar's Today's plan box):** tick the day's tasks against a capacity column, with the TUI's rules. Obligations (overdue, due or scheduled today) are fixed; tasks planned on an earlier day and still open roll over ticked; everything else is a candidate. The column takes obligations, calendar time and the buffer off the day's capacity, counts tasks without an estimate apart, and warns, but never blocks, when the choice goes over. Confirming writes only `momentum-plan-YYYY-MM-DD` tags, never dates, and offers no undo: reopen Plan Today to change the plan. When a change fails partway, the sheet stays open with its ticks, and confirming again finishes without applying anything twice. The sidebar box counts the tasks planned for today outside the obligations. `[planning] enabled = false` hides Plan Today.
  - In the sheet: ↑ ↓ or `j` `k` move over the rows you can tick, Space ticks, `g` `G` go to the first or last, `/` or ⌘F filter the candidates, ↩ confirms (in the filter, it returns to the list), and Esc cancels (in the filter, it clears the filter first).
- **Calendar:** with `[calendar]` on, each load reads its local `.ics` files (see [Daily planning and calendar awareness](../tui/README.md#daily-planning-and-calendar-awareness)). Busy time in working hours counts against the day's capacity, overlapping events once, and Plan Today shows the merged blocks with their titles on hover. When a file can't be read, the whole calendar is unavailable and planning goes on with tasks. Event titles are never stored or logged.

- **Settings (⌘, or the sidebar's Settings row):** a window of five tabs, Projects, Planning, Calendar, Appearance and Diagnostics. Appearance is built: System, Light or Dark, for this Mac only, applied at once. The other tabs say how to change the same thing in `config.toml` until they are built.

The bridge reads `config.toml` again whenever the file changes, on each load and when Settings opens, so `[planning]`, `[calendar]`, `[projects]`, `refresh_interval` and `[sync]` edits apply without a restart. A file that no longer loads leaves the last good settings in use, and Settings says why. A changed `[backend] url` still needs the app to be quit and reopened, and Settings says so.

Press `?` in the app for every shortcut.

## Server and sign-in

The app reads the `[backend]` section of Momentum's `config.toml`, the file the TUI reads (see [Server and sign-in](../tui/README.md#server-and-sign-in)):

```toml
[backend]
url = "https://momentum.example.com"
```

Without it, the window says how to add it; quit and reopen the app after editing the file.

- **Sign in:** the window shows **Sign In…** until the app is signed in, and **Momentum Task Manager ▸ Sign In…** signs in again. The sheet shows a short code and opens the browser, where you enter it and sign in with Google or GitHub; the app checks every few seconds until you finish. Signing in again replaces the sign-in and revokes the old one.
- **Where the sign-in lives:** in the login Keychain, as a generic password under the service `dev.jvrviegas.momentum.sign-in` and the server's address. The app hands the token to the bridge over its standard input, and again whenever the bridge restarts; it is never in arguments, the environment, or a file. The app does not use the TUI's `credentials.json` from `momentum login`, so the app and the TUI are separate sessions, each with its own undo. Builds signed ad hoc, such as the Debug build, may ask for Keychain access again after a rebuild, because the Keychain recognizes the app by its signature.
- **Sign out:** **Momentum Task Manager ▸ Sign Out** revokes the sign-in on the server and removes it from the Keychain. When the server cannot revoke it, the app still removes it and says the token works until it expires.
- **States:** the toolbar reads **Online**, **Saved · undo 12s** after a change, **Server unreachable** (click to try again), or **Signed out** (click to sign in). Nothing is queued while the server is unreachable, and quitting never waits.
- **Tasks from Taskwarrior:** run `momentum import` in a terminal (see the [TUI README](../tui/README.md#bringing-tasks-over-from-taskwarrior)). The sidebar shows the account and the server.

## Not yet built

Where each one already exists; — means it does not apply.

| Item | Description | TUI | macOS |
|---|---|---|---|
| Settings window | Projects catalog, appearance, planning, calendar, and diagnostics | Partly: Projects and Appearance | Partly: Appearance |
| Project migration | Rename a project in the catalog and move its pending tasks to the new value | Yes | No |
| Signed release build | A build signed for distribution, not ad hoc like the Debug build | — | No |

## Nice to have

From the deferred backlog in [`tui/IMPLEMENTATION_PLAN.md`](../tui/IMPLEMENTATION_PLAN.md#deferred-backlog).

| Item | Description | TUI | macOS |
|---|---|---|---|
| Custom smart views and filters | Saved views beyond Inbox, Today, and Completed | No | No |
| Configurable keybindings | Change the shortcuts, with checks for clashing keys | No | No |
| Annotation and dependency editing | Change a task's annotations and dependencies | No | No |
| Bulk selection and operations | Select several tasks and act on them at once | No | No |
| Task duplication | Copy a task into a new one | No | No |
| Desktop notifications/background agent | Notifications, and a helper that runs while Momentum is closed | No | No |
| Homebrew tap | Install with `brew` | No | No |
| Native Windows support | Run on Windows | No | — |
