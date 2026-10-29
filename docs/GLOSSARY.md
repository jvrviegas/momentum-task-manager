# Glossary

Every thing in the Mac app has one name. Started from the words M1 puts on screen (S-001 to S-008), read from the Swift views. Later milestones add their terms when their `UX:` blocks are written.

## Voice

Plain, direct, brief. Say "you" to the person, never "the user". Say "click", never "tap". Errors say what happened and what to do, without blame.

## Terms

| Term | Means | Not |
| --- | --- | --- |
| Task | One thing to do, kept on the server | to-do, item, entry, TaskItem |
| Description | The words that name a task, what you type when you add one | title, name, summary |
| Server | The Momentum server that keeps your tasks and your sign-in; named by `[backend] url` | backend, cloud, account |
| Sign In | Prove who you are to the server, with a code you enter in the browser (a menu item and a button, with an ellipsis: **Sign In…**) | log in, login, authenticate, connect |
| Sign Out | Forget the sign-in on this Mac and revoke it on the server | log out, disconnect |
| Code | The short code the sheet shows and you enter in the browser | device code, user code, token |
| View | One of the three sidebar pages, a window onto your tasks | list, folder, tab, page |
| Inbox | The view of pending tasks that are not due or scheduled today, grouped by project | backlog, all tasks |
| Today | The view of what is overdue, due, scheduled or planned for today | agenda, dashboard |
| Completed | The read-only view of the tasks finished in the last 30 days; also the state a task is in once done | done, archive, history |
| New Task | The command and the sheet that add a task (⌘N, `c`, the toolbar button, File ▸ New Task) | Quick capture, capture, quick add, create |
| Add Task | The button that adds the task in the review sheet; the toast says "Added “…”" | Save, Create, Submit |
| Complete | Mark a task done; it leaves its view and shows in Completed | check, finish, close, tick |
| Refresh | Load the tasks again from the server | reload, sync, update |
| Undo | Take back the last change, for as long as the toolbar shows "Saved · undo" and a countdown | revert, restore |
| Online | The toolbar's word when the server answered and nothing is waiting | Connected, Synced |
| Server unreachable | The toolbar's words when the server did not answer; click to try again | Offline, Disconnected |
| Settings | The window where you change how the app works: Apple's word, opened with ⌘, or the sidebar's **Settings** row | Preferences, Options, Configuration |
| config.toml | The file that holds the server's address and every setting; named exactly so, in monospace | config, settings file, preferences file |
| Projects, Planning, Calendar, Appearance, Diagnostics | The five tabs of Settings, in that order | General, Advanced, Account |
| Appearance | Whether this Mac's window is light or dark: System (follows the Mac), Light, Dark | Theme, Mode |
| Show config.toml in Finder | The button that reveals the file, for the first step of any edit | Open Config, Edit Config |
| Plan Today | The sheet where you choose today's tasks against the hours the day has: P, ⇧⌘P, the toolbar's Plan button or the sidebar's Plan Today button; **Use Plan Today** in Settings turns all of it on or off | daily plan, planner, planning ritual |
| Capacity | The length of time a day can hold: the daily capacity, or a weekday's own | workload, budget, hours |
| Buffer | Time kept free each day for what you can't plan | slack, padding, margin |
| Calendar file | An `.ics` file whose meetings take time off a day's capacity; added in Settings ▸ Calendar | calendar feed, subscription, source |
| Available, Stale, Missing, Unreadable | The four states of a calendar file, always a word with a symbol | OK, Old, Not found, Error, Broken |
| Source | Where this Mac reads its meetings from, **Calendar files** or **macOS Calendar**; kept on this Mac, not in `config.toml` | provider, account, feed |
| macOS Calendar | The Calendar app's own calendars, read with the Mac's permission; Settings ▸ Calendar lists them by account with a switch each | iCloud calendar, system calendar, EventKit |
| Project | A place tasks belong to, written as a dotted value such as `work.client`; its name labels the last segment | folder, category, list |
| Catalog | The projects kept in `config.toml`, which New Task suggests; managed in Settings ▸ Projects | project list, suggestions file |
| From tasks | The tag of a project that only tasks use, with **Add to Catalog…** to keep it | discovered, orphan, unknown |
| Include subprojects | The choice that makes a rename also move the entries and tasks below the project | cascade, recursive |
| Catalog only, Catalog + pending tasks | The two ways to save a rename: the catalog alone (the default), or the catalog and the pending tasks that use the old value | label only, migrate |
| Check, Report | A **Check** is one result of Diagnostics, always a word (OK, Skipped, Problem) with a symbol; the **Report** is all of them with the versions and the config path, which **Copy Report** puts on the clipboard | test, health check, log, diagnostics file |
| Working hours | The hours of the day whose meetings take time off the capacity; Start and End in Settings ▸ Calendar, 09:00 to 17:00 by default | office hours, availability, day length |
| Free event | An event the calendar marks as free rather than busy, such as a reminder; **Count events shown as free** makes it count | transparent event, available event |
| Stale after | How long a calendar file may go unchanged before it shows as Stale; `0m` means never | expiry, max age, refresh interval |

## Open naming questions

Found while reading the built views. Each is one idea with two names, so each is a bug to settle:

1. **The app's own name.** The Dock, menu bar and window say "Momentum Task Manager". The sign-in sheet ("Sign in to Momentum"), the startup error ("Couldn’t start Momentum") and the server empty state ("Momentum keeps your tasks on…") say "Momentum". Decide whether "Momentum" alone means the product (server and apps) and "Momentum Task Manager" this app, then make every string follow.
2. **New Task or Quick capture.** The menu, toolbar and toast say "New Task" and "Added"; the Keyboard Shortcuts sheet's group and its `c` row say "Quick capture". The glossary picks New Task.
3. **Refresh or reload.** The menu and toast say "Refresh"; the toolbar's hover text says "Click to reload."
4. **Couldn’t or Unable to.** The app writes "Couldn’t load tasks", "Couldn’t start Momentum" and "Couldn’t read the sign-in…"; Apple's pattern is "Unable to load tasks". The app is consistent with itself, so this is a style choice, not a bug.
5. **Plan Today or daily plan.** The sheet, the button and the menu say "Plan Today"; the toasts after confirming say "Daily plan saved · N tasks" and "Daily plan unchanged". The glossary picks Plan Today for the sheet and the feature; the toasts could say "Plan saved" and "Plan unchanged".
