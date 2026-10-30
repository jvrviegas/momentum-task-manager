# Momentum Task Manager (macOS) — user stories

Scope: the native macOS app in `macos/`. The terminal app and the server appear only where a story needs them. This backlog was written after M1–M5 were built, so those stories say what the app does today; `Status: done` means the behavior exists and the check named on the line passes or was tried by hand.

Sources, since the project has no `docs/BRIEF.md`, `docs/research/` or `docs/screens/INDEX.md`: [`macos/README.md`](../macos/README.md), [`tui/DESIGN.md`](../tui/DESIGN.md), and the plans in [`docs/plans/`](./plans/).

How a line is checked:

- `[unit]` a Swift test in `macos/MomentumTests` or a Go test in `tui/internal/bridge` or `tui/internal/quickadd`. The line names the Swift test when there is one.
- `[manual]` a person tried it in the running app. The project has no snapshot or self-test harness yet, so lines that would be `[snap]` or `[self]` on `done` stories are `[manual]`. S-040 adds the harness; the `Screens:` names are the pages it will show.
- `[snap]` and `[self]` appear only on stories that are not built yet.

## M1 Core loop

Set up, sign in, see tasks, add one, complete one. Tasks live on the server, so they survive a restart.

### S-001 Add the server address
As the user, I open the app before a server is set up and it tells me what to add and where.
Screens: setup.png
Acceptance:
- [unit] `withoutAServerNothingLoads`: no server in `config.toml` means nothing loads and no sign-in is offered.
- [manual] The window says "Add a Momentum server", shows the config file's path and a `[backend] url = "…"` example, and says to quit and reopen after editing.
- [manual] **Sign In…** and **Sign Out** are missing from the app menu until a server is set.
UX:
- Flow: open the app with no `[backend] url` -> the list area says "Add a Momentum server" with the config file's path and the two lines to add -> edit `config.toml` elsewhere -> quit and reopen -> the signed-out page (S-002). Nothing has changed, so there is no way back to give.
- Surface: the task list's empty state, inline (information that needs no action in the app goes inline, never in an alert).
- States: this is the first-run state; there is no loading or error. The path and the snippet are selectable text.
- Words: title "Add a Momentum server"; "Momentum keeps your tasks on a Momentum server. Add its address to <path>:"; `[backend]` / `url = "https://momentum.example.com"`; "Then quit and reopen Momentum Task Manager, and sign in."
- Access: VoiceOver reads the title and description (system empty-state view); the snippet can be selected and copied.
- Review: the empty state has no button for its first action, so you must find the file yourself. A **Show Config in Finder** button would fix it. Settings (S-054) keeps `[backend] url` needing a restart, so the last line stays true.
Status: done

