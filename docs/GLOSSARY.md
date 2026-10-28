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

## Open naming questions

Found while reading the built views. Each is one idea with two names, so each is a bug to settle:

1. **The app's own name.** The Dock, menu bar and window say "Momentum Task Manager". The sign-in sheet ("Sign in to Momentum"), the startup error ("Couldn’t start Momentum") and the server empty state ("Momentum keeps your tasks on…") say "Momentum". Decide whether "Momentum" alone means the product (server and apps) and "Momentum Task Manager" this app, then make every string follow.
2. **New Task or Quick capture.** The menu, toolbar and toast say "New Task" and "Added"; the Keyboard Shortcuts sheet's group and its `c` row say "Quick capture". The glossary picks New Task.
3. **Refresh or reload.** The menu and toast say "Refresh"; the toolbar's hover text says "Click to reload."
4. **Couldn’t or Unable to.** The app writes "Couldn’t load tasks", "Couldn’t start Momentum" and "Couldn’t read the sign-in…"; Apple's pattern is "Unable to load tasks". The app is consistent with itself, so this is a style choice, not a bug.
