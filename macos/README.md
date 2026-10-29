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
- **Calendar:** each load reads the meetings of the Source: the `.ics` files of `[calendar]`, this Mac's macOS Calendar, or your Google Calendar, through the server, when `[calendar] source` is `google` (see [Daily planning and calendar awareness](../tui/README.md#daily-planning-and-calendar-awareness)). Busy time in working hours counts against the day's capacity, overlapping events once, and Plan Today shows the merged blocks with their titles on hover. Plan Today always asks Google again; the sidebar's meter reuses what a load read for up to five minutes, so it can lag a new Google meeting by that long. A Google meeting you declined doesn't count. When the calendar can't be read (a file, the Mac's access, or a Google connection that is missing, needs reconnecting or can't be reached), the whole calendar is unavailable and planning goes on with tasks. Event titles are never stored or logged.

- **Settings (⌘, or the sidebar's Settings row):** a window of five tabs, Projects, Planning, Calendar, Appearance and Diagnostics. Each change is applied at once and written to `config.toml`, keeping its comments and other keys; a save is refused, and the tab reloaded, when the file changed since the tab read it.
  - **Projects:** the catalog in `config.toml` and the values found only on tasks, in one list with the hierarchy, each tagged Catalog or From tasks and with its pending-task count. **Add Project…**, **Edit…** and **Remove…** each open their own sheet or prompt and save at once, and a From tasks row offers **Add to Catalog…**. An edit that changes the full value goes on to a second step: **Catalog only** (the default) or **Catalog + pending tasks**, with **Include subprojects**, the counts and a warning when tasks already use the new value. The catalog is saved before the tasks move, the server moves all of them or none, and a rename that moves tasks closes the undo window.
  - **Planning:** **Use Plan Today** turns the Plan button, the sidebar's Today’s plan box, the menu item and `P` on or off. Daily capacity, buffer and each weekday's own capacity are typed as `8h` or `7h30m`; a value the config would refuse stays in its field with a message and is not saved.
  - **Calendar:** **Use calendar files** and the list of `.ics` files, added with **Add…** and taken out with **Remove**. Each file shows whether it is available, stale, missing or unreadable, and **Stale after** sets how long a file may go unchanged before it shows as stale. **Working hours** (Start and End), **Count all-day events** and **Count events shown as free** choose which meetings take time off the day. **Source** chooses between **Calendar files**, **macOS Calendar** and **Google Calendar**. The first and the last are written to `config.toml` as `[calendar] source`, so the terminal app on this Mac follows them, and choosing either clears a macOS Calendar choice; macOS Calendar is kept for this Mac only and counts over what `config.toml` says. Choosing macOS Calendar asks the Mac for access to your calendars, lists them by account with a switch each, and hands today's events to the bridge on each load and plan (titles only with a plan). Without access, Plan Today says “Calendar unavailable: no access” and the tab offers to open System Settings. Choosing it never changes `[calendar] enabled` or `paths`. Choosing Google Calendar counts whatever **Use calendar files** says and hides the file list. The tab says who is connected (“Connected as …”), or offers **Connect…**, which opens the browser on a one-time link and waits for you to approve in Google (**Open Browser** and **Cancel**; the link works for 10 minutes). The connection belongs to your Momentum account, so another Mac sees it without the browser. When Google no longer accepts it the tab says “Needs reconnecting” and offers **Reconnect…**, and **Disconnect…** asks first and then ends the connection for every Mac and the terminal app, saying so if Google could not be asked to revoke its access. Signed out, or on a server that doesn't offer Google Calendar, the tab says that instead. The app only reads; a build with the hardened runtime needs the `com.apple.security.personal-information.calendars` entitlement.
  - **Appearance:** System, Light or Dark, for this Mac only.
  - **Diagnostics:** the checks of the server, the sign-in, the time zone, the config and the calendar (which asks the server whether Google Calendar is connected when `[calendar] source` is `google`, and macOS Calendar when it is the source), each an OK, Skipped or Problem with its detail, the app and bridge versions and the config path. **Show in Finder** reveals `config.toml`, **Copy Report** copies the checks and versions for a bug report, with no sign-in code or event title, and **Check Again** runs them again. The checks are the terminal app's `momentum doctor`, without its Terminal and State directory checks.

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
| Settings window | Projects catalog, appearance, planning, calendar, and diagnostics | Partly: Projects and Appearance | Yes |
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