### S-002 Sign in with a code
As the user, I choose Sign In…, finish in the browser with Google or GitHub, and the app signs in.
Screens: sign-in.png, today.png
Acceptance:
- [manual] Signed out, the window says "Sign in to see your tasks" with a **Sign In…** button; the toolbar reads "Signed out".
- [manual] The sheet shows a short code, **Open Browser**, **Copy Code**, and the address to visit; the browser opens once per code.
- [unit] `signInPollsUntilThePersonSignsInAndRevokesThePreviousSignIn`: the app polls until the browser finishes and revokes the sign-in it replaces.
- [unit] `aRefusedSignInOffersToTryAgain`, `cancellingStopsPolling`: a refusal shows its message and **Try Again**; Cancel stops polling.
- [unit] `restoresTheKeychainSignInForTheServer`, `anExpiredSignInCountsAsSignedOut`, `aSignInTheKeychainRefusesIsRevoked`: the sign-in is kept in the login Keychain, restored on the next start, and an expired or refused one counts as signed out.
- [manual] A toast says "Signed in as" and the account's email, and the sidebar footer shows the email and the server.
UX:
- Flow: the empty-state **Sign In…** button, the toolbar's "Signed out", or Momentum Task Manager ▸ Sign In… -> sheet "Sign in to Momentum" shows the server, then the code -> the browser opens by itself once per code (**Open Browser** opens it again) -> enter the code and choose Google or GitHub -> the sheet closes, a toast says "Signed in as <email>", tasks load. Way back: Esc or **Cancel** closes the sheet and stops waiting; after a failure **Try Again** starts over.
- Surface: sheet (one short task that must finish or cancel before going on).
- States: starting ("Starting sign-in…" with a spinner); waiting (the code, "Waiting for you to finish signing in…"); failed (the server's message with a warning symbol, **Try Again**); signed out with no tasks shown ("Sign in to see your tasks", **Sign In…**).
- Words: "Sign In…" (ellipsis, since it opens a sheet); "Enter this code in your browser, then sign in with Google or GitHub."; "Open Browser", "Copy Code", "Or go to <address>"; "Try Again", "Cancel".
- Access: Return opens the browser, Esc cancels; VoiceOver reads the code one character at a time; the code and the address are selectable; the failure is a symbol, a color and a sentence.
- Review: the sheet's title says "Sign in to Momentum" while the app is "Momentum Task Manager" (glossary question 1).
Status: done

### S-003 Sign out
As the user, I choose Sign Out and the app forgets me.
Screens: setup.png, today.png
Depends: S-002
Acceptance:
- [unit] `signOutRevokesAndForgetsTheSignIn`: the server revokes the sign-in and the Keychain entry is removed.
- [unit] `signOutForgetsTheSignInWhenTheServerCannotRevokeIt`: the app still signs out and the toast says the token works until it expires.
- [manual] Sign Out is off while signed out; New Task, Plan and the other task actions are off too.
UX:
- Flow: Momentum Task Manager ▸ Sign Out -> the sign-in is revoked on the server and removed from the Keychain -> a toast says "Signed out" and the toolbar reads "Signed out". Way back: Sign In… (nothing is lost, since the tasks live on the server).
- Surface: menu command that acts at once, with no confirmation, because signing back in restores everything.
- States: done is the toast and the toolbar; error is the second toast below. The menu item is off while signed out or while a sheet is open.
- Words: "Sign Out" (no ellipsis, acts at once); "Signed out"; when the server can't revoke it: "Signed out. The server couldn’t revoke the sign-in, so its token works until <date>: <reason>".
- Access: reachable from the menu bar; the toast is announced as a changing element.
- Review: the failure toast uses "token", a word the glossary doesn't have, and runs long for a three-line toast. "Signed out. The sign-in stays valid on the server until <date>." says it with a glossary word.
Status: done

### S-004 See my tasks when I open the app
As the user, I open the app and see Today when it has work, otherwise Inbox.
Screens: today.png, inbox.png
Depends: S-002
Acceptance:
- [unit] `startupOpensTodayWhenItHasWork`, `startupOpensInboxWhenTodayIsEmpty`.
- [manual] The sidebar lists Inbox, Today and Completed; Inbox and Today show how many tasks they hold, and Completed shows no count.
- [manual] While tasks load the window says "Loading tasks from the server…"; a failed load says "Couldn’t load tasks" and "No data changed", with **Try Again**.
- [manual] Quit and reopen: the same tasks are there.
UX:
- Flow: launch -> the toolbar reads "Connecting…" and the list says "Loading tasks from the server…" -> Today opens if it has work, otherwise Inbox -> the sidebar counts appear. Way back: none needed; on failure **Try Again** loads again.
- Surface: sidebar pages (a place people come back to), with the window title naming the view and its subtitle the date (Today) or the rule (Inbox, Completed).
- States: empty per view ("Nothing due today" with **New Task** and **Open Inbox**; "No pending tasks" with **New Task**; "No recently completed tasks"); loading as above; error "Couldn’t load tasks" with "No data changed." and **Try Again**; a startup error "Couldn’t start Momentum" with **Try Again**; too much is a scrolling list of sections, with long descriptions cut to one line.
- Words: the sidebar names "Inbox", "Today", "Completed"; counts beside Inbox and Today only.
- Access: ↑ ↓ in the sidebar, `1` `2` `3` and ⌘1 to ⌘3 switch views; each row reads as one element.
- Review: a long description is cut to one line with no hover text for the full text (it shows in the inspector, S-010); "Couldn’t start Momentum" uses the product name (glossary question 1).
Status: done

### S-005 Add a task by typing
As the user, I press ⌘N, type a description and press Return, and the task appears in the list.
Screens: capture.png, inbox.png
Depends: S-004
Acceptance:
- [manual] ⌘N, `c` and the **New Task** toolbar button open the sheet; all three are off while signed out.
- [manual] Return adds the task, a toast says "Added “…”", and the new task is selected in the list.
- [manual] Esc closes the sheet and adds nothing.
UX:
- Flow: ⌘N, `c`, the toolbar's **New Task**, or File ▸ New Task -> sheet, text field in focus -> type -> Return adds it, the sheet closes, a toast says "Added “…”", the new task is selected -> `u` or ⌘Z takes it back while the countdown runs. Way back: Esc closes the sheet without adding.
- Surface: sheet (one short task that must finish or cancel before going on). The Panel row fits repeat entry better; that belongs to the quick-add work, not here.
- States: empty field shows an example sentence as its placeholder; adding shows a spinner in the footer; an error shows inline in the sheet, under the field, in red with a warning symbol; input the app can't read one way goes to the review sheet (S-025).
- Words: toolbar "New Task"; footer hints "↩ Add" (or "↩ Review" when the input needs it) and "esc Close"; toast "Added “<description>”".
- Access: the field is labeled "New task" for VoiceOver; the whole flow works from the keyboard.
- Review (most serious first): (1) the sheet has no **Add Task** and no **Cancel** button, only the keyboard hints, so a pointer or VoiceOver user can't finish; the review sheet already has both. (2) The toolbar's hover text "New Task (⌘N)" repeats the name instead of saying what the button does, for example "Add a task to your list (⌘N)". (3) The sheet has no title, and the shortcuts sheet calls it "Quick capture" (glossary question 2).
Status: done

### S-006 Complete a task
As the user, I select a task and press Space, and it is done.
Screens: today.png, completed.png
Depends: S-004
Acceptance:
- [unit] `completionMovesSelectionToTheNextTaskThenThePrevious`.
- [unit] `mutationOpensUndo`: a toast says "Completed “…”" and offers Undo.
- [unit] `failedMutationShowsErrorAndKeepsUndoClosed`: a failed completion shows the server's error and offers no Undo.
- [manual] Clicking the circle, ⌘↩ and **Complete** in the Task menu do the same.
UX:
- Flow: select a task and press Space, or ⌘↩, or Task ▸ Complete, or right-click ▸ Complete, or click the circle in its row -> the row leaves its view, the selection moves to the next task (else the previous), a toast says "Completed “…”" with Undo and a countdown -> Undo, `u` or ⌘Z brings it back. Way back: Undo, for the length of the countdown.
- Surface: inline (changes one value on something already on screen), with no confirmation, because Undo covers it.
- States: done is the toast, the countdown and the toolbar's "Saved · undo 12s"; error is a red toast with the server's message and no Undo. In Completed, Space says "Completed tasks are read-only".
- Words: "Complete"; toast "Completed “<description>”"; "Undo" with its ⌘Z key.
- Access: the circle is a button labeled "Complete <description>"; hovering fills it with a check so the circle is not color alone; Space works from the list and the sidebar.
- Review: the circle has no hover text ("Complete this task (Space)"); the Edit menu item reads "Undo" where "Undo Complete Task" would name the action.
Status: done

### S-007 Know whether the server is reachable
As the user, I read the toolbar to learn whether my changes are reaching the server.
Screens: today.png
Depends: S-004
Acceptance:
- [unit] `serverStateFollowsWhatTheServerAnswers`.
- [manual] The toolbar reads "Connecting…", "Online", "Server unreachable" (click to try again) or "Signed out" (click to sign in), with its own symbol, and the hover text names the server.
- [manual] With the server down, a change fails with an error toast and the list keeps what it had; nothing is queued, and quitting never waits.
UX:
- Flow: the toolbar's status button always shows the state; click it to act: "Signed out" opens the sign-in sheet (S-002), any other state refreshes (S-008). Way back: none needed.
- Surface: inline, a toolbar item (changes nothing modal).
- States: "Connecting…" while the first load runs; "Online"; "Saved · undo 12s" counting down after a change; "Server unreachable" in the warning color with a triangle; "Signed out" in the warning color with a person symbol. It is hidden when no server is set.
- Words: hover texts "Sign in to <server>", "Momentum can’t reach <server>. Click to try again.", "Tasks are kept on <server>. Click to reload."
- Access: the state is a word, a symbol and a color, never color alone; VoiceOver reads the word.
- Review: the hover text says "reload" where the menu says "Refresh" (glossary question 3); VoiceOver hears only the state, with no hint that pressing it refreshes or signs in.
Status: done

### S-008 Refresh from the server
As the user, I press `r` and see what changed on the server.
Screens: today.png
Depends: S-004
Acceptance:
- [manual] `r`, ⌥⌘R and a click on "Online" reload, and a toast says "Refreshed from the server".
- [manual] The list reloads by itself every `refresh_interval` (60 seconds by default; `0s` turns it off) and keeps showing its rows while it does.
- [manual] Auto-refresh waits while a sheet is open.
UX:
- Flow: `r`, ⌥⌘R, View ▸ Refresh, or a click on the toolbar's status -> the list reloads in place -> a toast says "Refreshed from the server". The same reload runs by itself every `refresh_interval` without a toast. Way back: none needed; a refresh changes nothing on the server.
- Surface: inline, with a toast for the result.
- States: during a reload the rows stay; a failed manual reload is a red toast with the error and the rows stay; the first load failing is the full-window error in S-004.
- Words: "Refresh"; "Refreshed from the server".
- Access: a menu item with its shortcut; the toast is announced.
- Review: the error text comes from the bridge or the server; read each one aloud and make sure it says what to do (for example "Momentum can’t reach <server>. Check your connection.").
Status: done

## M2 Organize and edit

Group tasks, read them, change them, and remove them.

### S-009 Browse Inbox by project
As the user, I open Inbox and see my tasks under their projects.
Screens: inbox.png
Depends: S-004
Acceptance:
- [manual] Each project is a section with a folder symbol and its task count, and its full value when the section's title is shorter; tasks without a project have a section of their own.
- [manual] A task due or scheduled today, or overdue, is not in Inbox; the empty state says "No pending tasks" and offers **New Task**.
Status: done

### S-010 Read a task's details
As the user, I press Return on a task and read everything about it in the inspector.
Screens: inspector.png
Depends: S-004
Acceptance:
- [unit] `returnShowsAndHidesTheInspector`.
- [manual] The inspector shows project, priority, due, scheduled, estimate, tags, the repeat rule, what the task depends on, and its notes; technical fields sit in a folded section.
- [manual] With nothing selected it says "No Selection"; ⌃⌘I and the toolbar button show and hide it; double-clicking a row opens it.
Status: done

### S-011 Edit a task in the sheet
As the user, I press `e`, change fields, and save; only what I changed is sent.
Screens: edit.png, inspector.png
Depends: S-010
Acceptance:
- [unit] `editFieldErrorsReachTheEditor`: a value the bridge refuses shows under its own field, the other edits stay, and the sheet stays open.
- [manual] The sheet has description, project (with suggestions from the catalog), priority (None, Low, Medium, High), due, scheduled, estimate (1 minute to 24 hours), repeats and tags.
- [manual] **Save** is off until a field changes; Return or ⌘S saves and Esc cancels; a toast says "Updated “…”" and offers Undo.
- [manual] Clearing a field removes the value on the server.
Status: done

### S-012 Edit one field with its key
As the user, I press a letter and land in the edit sheet on that field.
Screens: edit.png
Depends: S-011
Acceptance:
- [unit] `actionKeysOpenTheirDialogsAndEditors`: `e` description, `E` estimate, `R` repeats, `p` project, `!` priority, `D` due, `S` scheduled, `t` tags.
- [manual] The same keys work from the sidebar; ⌘E opens the sheet on the description.
Status: done

### S-013 Start and stop a task
As the user, I press `s` on the task I am working on.
Screens: today.png, inspector.png
Depends: S-004
Acceptance:
- [manual] `s`, ⌘T and the inspector's **Start** and **Stop** toggle it, and a toast says "Started “…”" or "Stopped “…”" with Undo.
- [manual] An active task shows an "Active" badge in its row and "Active since" with the time in the inspector.
Status: done

### S-014 Delete a task
As the user, I press `d`, confirm, and the task is gone.
Screens: delete.png
Depends: S-004
Acceptance:
- [manual] The dialog asks "Delete “…”?", says how long the delete can be undone, and takes `y` to delete and `n` to cancel.
- [manual] For a task in a repeating series it says the other instances are not affected.
- [manual] A toast says "Deleted “…”" and offers Undo; the selection moves to the next task.
Status: done

### S-015 Stop a task repeating
As the user, I press `x` on a repeating task to end its series.
Screens: stop-repeating.png
Depends: S-004
Acceptance:
- [unit] `actionKeysOpenTheirDialogsAndEditors`.
- [manual] The dialog says the series ends, no new instances are made, and existing instances and history stay.
- [manual] On a task that does not repeat, `x` shows "This task doesn’t repeat".
Status: done

### S-016 Act on a task with the pointer
As the user, I right-click a task, or hover over it, and use its actions.
Screens: context-menu.png, today.png
Depends: S-004
Acceptance:
- [manual] The context menu on a pending task offers Complete, Start or Stop, Edit…, Stop Repeating… (only on a repeating task) and Delete….
- [manual] The hovered or selected row shows start, edit and delete buttons.
- [manual] On a completed task the context menu offers only **Show Details**.
Status: done

### S-017 Look back at Completed tasks
As the user, I open Completed to see what I finished in the last 30 days.
Screens: completed.png
Depends: S-006
Acceptance:
- [unit] `actionKeysSayCompletedTasksAreReadOnly`: Space, `s`, `d`, `x` and the edit keys say "Completed tasks are read-only".
- [manual] The subtitle reads "Last 30 days · read-only"; each row shows when the task was completed; the inspector says to undo the completion, while undo lasts, to change it.
- [manual] The empty state says "No recently completed tasks", and the app never opens on Completed by itself.
Status: done

## M3 Time

Dates, what is overdue, how long things take, and taking a change back.

### S-018 See Today in groups
As the user, I open Today and see Overdue, Due Today, Scheduled Today and Planned Today.
Screens: today.png
Depends: S-004
Acceptance:
- [manual] Each group is a section with its count; a task that fits several groups shows once, in the first of them, in that order.
- [manual] The subtitle shows today's weekday and date; the empty state says "Nothing due today" and offers **New Task** and **Open Inbox**.
- [manual] A task scheduled before today is not overdue; only its due date makes it so.
Status: done

### S-019 See how many tasks are overdue
As the user, I glance at the sidebar to see how many tasks are overdue.
Screens: today.png
Depends: S-018
Acceptance:
- [manual] A red count sits beside Today when tasks are overdue, its hover text says "N overdue", and it goes away at zero.
Status: done

### S-020 Read a row at a glance
As the user, I read a task's row to know when it is due, how long it takes and how important it is.
Screens: today.png, inbox.png
Depends: S-004
Acceptance:
- [manual] A row shows its due or scheduled date ("Yesterday", "N days overdue", a scheduled date with a calendar symbol), its estimate with a clock, and a priority mark; the date's color says whether it is overdue.
- [manual] A task with an estimate the app can't read shows "Estimate" with a warning instead of a value.
- [manual] A repeating task shows a repeat symbol and a task with notes a note symbol; `momentum-plan-` tags are not shown.
- [manual] In a window narrower than 600 points the project moves from the right into a line below the title.
Status: done

### S-021 Undo the last change
As the user, I press `u` or ⌘Z right after a change and it comes back.
Screens: today.png
Depends: S-006
Acceptance:
- [unit] `undoLastsForTheMutationDelay`, `undoClosesAWindowWhoseTickIsLate`, `aZeroMutationDelayOffersNoUndo`.
- [manual] After a change the toolbar reads "Saved · undo 12s" and counts down, the toast has an Undo button with a countdown ring, and `[sync].mutation_delay` (15 seconds by default) sets how long.
- [manual] After the window closes, `u` says "Undo is only available for 15 seconds after a change"; a successful undo says "Last change undone".
- [manual] ⌘Z undoes typing while a text field has focus, and the last change otherwise.
Status: done

## M4 Speed

Add and find tasks without leaving the keyboard.

### S-022 Add a task in plain language
As the user, I type "Prepare proposal tomorrow at 3pm, every Friday, about 1h" and the app finds the date, repeat and estimate.
Screens: capture.png
Depends: S-005
Acceptance:
- [unit] The Go interpreter in `tui/internal/quickadd` reads dates, repeats and estimates from plain language.
- [manual] Recognized phrases are underlined in their field's color as I type, and a "Saves as" line shows the description that will be kept.
- [manual] The sheet's placeholder shows an example sentence.
Status: done

### S-023 Add a task with triggers
As the user, I type `#work.client !high @tomorrow +planning ~1h30m ^weekly` and each part lands in its field.
Screens: capture.png
Depends: S-005
Acceptance:
- [unit] `tui/internal/quickadd`: a trigger counts only at the start of a word; `bob@example.com` stays text; a repeated project, priority, due, scheduled or estimate trigger is refused.
- [manual] `\#launch` keeps the `#` as text.
- [manual] The footer lists the triggers: `#` project, `!` priority, `@` due, `>` scheduled, `+` tag, `~` estimate, `^` repeat.
Status: done

### S-024 Accept a suggestion
As the user, I type `#wo`, press ⇥, and the project is completed.
Screens: capture-suggestions.png
Depends: S-023
Acceptance:
- [manual] Suggestions list projects, tags, priorities and dates; ↑ ↓ choose, ⇥ accepts and Return adds the whole task.
- [manual] Esc closes suggestions first; on a highlighted phrase it keeps the phrase as text; otherwise it closes the sheet.
Status: done

### S-025 Review unclear input before adding
As the user, when the app can't read my input one way, I fix it before the task is added.
Screens: capture-review.png
Depends: S-022
Acceptance:
- [unit] `reviewDatesIgnoreEmptyValues`, and the capture review tests in `tui/internal/bridge`.
- [manual] The sheet says "Review before adding" with each field, **Keep as Is** per field, **Edit Text**, **Add Using Triggers Only**, **Add Task** and **Cancel**.
- [manual] Nothing is added until I choose one of the add buttons.
Status: done

### S-026 Move and switch views with the keyboard
As the user, I use `j` `k` and the number keys instead of the mouse.
Screens: today.png
Depends: S-004
Acceptance:
- [unit] `movementKeysStayInsideTheList`: `j` `k` ↓ ↑ `g` `G` stop at the first and last task.
- [unit] `numberKeysOpenTheViewsInTheTUIOrder`: `1` Inbox, `2` Today, `3` Completed, and ⌘1 to ⌘3 from the View menu.
- [unit] `keysWaitForOverlaysAndLeaveCommandShortcutsToTheMenus`: single keys do nothing while a sheet is open or a text field has focus.
Status: done

### S-027 Search the current view
As the user, I press `/` and narrow the list by typing.
Screens: search.png
Depends: S-004
Acceptance:
- [unit] `fuzzyTextCoversDescriptionProjectAndTags`, `projectTermIsExactAndCaseInsensitive` (`project:work`, `project:none`), `scoresMatchTheTUI`, `searchFiltersSectionsAndRepairsSelection`.
- [unit] `slashFocusesSearchAndEscClearsIt`: `/` and ⌘F focus the field; Return keeps the filter and hands focus to the list; Esc clears it.
- [manual] The subtitle reads "N of M match"; nothing found shows the search empty state.
Status: done

### S-028 See every shortcut
As the user, I press `?` and see every shortcut in groups.
Screens: shortcuts.png
Depends: S-004
Acceptance:
- [manual] `?`, ⇧⌘/ and the sidebar's **Keyboard Shortcuts** row open the sheet; Navigate, Act on selection, Edit a field, Quick capture, Plan Today and App are listed, plus the capture triggers.
- [manual] Return or **Done** closes it.
Status: done

### S-029 Run task commands from the menu bar
As the user, I use the Task menu and ⌘ shortcuts instead of single keys.
Screens: menu.png
Depends: S-004
Acceptance:
- [manual] The Task menu has Complete (⌘↩), Start or Stop (⌘T), Edit… (⌘E), Stop Repeating… and Delete…; New Task is ⌘N and Refresh is ⌥⌘R.
- [manual] Items are off when no pending task is selected, while a sheet is open, or while signed out.
Status: done

## M5 Plan the day

Choose today's work against the hours the day really has.

### S-030 Open Plan Today
As the user, I press `P` and the planning sheet opens.
Screens: plan-today.png, today.png
Depends: S-018
Acceptance:
- [unit] `shiftPOpensPlanTodayFromTheList`, `planTodayWaitsForASignInAndRespectsPlanningOff`.
- [manual] `P`, ⇧⌘P, the **Plan** toolbar button and the sidebar's **Plan Today** button open it; all four are off while signed out.
- [unit] `shiftPIsNoShortcutWithPlanningOff`: `[planning] enabled = false` hides the button, the box, the menu item and the key.
Status: done

### S-031 Tick the tasks I will do today
As the user, I tick tasks in the sheet and see what is left of the day.
Screens: plan-today.png
Depends: S-030
Acceptance:
- [unit] `tickingSumsTheChosenEstimatesAndWarnsOverCapacity`: the column adds up the estimates of ticked tasks and warns, never blocks, when the day goes over.
- [unit] `movementSkipsObligationsAndStopsAtTheEnds`, `spaceTicksTheRowUnderTheCursor`: ↑ ↓ or `j` `k` move over the rows I can tick, Space ticks, `g` `G` go to the first or last row.
- [manual] Rows say why a task is in its group; tasks planned on an earlier day and still open start ticked; tasks without an estimate are counted apart, never as zero.
- [unit] `minutesReadAsTheTUIShowsThem`.
Status: done

### S-032 Keep today's obligations in the plan
As the user, I can't untick what is overdue or due or scheduled today.
Screens: plan-today.png
Depends: S-031
Acceptance:
- [unit] `obligationsCannotBeUnticked`.
- [manual] Obligations show as fixed rows that the cursor skips, and their minutes come off the capacity before anything else.
Status: done

### S-033 Filter the candidates
As the user, I press `/` in the sheet and narrow the candidates.
Screens: plan-today-filter.png
Depends: S-031
Acceptance:
- [unit] `slashAndCommandFFocusTheFilter`, `theFilterNarrowsTheCandidatesAndKeepsTheirTicks`.
- [unit] `escapeCancelsAndReturnConfirms`: in the filter, Return goes back to the list and Esc clears the filter; in the list, Return confirms and Esc cancels.
Status: done

### S-034 Confirm the plan
As the user, I press Return and the plan is saved.
Screens: plan-today.png, today.png
Depends: S-031
Acceptance:
- [unit] `confirmingClosesTheSheetAndOffersNoUndo`: a toast says "Daily plan saved · N tasks" with no Undo, and ⌘Z can't revert one task's tag.
- [unit] `anUnchangedPlanSaysSo`: an unchanged plan closes the sheet with "Daily plan unchanged".
- [manual] The plan writes only `momentum-plan-YYYY-MM-DD` tags, never dates; reopening Plan Today is how a plan changes.
- [unit] `listKeysWaitWhileThePlanIsOpen`: the list's single keys wait while the sheet is open.
- [manual] Auto-refresh waits too, so the rows don't move under my ticks.
Status: done

### S-035 Finish a plan that stopped partway
As the user, when a save fails halfway, I confirm again and the rest is saved.
Screens: plan-today-partial.png
Depends: S-034
Acceptance:
- [unit] `aPartialSaveKeepsTheSheetAndItsTicks`: the sheet stays open with my ticks and says "Saved k of n changes. Confirm again to finish — nothing is applied twice."
- [unit] `aFailedSaveKeepsTheSheetOpen`.
- [manual] A task completed on another device while the sheet was open is skipped, not an error.
Status: done

### S-036 See today's plan in the sidebar
As the user, I glance at the sidebar and see how full the day is.
Screens: today.png
Depends: S-031
Acceptance:
- [manual] The **Today’s plan** box shows how many tasks are chosen, a meter of obligations, calendar time, buffer and chosen work against the day's capacity, the hours the tasks take ("2h of tasks"), and what is left ("1h free", or "30m over" in red).
- [manual] The hover text says how many tasks have no estimate and aren't counted.
- [manual] The count includes only tasks tagged for today outside the obligations.
Status: done

### S-037 See my calendar in the plan
As the user, I see how much of the day my meetings take.
Screens: plan-today.png
Depends: S-031
Acceptance:
- [unit] `calendarTimeComesOffWhatIsAvailable`, `decodesThePlanItsCalendarAndTheSummary`, and the calendar tests in `tui/internal/bridge`: busy time inside working hours counts against capacity, and overlapping events count once.
- [manual] The sheet shows the merged blocks, with their titles on hover; with `[calendar]` off there is no strip.
- [manual] Event titles are never stored or logged.
Status: done

### S-038 Keep planning when the calendar can't be read
As the user, when a calendar file is missing or broken, I plan with tasks alone and know why.
Screens: plan-today-calendar-unavailable.png
Depends: S-037
Acceptance:
- [unit] The bridge tests for missing and stale files: one unreadable file makes the whole calendar unavailable and its time counts as zero.
- [manual] The sheet says "Calendar unavailable:" with the reason, and ticking, saving and the sidebar meter work.
Status: done

## M6 Settings and checks

Change how the app works without editing `config.toml`. Plan: [`docs/plans/macos-settings.md`](./plans/macos-settings.md). Story to task: S-040 harness; S-041 and S-042 C02; S-043 and S-044 C01 and C03; S-045 and S-046 C04; S-047 C05; S-048 to S-052 C06 and C07; S-053 C08; S-054 and S-055 C01. Task C09 (README) goes with the last story.

### S-040 See every page in light and dark
As the developer, I run one command and get a window snapshot of every page, in both looks.
Screens: every page named in this file that the app already has
Acceptance:
- [self] `macos/tools/snap.sh` builds the Debug app and saves a 1120×720 window image per page and look, `<page>-light.png` and `<page>-dark.png`, in `/tmp/Momentum-shots`. It takes every page named under `Screens:` in M1 to M5, and the states the `UX:` blocks name: `loading`, `load-error`, `startup-error`, `signed-out`, `sign-in-failed`, `today-empty`, `inbox-empty`, `completed-empty`, `undo` and `unreachable`.
- [self] A story's `[snap]` lines can be checked by looking at its page. Each Settings story adds its own page to `SnapshotScenes.swift` when it is built.
- [unit] `everyPageRunsOnTheFakeBridgeAndAnInMemorySignInStore`: every page runs on a fake bridge and an in-memory sign-in store, so a run reaches no server and reads no `config.toml` or Keychain item. The run has its own bundle identifier, so the real app's preferences and saved window state stay as they are.
- [unit] `aFailedBuildStopsTheScript`: the script ends with the build's exit status and takes no snapshot when the build fails.
- [manual] The menus (`context-menu.png` in S-016, `menu.png` in S-029) are not app windows, so the harness skips them; try them by hand.
Status: done

### S-041 Open Settings
As the user, I press ⌘, and a Settings window opens with Projects, Planning, Calendar, Appearance and Diagnostics.
Screens: settings-projects.png, settings-reopened.png
Depends: S-040
Acceptance:
- [snap] `settings-projects`: a window 760 points wide with 600 points of content under the toolbar tabs Projects, Planning, Calendar, Appearance and Diagnostics, in that order.
- [self] `tools/snap.sh` checks both entry points: the app menu has **Settings…** with ⌘, and it opens the window, and a click on the sidebar's **Settings** row opens it too. The row sits above **Keyboard Shortcuts** and shows ⌘,.
- [snap] `settings-reopened`: closing the window and opening it again keeps its tabs.
- [manual] Appearance applies at once (S-042). The rule that an invalid value shows inline and is not saved belongs to each tab's story, since no other tab has a value to change yet.
UX:
- Flow: ⌘,, Momentum Task Manager ▸ Settings…, or the **Settings** row in the sidebar footer -> the Settings window opens on the tab used last (Projects the first time), or comes forward if it is already open -> click a tab to switch -> ⌘W closes it. Every change in a tab applies at once, so there is nothing to save and nothing lost by closing.
- Surface: new window (a self-contained place people keep open beside the main window), a `Settings` scene with toolbar tabs in the order Projects, Planning, Calendar, Appearance, Diagnostics, 760 points wide with 600 of content, not resizable. The sidebar row is a shortcut to it.
- States: Appearance is built (S-042). The other four tabs show a placeholder until their stories land: a symbol, "<Tab> isn’t in Settings yet", "For now, edit `[planning]` in `config.toml`. Changes apply without restarting." (Projects names the `[[projects]]` entries, Calendar `[calendar]`; Diagnostics: "For now, run `momentum doctor` in a terminal."), and a **Show config.toml in Finder** button. A config problem or a changed `[backend] url` adds the banner from S-054 above every tab.
- Words: "Settings" (Apple's word, in the menu and the sidebar row, with ⌘, beside it as a key cap), tab names "Projects", "Planning", "Calendar", "Appearance", "Diagnostics", button "Show config.toml in Finder".
- Access: the system gives the Settings window its ⌘, shortcut and ⌘W; the tabs are in the window's toolbar and reachable with the keyboard; the window title is the selected tab's name; the sidebar row is a button with the visible label "Settings".
Status: done

### S-042 Choose the Mac's appearance
As the user, I pick System, Light or Dark and the whole app follows at once.
Screens: settings-appearance.png, today.png, appearance-light.png, appearance-dark.png, appearance-dark-delete.png
Depends: S-041
Acceptance:
- [unit] `eachChoiceMapsToAColorScheme`, `theChoicesReadSystemLightDark`: the saved choice maps to the color scheme, and System leaves it to the Mac.
- [snap] `settings-appearance` shows the three choices as cards, System split light and dark; `appearance-light` and `appearance-dark` show Today in each, whatever the Mac is set to; `appearance-dark-delete` shows that a dialog follows the choice.
- [unit] `theChoiceIsKeptOnThisMacUnderItsOwnKey`: the choice is kept in this Mac's preferences under `appearance`; the terminal app's `theme` key is neither read nor written.
UX:
- Flow: Settings ▸ Appearance -> choose System, Light or Dark -> the main window, its sheets and dialogs, and the Settings window change at once -> the choice is kept on this Mac and is there at the next launch. Way back: choose the earlier one; there is no Save, and no Undo is needed.
- Surface: inline (a row of three choice cards, each a button, that changes one value on something already on screen), in the Appearance tab.
- States: System is the default and follows the Mac's own light or dark setting, including when it changes while the app is open. The chosen card has an accent border and a bold word. There is no empty, loading or error state.
- Words: heading "Appearance", choices "System", "Light", "Dark", note "System follows your Mac’s setting. This applies to this Mac only; the terminal app keeps its own theme."
- Access: each card is a button labeled with its choice and reported as selected when it is; the words, not the pictures, say which is which; with Full Keyboard Access, Tab reaches the cards and Space chooses one.
Status: done

### S-043 Turn planning on or off
As the user, I flip a switch and Plan Today appears or goes away.
Screens: settings-planning.png, settings-planning-off.png, today-planning-off.png
Depends: S-041, S-054
Acceptance:
- [unit] Go `TestASavedPlanningChangeAppliesAtOnce`, Swift `turningPlanningOffHidesPlanTodayAndOnBringsItBack`: the switch saves to `[planning] enabled` at once, on the version of the file the tab read, and the main window follows without a restart.
- [snap] `settings-planning-off` shows the capacity fields dimmed; `today-planning-off` shows the main window with no Plan button and no Today’s plan box.
- [unit] The menu item and `P` follow the same flag: `planTodayWaitsForASignInAndRespectsPlanningOff`, `shiftPIsNoShortcutWithPlanningOff`.
- [unit] Swift `aWriteThatFailedSaysWhy`: a failed write leaves the switch where it was and says why.
UX:
- Flow: Settings ▸ Planning -> switch **Use Plan Today** on or off -> it saves to `[planning] enabled` at once -> in the main window the Plan button, the sidebar's Today’s plan box, the Plan Today… menu item and the P key appear or go within a second, with no restart. Way back: flip the switch again.
- Surface: inline (a switch changes one value on something already on screen), the first row of the Planning tab.
- States: on by default. Off: the capacity fields below dim and cannot be edited, and nothing else changes. A save that fails shows a notice at the top of the tab, with the switch back where it was: "Unable to save config.toml. <reason>". A save refused because the file changed elsewhere reloads the tab and says "config.toml changed outside Momentum. The tab now shows what it says; make the change again." (S-055).
- Words: switch "Use Plan Today", caption "Adds the Plan button, the Today’s plan box in the sidebar, the Plan Today… menu item and the P key."
- Access: the switch is labeled with its text and VoiceOver reads "Use Plan Today, on"; Space toggles it; the error is text with a warning symbol.
Status: done

### S-044 Set how much a day holds
As the user, I type `8h` for the day and `7h30m` for Fridays, and the plan uses them.
Screens: settings-planning.png, settings-planning-error.png
Depends: S-043
Acceptance:
- [unit] Go `TestPlanningPatchRefusesWhatTheConfigRefuses`, `TestAnInvalidLengthIsRefusedOnItsFieldAndNothingIsWritten`, `TestAnInvalidPlanningPatchWritesNothing`: a value the config would refuse is refused on its own field, with a message that says what to type, and nothing is written, not even the valid keys.
- [unit] Swift `aRefusedLengthStaysOnItsFieldAndChangesNothing`; [snap] `settings-planning-error`: the typed value stays in its field with the message under it (a weekday's message sits under the grid and names the day), and the harness's Return shows that Return saves.
- [unit] Go `TestAnEmptyWeekdayGoesBackToTheDailyCapacity`: an empty weekday field means the daily capacity.
- [snap] `settings-planning`: Daily capacity, Buffer, and a grid of seven weekday fields, Monday first, with the daily capacity as their placeholder.
- [unit] Go `TestASavedPlanningChangeAppliesAtOnce`: the next load, and so the sidebar's meter, uses the new capacity.
- [manual] Leaving a field with Tab or a click saves it too; not tried by hand yet.
UX:
- Flow: Settings ▸ Planning -> click Daily capacity, Buffer or a weekday, type a length of time such as 8h or 7h30m -> Return, or Tab or a click elsewhere, saves it -> the sidebar's meter uses it on the next load, which the app asks for right after the save. A value that isn't valid stays in the field with its message and is not saved. Way back: type the earlier value; emptying a weekday field makes that day use the daily capacity.
- Surface: inline text fields in grouped rows (each changes one value), with the weekdays as a grid of seven labeled fields, Monday first.
- States: a weekday field's placeholder shows the daily capacity ("8h"); errors under the field for Daily capacity and Buffer, and under the grid for a weekday (naming the day, so the grid never changes height), with the refused field outlined: "Enter a length of time, such as 8h or 7h30m.", "Daily capacity must be longer than zero.", "Buffer can’t be negative.", "A day must be longer than zero. Leave it empty to use the daily capacity."; a saved value just stays, with no toast, since the field is the feedback; the fields dim while Plan Today is off.
- Words: group "Capacity", labels "Daily capacity", "Buffer" and "Capacity by weekday", the weekday names, hint "Type a length of time, such as 8h or 7h30m. Leave a day empty to use the daily capacity."
- Access: every field has a visible label; Return saves and Tab moves on and saves; VoiceOver reads the label, the value and any message under it; a message is text, never color alone.
Status: done

### S-045 Choose calendar files
As the user, I add and remove `.ics` files and see whether each can be read.
Screens: settings-calendar.png, settings-calendar-empty.png, settings-calendar-refused.png
Depends: S-041, S-054
Acceptance:
- [unit] Go `TestCalendarFilesAreKeptWithATildeAndOnce`, `TestCalendarFilesAreKeptWithATildeAndTheSwitchNeedsOne`; Swift `calendarFilesAreAddedAndRemovedAsAWholeList`: a file under the home folder is stored with `~/`, a repeat once, and **Remove** takes one out of the list.
- [manual] **Add…** opens the system file panel for `.ics` files; not tried by hand yet.
- [unit] Go `TestInspectSaysWhatEachFileIs`, `TestSettingsShowTheValuesInUseAndEachCalendarFile`: each file's status (available, stale, missing, error) comes from loading it on its own, from fixtures, and holds no path and no event title.
- [unit] Go `TestCalendarPatchRefusesWhatTheConfigRefuses`, Swift `switchingCalendarFilesOnWithNoneIsRefusedOnTheSwitch`: switching on with no files, or removing the last one while on, is refused with a message; [snap] `settings-calendar-refused`.
- [snap] `settings-calendar` lists the four states, each a symbol and a word; `settings-calendar-empty` shows the empty list.
UX:
- Flow: Settings ▸ Calendar -> **Add…** opens a file panel for `.ics` files -> choose one or more -> each appears in the list with its status -> switch **Use calendar files** on -> Plan Today counts their meetings against the day's capacity. **Remove** takes a file out of the list; the file itself stays. Way back: Remove, switch off, or Esc to leave the panel.
- Surface: an inline list in the Calendar tab; **Add…** opens the system open panel as a sheet on the Settings window (one short task that must finish or cancel).
- States: empty list "No calendar files yet" with "Export a calendar to an .ics file, then add it here. Momentum reads the file; it never fetches a calendar for you." and **Add…**; each row shows the path (`~/` for a file under the home folder, in monospace) and a status as a symbol and a word: "Available", "Stale · last changed 3 days ago", "Missing", "Unreadable: <reason>"; the switch turned on with no files stays off with "Add a calendar file first." beneath it; removing the last file while the switch is on is refused with "Turn off Use calendar files before removing the last one."; a failed save shows "Unable to save config.toml. <reason>" in the notice at the top of the tab.
- Words: switch "Use calendar files", caption "Meetings in these files take time off each day’s capacity in Plan Today.", group "Calendar files", buttons "Add…" and "Remove", Remove's hover text "Remove this file from the list. The file itself stays."
- Access: each row reads as one element, "<path>, <status>"; **Remove** is labeled "Remove <path>"; status is a word and a symbol, never color alone; the panel is the system's own and keyboard-complete.
Status: done

### S-046 Say which hours and events count
As the user, I set my working hours and whether all-day and "free" events count as busy.
Screens: settings-calendar.png, settings-calendar-empty.png
Depends: S-045
Acceptance:
- [unit] Go `TestSavedWorkingHoursAndEventChoicesApplyAtOnce`, `TestMovingBothWorkingHoursInOneSaveIsCheckedAsAPair`, `TestSaveCalendarWritesHoursEventsAndTheStaleLimit`; Swift `workingHoursEventChoicesAndTheStaleLimitSaveAtOnce`: working start and end, **Count all-day events** and **Count events shown as free** save to `[calendar]` at once, keeping comments and other keys, and the next load's busy minutes follow them.
- [unit] Go `TestAnInvalidCalendarValueIsRefusedOnItsFieldAndNothingIsWritten`, `TestCalendarPatchChecksHoursAgainstWhatIsThere`; Swift `hoursThatEndBeforeTheyStartAreRefusedAndTheFieldsKeepTheFilesValues`: an end that is not after the start, a time that is not one, and a length that is not one or is negative are refused on their own field, with a message that says what to do, and nothing is written, not even a valid switch in the same save.
- [unit] Go `TestTheStaleLimitChangesWhatEachFileSays`, `TestSettingsShowTheCalendarDefaultsForKeysTheFileLeavesOut`: Stale after changes each file's state, `0m` means never, and a key the file leaves out shows its default (09:00, 17:00, both switches off, 24h). It touches nothing but a file's state, so it applies to files only; S-047's events carry no file age.
- [unit] Swift `aTimeFieldAndTheConfigTradeInHHMM`, `theCalendarHoursAndEventChoicesDecode`: a time field and `config.toml` trade in HH:MM, and the bridge's answer decodes.
- [snap] `settings-calendar`, `settings-calendar-empty`: the **Stale after** row under the files, then the Working hours and Events that count groups; with four files listed the last group is below the fold, and the tab scrolls.
- [manual] Changing **Start** or **End** by hand, and that a refused End puts the field back on the file's value; not tried by hand yet, since the snapshot harness cannot drive a time field.
UX:
- Flow: Settings ▸ Calendar -> change **Start** or **End** under Working hours, flip **Count all-day events** or **Count events shown as free**, or type a length in **Stale after** -> each saves to `[calendar]` at once -> the app loads again right after the save, so the sidebar's meter and the next Plan Today use it. A value the config would refuse is not saved and says why. Way back: set the earlier value.
- Surface: inline (each control changes one value on something already on screen), in the Calendar tab. Working hours and the two switches sit in their own groups, because they apply to every source; Stale after sits in the Calendar files group, because it applies to files only.
- States: defaults are 09:00 to 17:00, both switches off, Stale after 24h; a missing key shows its default. The hours are time fields that cannot hold a bad format. An End that is not after the Start is refused under the group, "Working hours must end after they start.", and the fields show what the file has. Stale after takes a length ("Enter a length of time, such as 8h or 7h30m.", "Stale after can’t be negative."); `0m` means a file is never Stale. A saved value just stays, with no toast. A failed write or a changed file shows the notice at the top of the tab, as in S-043. The controls stay usable while Use calendar files is off, because the values apply to every source.
- Words: groups "Working hours" and "Events that count"; labels "Start", "End", "Count all-day events", "Count events shown as free", "Stale after"; captions "Only meetings between these hours take time off a day’s capacity. This applies to every calendar source.", "An all-day event takes the whole working day off the capacity.", "Events your calendar marks as free, such as a reminder, count as busy too.", "A calendar file that hasn’t changed for this long shows as Stale. Calendar files only; 0m means never."
- Access: each control has its visible label, and VoiceOver reads the label and the value ("Start, 9:00 AM"); Return saves a typed length and Tab moves on and saves; a message is text with a warning symbol, never color alone.
Status: done

### S-047 Use the Mac's Calendar instead of files
As the user, I choose macOS Calendar as my source and pick which calendars count.
Screens: settings-calendar-mac.png, settings-calendar-denied.png, plan-today-calendar-denied.png
Depends: S-046
Acceptance:
- [unit] Swift `choosingMacOSCalendarAsksTheMacOnceAndLoadsAgain`: the Calendar tab has a **Source** picker with Calendar files and macOS Calendar; choosing macOS Calendar asks the Mac for access the first time, and the next load uses the Mac's events.
- [snap] `settings-calendar-denied`: denied, the tab says so and offers **Open System Settings…** (Privacy & Security ▸ Calendars); `plan-today-calendar-denied`: Plan Today says "Calendar unavailable: no access". Unit: Swift `aMacThatSaysNoLeavesNoAccessForLoadAndPlan`, `comingBackFromSystemSettingsWithAccessLoadsAgain`, Go `TestCalendarsTheAppCouldNotReadMakeTheCalendarUnavailable`.
- [snap] `settings-calendar-mac`: granted, calendars are listed by account with a switch each.
- [unit] Go `TestEventsFromTheMacGiveTheSameBusyTimeAsTheSameEventsInAFile`, `TestTheHoursAndEventChoicesApplyToTheMacsEventsToo`, `TestFromEventsNarrowsToTheDayAndIsNeverStale`: events handed to the bridge give the same busy minutes and blocks as the same events read from a file, and the working hours and event choices apply to them; Swift `titlesGoOnlyWithAPlan` and Go `TestTitlesComeBackOnlyWhenTheAppSendsThem`: titles go only with `plan`; Swift `eventTimesAreSentAsISO8601`.
- [unit] Swift `choosingMacOSCalendarNeverChangesTheConfigFile`, Go `TestTheMacCalendarNeedsNeitherTheFilesSwitchNorTheirPaths`: choosing macOS Calendar never changes `[calendar] enabled` or `paths`, and the bridge counts the Mac's events whatever they say; Swift `aCalendarSwitchedOffSendsNoEventsAndIsRemembered`: the source and the calendars that are off stay in this Mac's preferences, under `calendar.source` and `calendar.mac.off`.
- [manual] Run once with access granted and once denied, with a real calendar; not tried by hand yet. The harness and the tests use a fake event source, so no calendar is read and the Mac's permission prompt never appears. The usage description (`NSCalendarsFullAccessUsageDescription`) is in the built Info.plist.
UX:
- Flow: Settings ▸ Calendar -> **Source** ▸ macOS Calendar -> the Mac asks for access to calendars the first time -> the tab lists the calendars by account, each with a switch -> switch off the ones that should not count -> the app loads again at once, so the sidebar's meter and the next Plan Today use them. Denied: the tab says so and **Open System Settings…** opens Privacy & Security ▸ Calendars; coming back to the app with access given loads again. Way back: **Source** ▸ Calendar files.
- Surface: inline (a picker and switches change values on something already on screen), in the Calendar tab. The Mac's own permission alert is the system's; **Open System Settings…** leaves the app.
- States: not asked yet ("Momentum Task Manager asks your Mac for access to your calendars. It only reads the times of today’s meetings." and **Allow Access…**); denied (a lock symbol and "Momentum Task Manager can’t read your calendars", "Allow it in System Settings ▸ Privacy & Security ▸ Calendars. Until then, Plan Today shows “Calendar unavailable: no access”.", **Open System Settings…**); granted with calendars (account names as small headings, a switch per calendar, all on at first, a calendar added later counts until it is switched off); granted with none ("No calendars found"). The calendar files groups hide while the source is macOS Calendar, and come back, as they were, when it is Calendar files. Working hours and Events that count apply to both sources. A cancelled meeting is not counted.
- Words: picker "Source" with "Calendar files" and "macOS Calendar", caption "This applies to this Mac only. The terminal app keeps using the calendar files.", group "Calendars", caption "Meetings in the calendars that are on take time off each day’s capacity in Plan Today. Momentum Task Manager only reads today’s events; it never changes your calendar.", buttons "Allow Access…" and "Open System Settings…".
- Access: the picker and each switch are labeled with their text; the denied state is a symbol and words, never color alone; **Open System Settings…** and **Allow Access…** are buttons reachable with Tab.
Status: done

### S-048 Browse my projects
As the user, I open Projects and see my project tree.
Screens: settings-projects.png, settings-projects-empty.png
Depends: S-041
Acceptance:
- [snap] `settings-projects`: one list shows the catalog projects and the values found only on tasks, parents before their children and indented by depth, each with its name, its full value in monospace, a "Catalog" or "From tasks" tag and its task count. The tag is a word, filled for the catalog and outlined for a project from tasks.
- [unit] Go `TestProjectsListTheCatalogAndTheValuesFoundOnlyOnTasks`, `TestProjectsKeepTheCatalogWhenTheTasksCannotBeRead`; Swift `theListMirrorsWhatTheBridgeReturns`, `theBridgesAnswersDecodeAndAnEditEncodesWhatItReads`: the list mirrors what the `projects` operation returns, with a fake bridge; a task count is the pending tasks with exactly that value, and `workshop` is not a child of `work`; when the tasks cannot be read the catalog still shows, without counts, and the tab says why.
- [snap] `settings-projects-empty`: no projects yet, with **Add Project…**.
- [manual] The list in the real app, with the real server; not tried by hand yet.
UX:
- Flow: Settings ▸ Projects -> the list of every project -> a catalog row has an actions menu (**Edit…**, **Remove…**, also in its context menu) and a "From tasks" row has **Add to Catalog…** -> **Add Project…** under the list adds one. The tab reads the catalog again each time Settings is shown.
- Surface: inline list in the Projects tab (a place people come back to). Every change opens its own sheet or prompt (S-049 to S-052); nothing on the tab is a draft.
- States: empty "No projects yet" with "Add a project to the catalog so New Task suggests it. Projects your tasks already use show here too." and **Add Project…**; loading is a spinner; a task count reads "No tasks", "1 task" or "N tasks"; when the tasks cannot be read, "Task counts unavailable: <reason>" in the notice at the top and the counts are left out; a file that changed elsewhere or a failed save shows its notice there too; a long value truncates in the middle with the full value on hover.
- Words: group "Projects", tags "Catalog" and "From tasks", counts "No tasks", "1 task", "N tasks", footer "The catalog is what New Task suggests. A project from tasks is one your tasks use that the catalog doesn’t have.", buttons "Add Project…", "Add to Catalog…" and the menu items "Edit…" and "Remove…".
- Access: each row reads as its name, value, tag and count; the actions menu is labeled "Actions for <name>" and has the hover text "Edit or remove this project"; the tag is a word, never color alone; every action is also in the row's context menu.
Status: done

### S-049 Add or edit a catalog project
As the user, I add `work.client` under `work`, or change its name.
Screens: settings-project-sheet.png
Depends: S-048
Acceptance:
- [snap] `settings-project-sheet`: the sheet has a name, a parent picker, the value and the full resulting value, which is the parent and the value joined by a dot.
- [unit] Go `TestAnInvalidProjectDraftIsRefusedOnItsFieldAndNothingIsWritten`, `TestAddingAnEntryComposesItsValueAndKeepsTheFilesComments`, `TestChangingOnlyTheNameNeverOffersOrMovesATask`; Swift `aRefusedDraftStaysInTheSheetOnItsField`, `changingOnlyTheNameSavesWithoutAPreview`, `anEntryCannotMoveUnderItselfOrWhatIsBelowIt`: an invalid draft writes nothing and says why on its own field; a catalog-only edit never changes a task and offers none.
- [unit] Swift `addingSendsTheDraftOnTheRevisionItReadAndLoadsAgain`: the catalog saves from its own sheet, on the version of the file the tab read, and the tab and the main window, whose suggestions come from the catalog, read it again.
- [manual] Add a project and use it at once in New Task; not tried by hand yet.
UX:
- Flow: **Add Project…**, or **Edit…** on a row -> the sheet -> type a name, choose a parent (or None) and type the value; the full value shows as you type -> Return saves and the sheet closes; Escape cancels. An edit that changes the full value goes on, in the same sheet, to the rename step (S-051). Way back: Escape, or edit the project again.
- Surface: sheet (one short task that must finish or cancel), 460 points wide. It never opens another sheet: the rename step replaces its content.
- States: add starts empty with the primary button **Add Project**; edit starts with the entry's values and the primary button **Save**, which reads **Review Rename…** once the full value differs; a refused field is outlined by its message, "Enter a name.", "Use lowercase letters, numbers and hyphens, such as client-name.", "Choose a parent from the list.", "“work.client” is already in the catalog.", "A project can’t move under itself."; the parent list holds every project, except, when editing, the entry itself and what is below it; a failed write shows its reason in the sheet; a file that changed elsewhere closes the sheet and says so on the tab (S-055).
- Words: titles "Add Project" and "Edit Project", fields "Name", "Parent" (with "None"), "Value" and "Full value", buttons "Cancel", "Add Project", "Save", "Review Rename…".
- Access: each field has its visible label, and Return confirms and Escape cancels; a message is text with a warning symbol; the full value can be selected and copied.
Status: done

### S-050 Remove a project
As the user, I remove a project and say what happens to its children.
Screens: settings-project-remove.png
Depends: S-048
Acceptance:
- [snap] `settings-project-remove`: the prompt names the project and how many projects of the catalog are under it.
- [unit] Go `TestRemovingWithAndWithoutChildrenLeavesTheCatalogAsChosen`; Swift `removingAsksAboutTheChildrenAndSendsTheAnswer`: removing with and without children each leave the catalog as the choice says, comments and other entries stay, and removing never changes a task.
UX:
- Flow: **Remove…** on a row -> the prompt -> **Remove Only “Work”** or **Remove “Work” and 2 Projects Under It** (with no children, one **Remove**) -> the entry goes. Cancel, or Escape, changes nothing. Way back: add the project again.
- Surface: confirmation dialog (it removes a catalog entry, which a person asks about before it goes).
- States: with children, "It has 2 projects under it in the catalog. Tasks keep their project; removing never changes a task."; without, only the second sentence. Only catalog entries count as children, not values found only on tasks. A project tasks still use stays in the list as "From tasks". A failed write or a changed file shows its notice on the tab.
- Words: title "Remove “Work”?", buttons "Remove Only “Work”", "Remove “Work” and 2 Projects Under It", "Remove", "Cancel".
- Access: the system's own dialog, keyboard-complete; the destructive buttons are marked as such to VoiceOver.
Status: done

### S-051 Rename a project and move its tasks
As the user, I rename `work` to `job` and choose whether my pending tasks follow.
Screens: settings-project-rename.png, settings-project-rename-merge.png
Depends: S-049
Acceptance:
- [snap] `settings-project-rename`: the second step offers "Catalog only" (the default) and "Catalog + pending tasks", with "Include subprojects", the counts and the note that completed and deleted tasks keep their values; `settings-project-rename-merge`: the warning that tasks already use the new value, and the agreement that must be ticked first.
- [unit] Go `TestAPreviewSaysWhichPendingTasksARenameMoves`, `TestIncludingSubprojectsRenamesTheCatalogDescendantsAndMovesTheirTasks`, `TestAValueTasksAlreadyUseIsAMergeWarning`, `TestARenameWithNoMatchingTasksIsAValidCatalogSave`: `workshop` is not matched by `work`, a repeating task and a task under another status are left alone, the catalog and task counts are separate, and zero matches is a valid save.
- [unit] Go `TestTheCatalogIsSavedBeforeTheTasksMove`, `TestARefusedMigrationKeepsTheSavedCatalog`, `TestAStalePreviewIsRefusedAndNothingIsWritten`; Swift `aStalePreviewGoesBackToTheFormAndMovesNothing`, `aRefusedMigrationKeepsTheSheetOpenWithTheReason`: the catalog saves before the migration; a refused migration keeps the saved catalog; a stale preview is refused and needs a fresh one.
- [unit] Swift `aValueChangeAsksForAPreviewAndStartsOnCatalogOnly`, `catalogAndTasksSendsTheTasksTheChosenScopeShows`, `movingTasksOntoAValueTasksUseNeedsTheMergeAgreement`, `movedTasksCloseTheUndoWindowAndACatalogEditDoesNot`: nothing is moved without confirming, the choice starts on Catalog only each time, the tasks sent are the ones the preview lists for the Include subprojects choice, and the undo window closes after tasks move.
- [unit] Backend `TestApplyMovesThePreviewedTasksInOneChange`, `TestApplyChangesNothingIfAWriteFails`, `TestApplyRefusesAStalePreview`, `TestUndoRefusesAProjectMigration`: the server moves all tasks or none, and the migration has no undo.
UX:
- Flow: **Edit…** -> change the name, parent or value so the full value differs -> **Review Rename…** -> the second step of the same sheet, which says what “work” becomes -> choose **Catalog only** or **Catalog + pending tasks**, tick **Include subprojects** if its entries and tasks should follow -> **Save**, or **Rename and Move N Tasks** -> the catalog is saved, then the tasks move, and a toast says "Moved N tasks to “job”". **Back** returns to the form; Escape cancels and nothing changes.
- Surface: the same sheet (one short task), second step, because a sheet never opens another.
- States: counts read "One catalog entry changes, and 2 below it move with it." and "10 pending tasks move to “job”.", or "No pending tasks have this project, so only the catalog changes."; when tasks already use the new value and the catalog lacks it, "Tasks already use “job”. The tasks that move will join them." and "I understand that these tasks merge" must be ticked first; a value the catalog already has is refused in the form (S-049). If the tasks changed since the preview, nothing is written and the sheet goes back to the form with "Your tasks changed since the preview. Preview the rename again."; if the server refuses after the catalog is saved, the sheet says "The catalog is saved, but no task moved." with the reason, and that the old value now shows under From tasks, to rename again to move them; **Done** closes it.
- Words: title "Rename Project", "“work” becomes “job”", choices "Catalog only" and "Catalog + pending tasks", "Include subprojects", "Tasks you completed or deleted keep their old project. Undo can’t reverse a rename that moves tasks.", buttons "Back", "Cancel", "Save", "Rename and Move 10 Tasks", "Done".
- Access: the choices are a radio group and switches with their words; the merge agreement is a labeled switch and the primary button stays disabled until it is ticked; messages are text with symbols.
Status: done

### S-052 Add a project my tasks already use
As the user, I see a project that only exists on tasks and add it to the catalog.
Screens: settings-projects.png, settings-project-add-from-tasks.png
Depends: S-049
Acceptance:
- [snap] `settings-projects`: a "From tasks" row offers **Add to Catalog…**; `settings-project-add-from-tasks`: it opens the add sheet filled in, with the parent, the value and a name made from it.
- [unit] Swift `addToCatalogFillsTheSheetAndAddsNothingUntilItIsSaved`, `valuesSplitAtTheirLastDot`: nothing is added until that sheet is saved.
Status: done

### S-053 Check that everything is working
As the user, I open Diagnostics and copy a report for a bug report.
Screens: settings-diagnostics.png, settings-diagnostics-problems.png, settings-diagnostics-config-error.png
Depends: S-041
Acceptance:
- [snap] `settings-diagnostics`: the tab lists the checks for the server, the sign-in, the time zone, the config and the calendar, each a symbol and a word, with the app and bridge versions and the config path; `settings-diagnostics-problems` shows a failed and a skipped check.
- [manual] **Show in Finder** reveals the config file; **Copy Report** puts the report on the clipboard; not tried by hand yet.
- [unit] Go `TestDoctorChecksTheServerTheSignInTheZoneTheConfigAndTheCalendar`, `TestDoctorAnswersBeforeAServerIsNamed`, `TestDoctorCanLeaveOutTheTerminalChecks`: the `doctor` operation against the fake server, without the terminal app's Terminal and State directory checks, which belong to it.
- [unit] Go `TestDoctorLeavesSecretsAndEventTitlesOutOfTheReport`; Swift `theReportHoldsNoTokenAndNoEventTitle`, `theReportListsTheVersionsThePathAndEachCheck`: the copied report holds no token and no event title.
- [snap] `settings-diagnostics-config-error`, Go `TestDoctorFailsTheConfigCheckWithTheFilesOwnMessage`: a config error from S-054 shows in this tab too, as the same banner every Settings tab has, and as a failed config check with the file's own message.
- [unit] Swift `macOSCalendarIsOneMoreCheckOnlyWhenItIsTheSource`, `aRunThatFailedSaysWhy`, `theAppVersionReadsTheInfoPlistAndTheBridgesAnswerDecodes`.
UX:
- Flow: Settings ▸ Diagnostics -> the checks run when the tab opens -> read each one -> **Copy Report** to paste into a bug report, **Show in Finder** to reach `config.toml`, **Check Again** after fixing something.
- Surface: inline in the Diagnostics tab (information, no decision), with a banner above it when the config has a problem (S-054).
- States: loading is a spinner; each check is a symbol and a word, "OK", "Skipped" or "Problem", with its name and a detail line that can be selected; the footer says "Everything is working." or "Something needs attention."; when the checks cannot run, "Unable to run the checks" with the reason and **Check Again**; macOS Calendar adds a check when it is this Mac's source ("N calendars; read-only", or "no access; allow it in System Settings ▸ Privacy & Security ▸ Calendars"); before a server is named the server check fails with how to add one; after **Copy Report** a "Copied" mark shows for two seconds.
- Words: group "Checks", the check names "Backend server", "Sign-in", "Time zone", "Momentum config", "Calendar source" and "macOS Calendar", group "About" with "App", "Bridge" and "config.toml", buttons "Show in Finder", "Copy Report" and "Check Again"; hover text on Copy Report "Copy these checks and versions to paste into a bug report. They hold no sign-in code or event title."
- Access: each check reads as its name, word and detail; the result is a word and a symbol, never color alone; the buttons are reachable with Tab.
Status: done

### S-054 Edit `config.toml` in an editor and see it applied
As the user, I change `config.toml` in my editor, or the TUI, and the app picks it up without a restart.
Screens: settings-config-error.png, settings-restart.png
Acceptance:
- [unit] Go `TestAnEditToConfigAppliesOnTheNextLoad`, `TestStatusReadsTheFileBeforeSettingsDoes`, `TestOnlyAChangedFileIsReloaded`: the bridge reloads when the file's modification time or size changes, on each `load`, `plan` and `status`, so an edit shows in the next load and when Settings opens.
- [unit] Go `TestAFileThatNoLongerValidatesKeepsTheLastGoodSettings`, `TestTheConfigNoticeReachesTheAppInStatus`; Swift `aRefreshReportsAFileThatWasNotApplied`, `openingSettingsReadsTheFileAgain`: a file that no longer validates keeps the last good settings, and the error reaches the app. Settings shows it (`settings-config-error`); Diagnostics shows it too (S-053).
- [unit] Go `TestAChangedBackendURLWaitsForARestart`: a changed `[backend] url` is ignored until the app restarts, and Settings says so (`settings-restart`).
- [unit] Swift `theUndoDelayAndRefreshIntervalFollowTheFile`: the undo delay and the refresh interval follow the file after the next refresh.
UX:
- Flow: edit `config.toml` in an editor or the terminal app -> the app notices on its next load (every `refresh_interval`, ⌥⌘R, `r`) or when Settings opens, and applies the new values without restarting: Plan Today and its sidebar box appear or go, the capacity and calendar time in the meter change -> a file that no longer parses or validates changes nothing and Settings says so -> fix the file and the next load clears the message. A changed `[backend] url` still needs a restart, and Settings says so.
- Surface: inline banner at the top of every Settings tab, above its content (a status people can act on in their own time, never an alert).
- States: file fine (no banner); file invalid ("Unable to read config.toml", "The app is still using the last settings that worked." and the file's own message beneath, a warning symbol, **Show config.toml in Finder**); `[backend] url` changed ("`[backend] url` changed", "Quit and reopen Momentum Task Manager to use it."). Both can show at once, the invalid file first. Nothing changes in the main window.
- Words: "Unable to read config.toml", "The app is still using the last settings that worked.", "Show config.toml in Finder", "`[backend] url` changed", "Quit and reopen Momentum Task Manager to use it."
- Access: the banner is a symbol, a bold first line and a detail line, so meaning never rests on color; the message can be selected and copied; the button is reachable with Tab.
Status: done

### S-055 Never overwrite a config changed elsewhere
As the user, if the file changed while Settings was open, my save is refused and nothing is lost.
Screens: settings-conflict.png
Depends: S-054
Acceptance:
- [unit] A save is refused when the file changed since it was read; comments and unrelated keys stay; a symlinked config stays a symlink. Go `TestAFileThatChangedSinceItWasReadIsNeverOverwritten`, `TestSavePlanningChangesOnlyItsKeys` (comments and other keys stay), `TestSavePlanningKeepsASymlinkASymlink`, `TestASaveOnAFileThatChangedElsewhereIsRefused`, `TestAProjectSaveOnAFileThatChangedElsewhereIsRefused`; Swift `aFileThatChangedElsewhereReloadsTheTabAndSaysSo`, `aFileThatChangedElsewhereClosesTheSheetAndSaysSo` (Projects).
- [snap] `settings-conflict`: after a refusal the tab reloads from disk, says the file changed outside Momentum, and asks me to make the change again: the Planning tab shows the notice, and the daily capacity field shows what the file says, not what was typed.
Status: done

## M7 Release

### S-056 Open a signed build
As the user, I install a build that macOS opens without a warning about an unknown developer.
Screens: —
Acceptance:
- [manual] `spctl --assess` accepts the app and the bundled `momentum-bridge`.
- [manual] The build is hardened and signed with a Developer ID, not ad hoc; the Keychain prompt on rebuild no longer appears.
Deferred (2026-10-29): signing for distribution needs a Developer ID Application certificate, which needs a paid Apple Developer account, and there is none yet. Until then the app is built ad hoc, as before. When there is one: turning on the hardened runtime needs the `com.apple.security.personal-information.calendars` entitlement, or S-047's calendar access stops working in that build, and notarizing sends the app to Apple, which needs approval first.
Status: deferred

## M8 Google Calendar

Count the meetings in my Google Calendar against each day's capacity, on every machine I use. Plan: [`docs/plans/google-calendar.md`](./plans/google-calendar.md). Story to task: S-057 G02 to G09 for connecting, and G11; S-058 G05, G08 and G09; S-059 G05; S-060 G09; S-061 G05 and G11; S-062 G05 and G11; S-063 G10's `doctor` check; S-064 G10. Task G01 (the ADR) goes with S-057 and task G12 (docs) with the last story.

The plan's decisions D1–D9 were approved on 2026-10-29, each as recommended. Stories S-057 to S-064 are done by their tests and snapshot pages; what is left is the Cloud steps of plan C1, which are the owner's, and each story's manual check against a real Google account. Each story's `Decisions:` line names the ones it rests on. If a decision changes, only the stories that name it change.

M8 steps outside the Mac app in one place: S-064 is the terminal app's side (plan G10), which D6 asks for so that both apps on a machine agree. It stays in M8 (decided 2026-10-29).

Source rule (plan D9): D6 puts the source in `config.toml`, but S-047 keeps the Source in this Mac's preferences and the glossary says "not in `config.toml`". When the Mac holds macOS Calendar and `config.toml` says `google`, macOS Calendar counts, as S-047 already has the bridge count the Mac's events whatever `config.toml` says; choosing Google or Calendar files in the picker clears that preference and writes `source`. S-057 builds that rule and rewrites the glossary's Source row.

### S-057 Connect my Google Calendar
As the user, I choose Google Calendar as the Source, press **Connect…**, approve in the browser, and the tab says which Google account is connected.
Screens: settings-calendar-google-connect.png, settings-calendar-google-waiting.png, settings-calendar-google.png, settings-calendar-google-not-offered.png, settings-calendar-google-signed-out.png, settings-calendar-google-error.png
Depends: S-047
Decisions: D1, D2, D3, D5, D6, D8, D9
Acceptance:
- [snap] `settings-calendar-google-connect`: Source ▸ Google Calendar with no connection says what Momentum may read and offers **Connect…**; `settings-calendar-google-waiting`: after **Connect…** the tab says it is waiting for the browser and offers **Cancel**; `settings-calendar-google`: "Connected as <email>"; `settings-calendar-google-signed-out`: signed out, the tab says to sign in first; `settings-calendar-google-error`: an expired link shows its message and **Connect…** again.
- [snap] `settings-calendar-google-not-offered`: a server with no Google client or no `CALENDAR_TOKEN_KEY` answers `offered: false`, and the tab says Google Calendar isn't available on this server instead of offering **Connect…**.
- [unit] Go, fake Google on `httptest`: the one-time link expires after ten minutes and works once; the authorization request asks for `calendar.events.readonly`, `openid` and `email`, with offline access, consent and PKCE; the callback without the state cookie, or with a used state, is refused, so no one can attach a calendar to someone else's account.
- [unit] Go: the refresh token is stored sealed with AES-256-GCM, and a changed ciphertext or another key fails to open it; a key that is not 32 bytes of base64 is refused and no key turns the feature off; one connection per user, and another user's row is invisible.
- [unit] Go, captured logger: no link, state, code or token appears in any log line of the flow.
- [unit] Go: `[calendar] source` is `files` (the default) or `google`; `google` needs no `paths` and counts whatever `enabled` says, as macOS Calendar does (S-047), with the working hours checked either way; an unknown value is refused; `files` keeps today's rules.
- [unit] Swift `choosingGoogleWritesTheSourceAndShowsItInThePicker`, `choosingGoogleOrFilesClearsAMacOSCalendarChoiceThatWouldCountOverThem`, `aMacOSCalendarChoiceCountsOverWhatConfigSays`, `connectingOpensTheLinkAndWaitsUntilTheAccountIsConnected`, `cancelStopsWaitingAndConnectsNothing`, `aLinkThatExpiresEndsTheWait`, and for the wire `theBridgesAnswersDecode`, `aSourceChangeSendsOnlyTheRevisionAndTheSource`: choosing Google writes `[calendar] source = "google"` and clears a macOS Calendar choice (D9); **Connect…** opens the link and stops waiting once the status says connected, and the app loads again; **Cancel** stops waiting without connecting. Go `TestSavingTheSourceMakesPlanTodayReadGoogleAtOnce`, `TestSettingsShowTheCalendarSource` in the bridge.
- [unit] Swift `theConnectionBelongsToTheAccountSoAnotherMacSeesItWithoutTheBrowser`, Go `TestEachUserSeesOnlyTheirOwnCalendarConnection`, `TestSigningOutAndInAgainKeepsTheCalendarConnection`: the connection belongs to the account, so a Mac that signs in later and chooses Google sees "Connected as <email>" without the browser, and signing a device out, then in again on another, keeps it and never asks Google to revoke it.
- [manual] With the Cloud steps of plan C1 done, connect once with a real Google account (D8: expect the "unverified app" warning). Tests never reach Google, so this is the only check against the real thing. Not tried yet: it needs those Cloud steps, which are the owner's, and a real account.
UX:
- Flow: Settings ▸ Calendar -> **Source** ▸ Google Calendar -> the tab checks the connection -> not connected: it says what the server reads and offers **Connect…** -> **Connect…** opens the browser on a one-time link (a Momentum page names the account, then Google asks) -> the tab says "Waiting for your browser" -> approve in Google -> the tab says "Connected as <email>" and the app loads again, so the sidebar's meter and the next Plan Today count the meetings. Way back: **Cancel** while waiting; **Source** ▸ Calendar files or macOS Calendar at any point (the connection stays); S-062 takes the connection away.
- Surface: inline (a picker and a button change values on something already on screen), in the Calendar tab, like S-047. Not a sheet, since waiting for the browser blocks nothing. The browser page and Google's consent screen are the browser's and Google's own.
- States: checking ("Checking Google Calendar…" with a spinner, shown only when the answer takes over half a second); not signed in (lock symbol, "Sign in to connect Google Calendar", "Google Calendar belongs to your Momentum account. Choose Momentum Task Manager ▸ Sign In… first."); not offered (symbol, "Google Calendar isn’t available on this server", "Ask whoever runs the server to turn it on."); not connected (the description and **Connect…**); waiting (spinner, "Waiting for your browser", "Approve Momentum in the browser. This tab updates when you’re done. The link works for 10 minutes.", **Open Browser** and **Cancel**); connected (checkmark, "Connected as ada@example.com", the footer below); failed check ("Unable to check Google Calendar: <reason>" and **Try Again**); expired link ("The link expired. Choose Connect… to try again."). A refused or cancelled approval shows on the browser page ("Google Calendar wasn’t connected. Close this tab and choose Connect… again in Momentum Task Manager."), and the tab keeps waiting until **Cancel** or the link expires. Choosing Google Calendar is allowed whatever the server offers, as choosing macOS Calendar is allowed when the Mac says no. The calendar files groups hide while the Source is Google Calendar and come back as they were; Working hours and Events that count apply to all three sources.
- Words: picker "Source" with "Calendar files", "macOS Calendar" and "Google Calendar"; footer under the picker "Calendar files and Google Calendar also apply to the terminal app on this Mac. macOS Calendar applies to this app only."; group "Google Calendar"; description "The server reads today’s meetings in your primary Google calendar: their times, titles and whether you declined them. It never changes your calendar and never keeps the events."; footer when connected "The connection belongs to your Momentum account, not to this Mac. Choose Google Calendar on each Mac that should use it."; buttons "Connect…", "Open Browser", "Cancel", "Try Again".
- Access: the picker and the buttons are labeled with their text and reachable with Tab; the spinner is labeled "Waiting for your browser"; when the connection arrives VoiceOver announces "Connected as <email>"; every state is a symbol and words, never color alone.
- Built: the six pages were read in light and dark and match this block, each state a symbol and words. A connection Google has refused is worded by S-061 as Needs reconnecting.
- Review: `[calendar] enabled` and `paths` now mean calendar files only. `source = "google"` counts Google whatever `enabled` says, as S-047 does for macOS Calendar, so choosing Google writes `source` and nothing else; the plan is silent on this, so it is recorded here, and the TUI's `doctor` and docs follow it. Plan Today's "Calendar unavailable" line gives the reason but no way to Settings; S-061 may add one. The glossary's Source row changes with this story, and Connect is a new term that never means Sign In.
Status: done

### S-058 Count my Google meetings in Plan Today
As the user with Google connected, I open Plan Today and see my meetings taking time off the day.
Screens: plan-today.png, today.png
Depends: S-057, S-037
Decisions: D2, D3
Acceptance:
- [unit] Go: the same meetings give the same busy minutes and blocks from Google as from an `.ics` fixture; working hours, **Count all-day events** and **Count events shown as free** apply to both; an all-day event arrives as all-day and a "free" one as transparent. Go (bridge) `TestGoogleGivesTheSameBusyTimeAndBlocksAsTheSameEventsInAFile`, `TestHoursAndEventChoicesApplyToGoogleEventsToo`, (server) `TestEventsComeFromThePrimaryCalendarForTheDay`.
- [unit] Go, fake Google: a repeating meeting with one cancelled and one moved occurrence gives the right times, which `.ics` files can't; only the primary calendar is read, between the start and end of the device's day. `TestARepeatingMeetingWithACancelledAndAMovedOccurrence`, `TestEventsComeFromThePrimaryCalendarForTheDay`, `TestTheServerIsAskedForTodayInTheDevicesOwnOffset`.
- [unit] Go: a Google answer is never stale, so **Stale after** doesn't touch it; titles come back only with `plan`, never with `load`. `TestGoogleGivesTheSameBusyTimeAndBlocksAsTheSameEventsInAFile` (not stale), `TestALoadNeverCarriesATitleAndPlanTodayDoes`.
- [unit] Go, captured logger: the server stores and logs no event and no title. `TestLogsHoldNoSecretsAndNoTitles`, `TestConnectingGoogleCalendarOverHTTP`.
- [manual] The sheet shows the merged blocks with their titles on hover and the sidebar's meter counts them, as with files; run once with a real calendar. Not tried yet: it needs a real connection, which is S-057's manual check.
Status: done

### S-059 Leave out meetings I declined
As the user, a meeting I declined doesn't take time off my day, and one I accepted, tentatively accepted or haven't answered does.
Screens: plan-today.png
Depends: S-058
Decisions: D2, D4
Acceptance:
- [unit] Go, fake Google: a declined event is not among the events the server returns, so it is neither in the strip nor in the busy minutes; accepted, tentative and unanswered events are. `TestDeclinedMeetingsAreLeftOut`.
- [manual] Decline a meeting in Google and open Plan Today: its time comes back to the day. Files can't say declined, so the same meeting can differ between the two sources. Not tried yet, for the same reason as S-058's.
Status: done

### S-060 See a new meeting without waiting
As the user, I add a meeting in Google and open Plan Today, and it is already counted; the sidebar catches up within five minutes.
Screens: plan-today.png, today.png
Depends: S-058
Decisions: D7
Acceptance:
- [unit] Go, bridge with a fake server: `plan` always asks the server for events; a `load` within five minutes of the last fetch reuses its busy time and asks again after five; a reused answer carries no titles. `TestOpeningPlanTodayAlwaysAsksAndALoadReusesTheAnswerForFiveMinutes`, `TestACachedAnswerCarriesNoTitles`.
- [unit] Go: a load keeps the day's events, not the busy minutes, so a change to the working hours or the event choices applies to the next load at once with no new fetch, which keeps S-046's "applies at once"; a failure is never kept, and another account's events are never reused. `TestChangingTheHoursOrEventChoicesNeedsNoNewFetch`, `TestAnUnavailableAnswerIsNotKept`, `TestSigningInAsSomeoneElseForgetsTheCachedAnswer`.
- [manual] Add a meeting in Google, open Plan Today and see it counted; close the sheet and see the sidebar's meter follow within five minutes. Not tried yet, for the same reason as S-058's.
Status: done

### S-061 Reconnect when Google stops answering
As the user, when Google refuses my saved connection, I'm told to reconnect and can do it from the same tab, and planning goes on with tasks alone.
Screens: settings-calendar-google-reconnect.png, plan-today-calendar-google-reconnect.png
Depends: S-057, S-038
Decisions: D8
Acceptance:
- [unit] Go, fake Google: `invalid_grant` marks the connection "needs reconnecting", the events route answers `calendar_unavailable` with the reason, and the mark stays until the person reconnects or disconnects. In Testing a token lasts seven days (D8), so this is the weekly case until the client is in production. `TestGoogleRefusingTheConnectionMarksItNeedsReconnecting`, `TestReconnectingReplacesTheConnectionInPlace`, `TestAKeyThatNoLongerOpensTheTokenMeansReconnecting`.
- [snap] `settings-calendar-google-reconnect`: "Needs reconnecting" with **Reconnect…**; `plan-today-calendar-google-reconnect`: Plan Today says "Calendar unavailable: <reason>". Both read in light and dark.
- [unit] Go, bridge: no connection, a dead token and Google unreachable each give zero calendar minutes and the reason, and ticking, saving and the sidebar's meter keep working (S-038). `TestAGoogleCalendarThatCannotBeReadMakesTheCalendarUnavailable`, and in the server `TestEventsNeedAConnectionAndAnOfferingServer`, `TestAGoogleFailureLeavesTheConnectionAsItWas`; ticking and saving never read the calendar, as S-038's `TestAMissingCalendarFileMakesTheCalendarUnavailable` shows.
- [unit] Swift, fake bridge: **Reconnect…** follows S-057's connect flow; once connected the app loads again and the message goes. `aConnectionGoogleRefusedNeedsReconnecting`, `reconnectingFollowsTheConnectFlowAndMayUseAnotherAccount`, `cancelWhileReconnectingGoesBackToNeedsReconnecting`, `anExpiredReconnectLinkAsksToReconnectNotToConnect`.
- [manual] Wait for Google to refuse a saved connection (seven days in Testing), or revoke Momentum at myaccount.google.com, then open Plan Today and Settings ▸ Calendar. Not tried yet: it needs a real connection.
UX:
- Flow: Plan Today says "Calendar unavailable: Google Calendar needs reconnecting", or the sidebar's meter drops the meetings -> Settings ▸ Calendar (⌘,) -> Source ▸ Google Calendar shows "Needs reconnecting" -> **Reconnect…** opens the browser on a new one-time link (S-057's flow) -> "Waiting for your browser" -> approve -> "Connected as <email>" and the app loads again, so the meter and the next Plan Today count the meetings and the message goes. Way back: **Cancel** while waiting returns to "Needs reconnecting"; another **Source** at any time (the saved connection stays until it is reconnected or disconnected).
- Surface: inline, in the section S-057 built (a button changes the state of something already on screen). No alert and no toast, since nothing needs an answer at once and planning goes on with tasks. Plan Today gets no new button: it is a sheet, its line already says what to do, and Settings is ⌘, away. Every Source states its reason there without a link, so a way to Settings would be one change for all three and goes in Later.
- States: needs reconnecting (warning symbol, "Needs reconnecting", "Google no longer accepts the saved connection to <email>. Plan Today counts no meetings until you reconnect.", **Reconnect…**); then S-057's waiting, connected, expired and failed states, where **Cancel** and an expired link return to this state with **Reconnect…**, not **Connect…**. Reconnecting may use another Google account, and the tab then says "Connected as" that one. Plan Today shows its existing red line, "Calendar unavailable: Google Calendar needs reconnecting", and under it "Calendar time counts as zero; planning goes on with tasks."; the sidebar's meter counts no meetings.
- Words: "Needs reconnecting"; the detail above; **Reconnect…** (an ellipsis, since it opens the browser; "reconnect" because a connection exists and is being replaced); the expired line "The link expired. Choose Reconnect… to try again."
- Access: the state is a symbol and words, and its title and detail read as one element; **Reconnect…** is reachable with Tab; when the connection comes back VoiceOver announces "Connected as <email>", as in S-057.
- Review: Plan Today tells a person what is wrong but not where to fix it; the way is Settings ▸ Calendar. **Disconnect…** (S-062) will sit beside **Reconnect…** for someone who would rather remove the connection.
- Built: both pages were read in light and dark and match this block. The harness caught the light Plan Today page mid-transition once, blank; taking it again alone gave the right image, and the existing "unavailable" page was half-faded in the same run, so it is the harness's timing, not the page.
Status: done

### S-062 Disconnect Google Calendar
As the user, I disconnect Google, after being asked, and Momentum forgets it.
Screens: settings-calendar-google-disconnect.png, settings-calendar-google-not-revoked.png
Depends: S-057
Decisions: D1, D5
Acceptance:
- [snap] `settings-calendar-google-disconnect`: **Disconnect…** asks first, names the account and says Momentum will stop reading the calendar and delete its saved access; `settings-calendar-google-not-revoked`: after a disconnect Google could not revoke, the not-connected tab says the access may still show in the Google account's settings. All read in light and dark, with the same pages for the states that gained **Disconnect…**, `settings-calendar-google` and `settings-calendar-google-reconnect`.
- [unit] Go, fake Google: disconnecting calls Google's revoke and deletes the stored token and the connection row. `TestDisconnectingRevokesTheTokenAtGoogleAndDeletesTheConnection`, and over HTTP the last steps of `TestConnectingGoogleCalendarOverHTTP`.
- [unit] Swift, fake bridge: the source stays Google, the tab offers **Connect…** again, and Plan Today says "Calendar unavailable: <reason>" until I connect or choose another Source; the connection is the account's, so every Mac sees it gone. `disconnectingEndsTheConnectionLoadsAgainAndLeavesTheSourceAlone`; Go, bridge `TestDisconnectingForgetsTheKeptEventsSoTheNextLoadSaysSo` (without it the meetings stay counted for five minutes), server `TestEventsNeedAConnectionAndAnOfferingServer` for the reason, `TestUsersOnlySeeAndChangeTheirOwnConnection` and `TestSigningOutAndInAgainKeepsTheCalendarConnection` for whose connection it is.
- [unit] Go, fake Google: with Google unreachable the revoke fails, the connection is still deleted, and the answer says the token was not revoked at Google; Swift, fake bridge: the tab shows that. `TestDisconnectingDeletesTheConnectionEvenWhenGoogleCannotRevoke`, `TestATokenGoogleAlreadyCallsInvalidCountsAsRevoked`, `TestDisconnectingWithAKeyThatNoLongerOpensTheToken`; Swift `aDisconnectGoogleCouldNotRevokeSaysSoUntilTheNextConnect`.
- [unit] Swift, fake bridge: a disconnect that fails keeps the connection and says why, and **Disconnect…** is the way to try again; **Cancel** in the prompt changes nothing and asks the server nothing. `aDisconnectThatFailsChangesNothingAndTheButtonTriesAgain`, `cancellingThePromptAsksTheServerNothing`, `aConnectionThatNeedsReconnectingCanBeDisconnected`, `thereIsNothingToDisconnectBeforeAConnection`, `theBridgesDisconnectAnswerDecodes`.
- [manual] Disconnect once for real from a connected account, and look in your Google account's connections: Momentum should be gone. Not tried yet: it needs a real connection.
UX:
- Flow: Settings ▸ Calendar (⌘,) -> Source ▸ Google Calendar, connected or needing to reconnect -> **Disconnect…** -> the prompt names the account -> **Disconnect** -> the tab shows S-057's not-connected state with **Connect…**, the app loads again, and Plan Today says "Calendar unavailable: Google Calendar isn’t connected" until a person connects or chooses another Source. **Cancel**, or Escape, changes nothing. Way back: **Connect…** makes a new connection, with the same Google account or another.
- Surface: confirmation dialog, which the app already uses to ask before a removal (S-050): it ends a connection that belongs to the account, so it ends it for every Mac and the terminal app, and a person is asked first. The button sits inline in the section S-057 built, since it acts on what the section shows. The dialog is the system's; no sheet and no toast, since the tab changing is the feedback.
- States: **Disconnect…** shows with "Connected as <email>", and beside **Reconnect…** under "Needs reconnecting"; while the server is asked, both buttons are off. After it: the not-connected state of S-057. If Google could not be asked to revoke the access, the connection is still deleted, and above the description the tab says, with a warning symbol, "Couldn’t reach Google to revoke its access, so it may still show in your Google account’s settings. Remove it there." The note stays until the person connects again, and lives only as long as the app runs, since it is about something that already happened. If the server cannot be asked at all ("Unable to disconnect Google Calendar: <reason>"), an error line shows above "Connected as <email>", nothing has changed, and **Disconnect…** is the retry. Another Mac, or the terminal app, finds the connection gone the next time it asks.
- Words: button **Disconnect…** (an ellipsis, since it asks first); prompt title "Disconnect Google Calendar?"; message "Momentum will stop reading <email> and delete its saved access, for every Mac and the terminal app on your account. Plan Today counts no meetings until you connect again."; buttons **Disconnect** and **Cancel**; the notes above.
- Access: **Disconnect…** is reachable with Tab and the prompt is the system's, so Escape cancels and VoiceOver reads its title and message; the notes are a symbol and words, and their title and detail read as one element.
- Review: the connection belongs to the account, so disconnecting on one Mac removes it for all, and a terminal app whose `config.toml` says `google` then has nothing to read, which S-063's `momentum doctor` check reports. The word is Disconnect, never Sign Out, which is the Momentum sign-in. The failed-disconnect line is the same error line as S-057's expired link and has no page of its own; it is tested.
- Built: the four pages were read in light and dark and match this block. The prompt is the system's, anchored on the Calendar tab's form, as the Projects tab anchors its own. Writing the tests showed that the bridge's five-minute cache would have kept counting the meetings after a disconnect; `TestDisconnectingForgetsTheKeptEventsSoTheNextLoadSaysSo` guards it, and removing the cache clear made it fail.
Status: done

### S-063 Check Google in Diagnostics
As the user, I open Settings ▸ Diagnostics and see whether Google Calendar works.
Screens: settings-diagnostics-google.png
Depends: S-053, S-057
Decisions: D1, D6
Acceptance:
- [unit] Go: `momentum doctor` with `source = "google"` fails its calendar check, with the reason, when there is no connection or it needs reconnecting, and passes when connected; with `files` it checks the files as before. `TestGoogleCalendarPassesWhenItIsConnected`, `TestGoogleCalendarFailsWithItsReasonAndWhereToFixItOnTheMac`, `TestGoogleCalendarIsCheckedWhateverEnabledSays`, `TestTheFilesSourceIsCheckedAsBeforeAndNeverAsksGoogle`, and for the Mac's own doctor `TestTheMacsDoctorChecksGoogleCalendarAndNamesWhereToFixIt`, `TestTheMacsDoctorStillChecksTheFilesWhenTheyAreTheSource`.
- [snap] `settings-diagnostics-google`: the calendar check shows as a Problem with its symbol, its word and the reason. Read in light and dark.
- [unit] Go: the Report holds no link, token or event title. `TestTheReportHoldsNoLinkTokenOrEventTitle`, `TestAGoogleCheckThatCannotBeAnsweredIsAProblemWithoutSecrets`, `TestTheMacsDoctorLeavesSecretsOutOfAGoogleCheckThatCouldNotAnswer`.
- [unit] Go: with the server down, or no sign-in, the check is Skipped, never a Problem; a Google check passes only when the server says connected. `TestGoogleCalendarIsSkippedWithoutAServerOrASignIn`, `TestAServerClientThatCannotSayWhereGoogleStandsIsSkipped`.
- [manual] With a real connection, run `momentum doctor` in a terminal and open Settings ▸ Diagnostics: both show the check. Not tried yet: it needs a real connection.
UX:
- Flow: Settings ▸ Calendar's problem is read in Settings ▸ Diagnostics (the checks run when the tab opens) -> read "Calendar source" -> fix it where the detail says -> **Check Again**. In the terminal, `momentum doctor` prints the same check.
- Surface: inline, one more state of a row the Diagnostics list already has (information, no decision); no new screen, dialog or toast.
- States: with `[calendar] source = "files"` the check is as S-053 left it. With `source = "google"`, which counts whatever `[calendar] enabled` says (ADR 0003, D9): OK "Google Calendar connected as joao@gmail.com; read-only"; Problem "Google Calendar isn’t connected. Connect it in Settings ▸ Calendar.", "Google Calendar needs reconnecting. Reconnect it in Settings ▸ Calendar.", "Google Calendar isn’t available on this server. Ask whoever runs the server to turn it on.", or "Unable to check Google Calendar: <reason>" when the server is up but cannot answer; Skipped "needs the server" or "needs a sign-in" when the Backend server or Sign-in check did not pass, as the Sign-in check is Skipped with "needs the server".
- Words: the check stays "Calendar source", since it is about the Source; "read-only" echoes the files check; the reasons are the ones Plan Today, Settings and the server already use, and where the fix is in Settings ▸ Calendar the Mac's detail says so. `momentum doctor` gives the same reason without the Settings sentence, since the terminal app has no such tab; S-064 adds `momentum calendar connect` and the sentence that names it.
- Access: unchanged from S-053: each check reads as its name, word and detail, a word and a symbol, never color alone.
- Review: the check describes what `config.toml` names, which the terminal app follows too. When this Mac counts macOS Calendar over a `google` config (D9), both rows show, as they do with files today, so a Problem here can describe a source this Mac isn't using; the "macOS Calendar" row says what this Mac uses. One snapshot page shows the Problem; OK and Skipped look as S-053's rows do, and every state is unit-tested.
- Built: before this story a `google` source fell into the files check and reported "calendar source unavailable", or "disabled" with `enabled` off, which said nothing true about Google; the new tests failed that way first. The check reaches the server through an optional interface, so the terminal app's core client and the bridge's client both answer it without widening the interface the existing fakes use.
Status: done

### S-064 Use Google Calendar in the terminal app
As the user in the terminal, I run `momentum calendar connect` and the planner counts my Google meetings.
Screens: —
Depends: S-057, S-058
Decisions: D1, D6
Acceptance:
- [unit] Go, fake Momentum server: `momentum calendar connect` prints the link and waits for the connection; `status` shows the Google account or "needs reconnecting"; `disconnect` asks first and then disconnects. `TestConnectPrintsTheLinkAndWaitsUntilTheAccountIsConnected`, `TestConnectShowsTheAddressWhenNoBrowserOpens`, `TestReconnectWaitsUntilGoogleAcceptsTheConnectionAgain`, `TestConnectChangesNothingWhenGoogleAlreadyWorks`, `TestConnectEndsWhenTheLinkExpires`, `TestConnectStopsWaitingWhenInterrupted`, `TestConnectNeedsAServerThatOffersGoogleCalendar`, `TestStatusSaysWhereGoogleCalendarStands`, `TestDisconnectAsksFirstAndThenDisconnects`, `TestDisconnectChangesNothingUnlessTheAnswerIsYes`, `TestDisconnectHasNothingToDoWithoutAConnection`, `TestDisconnectAConnectionThatNeedsReconnecting`, `TestDisconnectSaysWhenGoogleCouldNotRevokeTheAccess`, `TestDisconnectThatFailsSaysWhy`, `TestCalendarCommandsNeedABackendAndASignIn`, `TestCalendarNeedsOneOfItsThreeCommands`, `TestExtractCommandFindsTheCalendarCommandWhereverTheFlagsAre`.
- [unit] Go: with `source = "google"` the planner opens with Google's busy time and blocks, the same minutes the bridge gives; a failure shows "Calendar unavailable: <reason>" and planning goes on. `TestThePlannerReadsGoogleWhateverEnabledSays`, `TestGoogleGivesThePlannerTheSameBusyTimeAndMeetingsAsTheSameEventsInAFile`, `TestTheFilesSourceNeverAsksGoogle`, `TestThePlannerAsksForTodayInTheDevicesOwnZone`, `TestAGoogleCalendarThatCannotBeReadLeavesPlanningRunning`, `TestAServerThatCannotReadGoogleLeavesPlanningRunning`.
- [unit] Go: `momentum doctor` in the terminal names the command that fixes a Google problem ("Run momentum calendar connect to connect it."), which S-063 left to this story; the Mac's doctor still names Settings ▸ Calendar. `TestGoogleCalendarFailsWithItsReasonAndWhereToFixItOnTheMac`, whose terminal column now holds the sentence.
- [manual] Run once against a real server with the real client. Not tried yet: it needs a real connection, which is S-057's manual check.
UX:
- Flow: `momentum calendar connect` -> it asks the server for the one-time link, prints it and tries to open the browser -> "Waiting for you to finish in the browser (Ctrl+C cancels)…" -> approve in Google -> "Connected Google Calendar as <email>." and, when this machine's `config.toml` doesn't already say so, the line that turns it on -> the next planner (`P`) counts the meetings. `momentum calendar status` says where the account stands, and `momentum calendar disconnect` asks and then ends the connection. Way back: Ctrl+C while waiting; anything but `y` at the question.
- Surface: command-line output, as `momentum login` and `logout` give theirs. The planner is the existing sheet, with its meeting strip and its "Calendar unavailable: <reason>" line; no new screen, key or dialog.
- States: connect, when the server offers nothing: "Google Calendar isn’t available on this server. Ask whoever runs the server to turn it on."; when already connected and working: "Google Calendar is already connected as <email>." and how to change account (disconnect first); otherwise the link, the waiting line, then connected, "the link expired. Run momentum calendar connect to try again." or "stopped waiting; nothing was connected." status: "Google Calendar connected as <email>; read-only" and whether this machine counts it, "Google Calendar isn’t connected. Run momentum calendar connect to connect it.", "Google Calendar needs reconnecting. Google no longer accepts the saved connection to <email>. Run momentum calendar connect to reconnect it.", or the not-offered line. disconnect: nothing connected says so and asks nothing; otherwise the question names the account, then "Disconnected Google Calendar.", plus a warning when Google could not be asked to revoke; a failure says why and changes nothing. A server that is down, or no sign-in, fails as every other command does.
- Words: "connect", "reconnect" and "disconnect", never "login" or "sign in", which are Momentum's own (glossary); the reasons are the ones the Mac, the server and `doctor` already use. The question is "Disconnect Google Calendar?" with "Momentum will stop reading <email> and delete its saved access, for every Mac and the terminal app on your account. The planner counts no meetings until you connect again." and `[y/N]`.
- Access: plain text a screen reader reads in order, every state in words, no color. Exit status is 0 when the command did its job or there was nothing to do, 1 when it could not (including a declined question and an expired link), and 2 for a misuse of the command.
- Review: the connection belongs to the account and the source to the machine (D6), so `connect` ends by saying what to set in `config.toml` instead of writing it: the terminal app never edits `[calendar]`, and a duplicate `[calendar]` table would break the file, hence "set `source` under `[calendar]`" and no snippet. `connect` on a healthy connection changes nothing and says so; a different account takes `disconnect` first, as in the Mac, where **Connect…** is not offered while connected. The planner has no five-minute cache (D7): it opens, asks the server once, and that is the answer.
- Built: the commands and the planner follow this block, and `tui/README.md`, `tui/DESIGN.md` (criterion 18 and the Server section) and the glossary now describe them. The fake server's tests were written first. Five deliberate breakages were each caught by the test meant for them: the planner counting Google only with `enabled` on, the day's window ending a day late, a connection Google refuses ending the wait for a reconnect, a disconnect that skips the question, and a link that is never checked for expiry (which waits for ever, so its test times out). `momentum calendar` is a command of its own in `extractCommand`, as `calendar connect`, so that `--config` may come after it; Go's flag parsing would otherwise have stopped at `connect` and left `--config` among the arguments. The planner reaches Google through an optional `GoogleCalendarClient`, as the doctor does, so the existing fake clients did not change. Plan task G12 (the rest of the docs) is not done: the server's README and DEPLOY steps and the Mac's README are still to write.
Status: done

## M9 Task details

Keep what I need to know about a task with it: notes, what it waits on, and a copy to start a similar one. The server already stores notes and dependencies and the Mac shows both, but no app can change them, and the snapshot seed has notes that nobody can add. Chosen on 2026-10-29 from Later. It lifts the "Editing annotations, dependencies" non-goal of [`tui/DESIGN.md`](../tui/DESIGN.md) for the Mac app.

Plan: [`docs/plans/task-details.md`](./plans/task-details.md). Story to task: N01 to N06 (the server, the TUI's client, the bridge and the Mac's bridge and store) serve every story; then S-065 and S-066 N07, S-067 and S-068 N08, S-069 N09, S-070 N10. Task N11 (docs) goes with the last story.

The plan's decisions D1–D5 were approved on 2026-10-29, each as recommended; they replace the rules R1–R4 this milestone first proposed. In short: only the Mac app edits notes and dependencies (D1); a task may depend on a pending or completed task (D2); notes are added and removed, not edited (D3); a duplicate copies the description, project, priority, dates, estimate and tags, less the plan tag (D4); and only pending tasks offer Duplicate (D5). Each story's `Decisions:` line names the ones it rests on. If a decision changes, only the stories that name it change.

### S-065 Add a note to a task
As the user, I select a pending task and add a note, such as "Asked Ana for the quote", and it shows under Notes with the time I added it.
Screens: inspector.png, inspector-add-note.png, inspector-note-error.png, completed.png
Depends: S-010
Decisions: D1, D3
Acceptance:
- [unit] Go (server): a change that adds a note stores it after the task's notes, with the server's time; an empty note is refused on its field and changes nothing; a task that isn't pending refuses it; the change is one `task_events` row, so Undo removes the note.
- [unit] Go (bridge): the op reaches the server for that task and answers `{}`, as `complete` does; a refused note comes back on its own field (plan C10).
- [unit] Swift: adding sends the note for the selected task, loads again and opens the undo window with a toast naming the task; a refused note stays in its field with the server's reason.
- [snap] `inspector`: a pending task's note, then the empty field "Add a note".
- [snap] `inspector-add-note`: a note being written in the focused field, with **Add**.
- [snap] `inspector-note-error`: a note the server refused keeps its words, with the reason under the field.
- [snap] `completed`: a completed task's inspector has no field and says "No notes.", not "No annotations.", as the row's hover text already says "note".
- [manual] A note added on the Mac shows in the terminal app's details after its next refresh.
UX:
- Flow: select a pending task -> the inspector's Notes field, `a` in the list, or Task ▸ Add Note -> the field "Add a note" has the focus (the inspector opens if it was hidden) -> type -> Return or **Add** -> the note shows last with its time, the field empties and keeps the focus for another, and the toast says "Added a note to “…”" with Undo -> Escape takes the focus back to the list. Way back: Escape leaves the draft; Undo (⌘Z, `u` or the toast) removes the note.
- Surface: inline, a field at the end of the inspector's Notes section (the first row of the surface table: it changes one value of the task already on screen). Not a sheet: a note is one line, typed while the task stays in view.
- States: pending with notes: the notes, oldest first, each with its time, then the field. Pending without notes: the field alone, whose placeholder says what it is for. Completed: no field, and "No notes." when it has none, since Completed is read-only. Writing: **Add** works once the note has more than spaces. Saving: the field and **Add** wait. Refused: the field keeps the words and shows the reason under it with a warning symbol, such as "cannot reach the Momentum server: …". Long: a note wraps in its card; the field grows to four lines, then scrolls. Choosing another task starts a new draft.
- Words: section "Notes"; placeholder "Add a note"; button "Add"; Task menu "Add Note" (no ellipsis: it moves the focus to a field, as Finder's Rename does); Keyboard Shortcuts row `a` "Add note"; toast "Added a note to “<description>”"; empty "No notes.". Note is the glossary's word; annotation is not.
- Access: `a` from the list, Return adds, Escape returns to the list. The field reads "New note" and **Add** "Add note"; each note reads as its words, then its time; the warning is a symbol and words, never color alone.
Status: doing

### S-066 Remove a note from a task
As the user, I remove a note that no longer matters, and Undo brings it back.
Screens: inspector.png
Depends: S-065
Decisions: D3
Acceptance:
- [unit] Go (server): the change names the note by its time and words and removes that note only; when the task no longer has it, as when another device removed it first, the change is refused as a conflict; Undo puts it back in its place with its time.
- [unit] Swift: removing sends the note's time and words, loads again and opens the undo window; Undo restores the note.
- [snap] `inspector`: each note of a pending task has a remove button in its corner; a completed task's notes have none (`completed`).
UX:
- Flow: the remove button (×) in a note's corner, or **Remove Note** in the note's context menu -> the note goes at once, and the toast says "Removed a note from “…”" with Undo. Way back: Undo (⌘Z, `u` or the toast) puts the note back in its place with its time.
- Surface: an inline button on the note (the first row of the surface table). No confirmation: Undo takes it back, and a note is a line, not a task. The button stays visible, quietly, so the keyboard and a snapshot can reach it.
- States: completed task: no button. Another device removed it first: the toast gives the server's reason, "the task has no such note", and the next refresh shows the notes as they are.
- Words: the button is the `xmark` symbol with hover help "Remove this note"; context menu "Remove Note"; toast "Removed a note from “<description>”". Remove, not Delete, which the glossary keeps for tasks.
- Access: the button reads "Remove note" and is reached with Tab when Keyboard navigation is on; VoiceOver offers "Remove Note" as an action on the note.
Status: todo

### S-067 Make a task wait on another
As the user, I say that "Send the invoice" depends on "Get the quote", and the inspector lists it under Depends on.
Screens: inspector-depends.png, depends-picker.png
Depends: S-010
Decisions: D1, D2
Acceptance:
- [unit] Go (server): a change adds a dependency on another task of the same account; the task itself, an unknown or deleted task, another account's task and a loop are each refused on the field with a reason, and nothing changes; the change is one `task_events` row, so Undo removes it.
- [unit] Go (bridge): the op, as in S-065.
- [unit] Swift: the picker offers pending tasks by description, with search, and never the task itself or one that would close a loop; choosing one sends it, loads again and opens the undo window.
- [snap] `depends-picker`: the picker over the main window, narrowed by a search.
- [snap] `inspector-depends`: Depends on lists the task by its description, which selects it when clicked, as it does today. The snapshot seed's task that waits on another, which this story lets anyone make, shows it.
Status: todo

### S-068 Stop a task waiting on another
As the user, I remove a dependency that no longer holds.
Screens: inspector-depends.png
Depends: S-067
Decisions: D2
Acceptance:
- [unit] Go (server): removing a dependency the task doesn't have is refused as a conflict; Undo puts the dependency back.
- [unit] Swift: removing sends it, loads again and opens the undo window. An entry the inspector shows only by its UUID, because that task is no longer loaded, can be removed too.
- [snap] `inspector-depends`: each entry under Depends on offers Remove.
Status: todo

### S-069 See which tasks are blocked
As the user, I see in Today and Inbox which tasks wait on an unfinished task, and in the inspector which tasks wait on this one.
Screens: today.png, inbox.png, inspector-depends.png
Depends: S-067
Acceptance:
- [unit] Swift: a task is blocked while any task it depends on is pending; a dependency that is completed, deleted or not loaded doesn't block it.
- [snap] `today`: a blocked row shows a symbol, with hover text naming what it waits on; the symbol goes once that task is completed. The order is the server's, whose urgency already counts blocked and blocking tasks (`core/domain/urgency.go`).
- [snap] `inspector-depends`: a section lists the pending tasks that wait on this one, each selecting its task when clicked.
Status: todo

### S-070 Duplicate a task
As the user, I duplicate a task to start a similar one, and the copy is selected.
Screens: context-menu.png, menu.png
Depends: S-016
Decisions: D4, D5
Acceptance:
- [unit] Go (bridge): duplicating creates a task with the fields D4 names through the server's create route, and the answer is kept as a wire file; a repeating task's copy doesn't repeat, and a planned task's copy is not in today's plan.
- [unit] Swift: Duplicate selects the copy and opens the undo window with a toast naming the task; Undo removes the copy.
- [snap] `context-menu`: Duplicate follows Edit… on a pending task. Task ▸ Duplicate has ⌘D, the Mac's key for it; a bare `d` stays Delete…, as in the terminal app.
Status: todo

## Later

- Choose which Google calendars count, not only the primary one (plan D3): needs the `calendar.calendarlist.readonly` scope and a picker in each app.
- Fix the `.ics` parser's missing `EXDATE` and `RECURRENCE-ID`, so a cancelled or moved occurrence of a repeating event in a file counts right (plan, out of scope).
- Get the Google client verified, so connecting stops showing the "unverified app" warning (plan D8).
- A way from Plan Today's "Calendar unavailable" line to Settings ▸ Calendar, for every source (S-061 chose to leave it out, since files, macOS Calendar and Google all say their reason there without a link).
- Custom smart views and filters.
- Configurable keybindings.
- Changing a note's words (M9, plan D3).
- Notes and dependencies in the terminal app (M9, plan D1).
- Selecting several tasks and acting on them together.
- Notifications, and a helper that runs while the app is closed.

## Dropped

- Offline use and a local task store: the server is the source of truth ([ADR 0002](./adr/0002-go-postgres-backend.md), P1).
- Contexts and hooks ([ADR 0002](./adr/0002-go-postgres-backend.md), D6), and the design's "Context" row and Taskwarrior log panel.
- Taskwarrior inside the app: `momentum import` in the TUI copies the tasks over.
- A Kanban board.
- Writing to Google Calendar, and showing calendar events anywhere but Plan Today (plan, out of scope): Momentum reads a day's busy time and nothing else.
- Telemetry, crash reporting and automatic update checks.
- Windows, and the Homebrew tap (TUI only).
