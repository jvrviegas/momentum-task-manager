# Architecture review, 2026-10-30

Scope: the Mac app in `macos/` and the Go bridge it runs (`tui/internal/bridge`), at the end of M9 (task details, stories S-065 to S-070), with `main` clean at `10f6e27`. The survey read the code and the git history; the proof for each card is to be recorded before its refactor.

**Overall: the code is still in good shape.** M9 added about 1,100 lines along the seams that were there: the dependency rules are a plain type (`TaskDependencies`, `DependencyPicker`) tested without a store, the note draft (`NoteDraft`) takes the store as a parameter instead of holding it, and the five new bridge operations follow the existing ones. No card is `Strong`. The two signals that brought this review, `AppStore` back near 600 lines and the inspector being M9's hottest file, are real but not urgent.

Not assessable: `docs/FIXLOG.md` still does not exist, so the "same area in 3 fix-log entries" signal could not be checked.

Hot spots (commits touching the file in the last 30 days): `Model/AppStore.swift` 16, `Snapshot/SnapshotScenes.swift` 15, `MomentumTests/AppStoreTests.swift` 14, `Snapshot/SnapshotBridge.swift` 13, `Bridge/Wire.swift` 13, `Bridge/ProcessBridge.swift` 12; on the Go side `bridge/server.go` 13, `bridge/ops.go` 9, `bridge/wire.go` 9. Since the last review (`c131b3d`), M9's commits touched `Views/InspectorView.swift` most (5), then `SnapshotScenes.swift` (4). Largest files: `Views/PlanTodayView.swift` 644, `Model/AppStore.swift` 592, `Views/CaptureView.swift` 513, `Snapshot/SnapshotBridge.swift` 501, `Bridge/Wire.swift` 462, `Views/InspectorView.swift` 428.

---

## Task actions are spelled out on every surface  [Worth exploring]

Files: `Views/TaskKeyCommands.swift`, `App/MomentumCommands.swift`, `Views/TaskListView.swift` (`TaskMenu`), `Views/TaskRow.swift` (`RowActions`), `Views/InspectorView.swift` (the action bar), `Views/ContentView.swift`

Signal: one feature touched more than 6 files. S-065 (Add a note) changed 11.

Problem: an action that opens something is written out where each surface offers it. Opening the editor (`store.editing = EditRequest(task: …, focus: .description)`) is in 5 files, the delete prompt (`store.pendingDelete = …`) in 5, the stop-repeating prompt in 4. M9 added two more of the same kind: Add Note (`store.showInspector = true` and `store.noteDraft.focusRequested = true`) in `TaskKeyCommands` and `MomentumCommands`, and Add Dependency… (`store.dependencyPicker = DependencyPicker(task:)`) in `MomentumCommands` and the inspector. Each surface also decides on its own which task an action may act on: the keys refuse in Completed with a toast, the Task menu refuses under a sheet, and the context menu and the hover buttons check `isPending`. A change to what Edit… opens, or to when Add Note is offered, means finding every copy.

Proposal: one store method per action that opens something, which every surface calls. Designed twice:

- A table, `enum TaskCommand: CaseIterable`, with each action's title, key, shortcut, availability and effect, which the menus, the keys and the Keyboard Shortcuts sheet would read. Rejected: the surfaces show different subsets in different orders (the context menu has a divider before Delete…, the Task menu has ⌘ shortcuts, the hover buttons have three actions), so the table would grow flags per surface, and menus cannot be snapshotted, so most of it would be checked only by hand.
- Store intents, chosen: the smallest interface, and each surface keeps its own words and order.

```swift
extension AppStore {
    func beginEdit(_ task: TaskItem, field: EditField = .description)
    func confirmDelete(_ task: TaskItem)
    func confirmStopRecurrence(_ task: TaskItem)
    /// Opens the inspector and moves the focus to its Notes field.
    func writeNote(on task: TaskItem)
    func pickDependency(for task: TaskItem)
}
```

In `Model/TaskIntents.swift`, so `AppStore.swift` stays under 600 lines. Each method guards on `isPending`, which the surfaces then no longer repeat.

Benefits: the next action is one store method plus its menu items; each intent gets a store test instead of being reached only through a view.

Proof of same behavior: the key handler tests in `AppStoreTests` and `TaskDetailsTests` pass unchanged; the `edit`, `delete`, `stop-repeating`, `inspector-add-note` and `depends-picker` pages match their pixels before and after, apart from the noise the 2026-10-29 sign-in refactor found in a spinner. The menus are checked by hand, since no page can show one.

Plan:
1. `refactor(macos)`: add `Model/TaskIntents.swift` with the five intents and their tests.
2. `refactor(macos)`: one commit per surface file (`TaskKeyCommands`, `MomentumCommands`, `TaskListView`, `TaskRow`, `InspectorView`) to call them.

Why not Strong: each copy is one or two lines and none has drifted into a bug. It earns its place when the next milestone adds an action, as "Changing a note's words" in Later would.

---

## AppStore is back near 600 lines  [Speculative]

Files: `macos/Momentum/Model/AppStore.swift` (592 lines; the class is about 535)

Signal: a type over 400 lines, and a file 8 lines under 600.

Problem: M9 added 53 lines: six actions, the Add Dependency sheet's state, the note draft and `dependencies`. By section: Lifecycle 120, Mutations 132, Sign-in (the account and sign-out) 86, Plan Today 55, Derived state 52, Messages 17. The first 56 lines are six value types that are not the store (`ViewName`, `ResolvedSection`, `Toast`, `EditField`, `EditRequest`, `ServerState`).

Proposal: move those six types to `Model/ViewState.swift`, unchanged. The file drops to about 536 lines. Plan Today and the mutations stay: each uses four to six members of the store (`exclusive`, `refresh`, `undoWindow`, `show`, `noteServer`), so a model of their own would need a host for all of them, which the 2026-10-29 review's "Sub-models reach back into the whole store" card already weighed against.

Benefits: navigation, and room before the 600-line signal.

Proof of same behavior: it compiles; the tests run unchanged.

Plan: one `refactor(macos)` commit. Do it when the next milestone would take the file past 600, or with the first card, whose intents go in a file of their own anyway.

---

## The inspector's sections share one file  [Speculative]

Files: `macos/Momentum/Views/InspectorView.swift` (428 lines; `TaskDetails` is 344)

Signal: the file M9 changed most. It is under the 600-line signal and its type under 400.

Problem: `TaskDetails` holds the header, the action bar, the properties, Repeats, Depends on, Blocks, Notes and the Details disclosure, and the file holds `NoteField` too. The notes code (the cards, their × and context menu, the field, about 90 lines) and the dependency code (Depends on with its links and ×, Blocks, about 60) each have actions of their own.

Proposal: move them to `Views/InspectorNotes.swift` and `Views/InspectorDependencies.swift`, each a `View` that takes the task (and, for notes, the pane focus). `TaskDetails` keeps the layout and the shared `section` helper, which becomes file-internal.

Benefits: locality for the next notes or dependency story.

Proof of same behavior: the `inspector`, `inspector-add-note`, `inspector-note-error`, `inspector-depends`, `inspector-blocks` and `completed` pages match their pixels before and after.

Plan: two `refactor(macos)` commits. Do it when a notes or dependency story next starts.

---

## Top recommendation

**Nothing needs doing now.** No card is `Strong`, and the one worth exploring, the task intents, pays off when the next milestone adds an action rather than before. If you want one change before then, do the intents with the `ViewState.swift` move (cards 1 and 2), which together take `AppStore.swift` to about 536 lines and the next action to one place.

## Considered, not recommended

- **One `Overlay` enum for the eight sheet and prompt flags,** so that `hasOverlay` (`AppStore.swift:138`) could not miss one and one modal at a time would be structural. It is a real risk: M9 added `dependencyPicker` by hand, and only it and `planSheet` are pinned by a test. But the flags have 118 uses (51 for `planSheet`), most of them sheet bindings and tests, for a mistake that has not happened.
- **One `perform(_ change:)` on `TaskBridge` for every task change,** folding M9's four note and dependency methods into a `TaskAction` with payloads. The methods are one-line typed adapters at the JSON boundary; merging them saves two one-liners per operation in the fakes and changes the call strings the tests read. Each new operation costs about eight one-line edits across the Go client, the bridge, the Swift protocol and its three adapters, which M9 paid five times without trouble.
- **Moving Plan Today out of the store.** It uses five store members (see card 2).
- **Sharing `FakeBridge` and `SnapshotBridge`,** as on 2026-10-29. `SnapshotBridge` gained copies for the `duplicate` page; the two still do different jobs.
- **Splitting `PlanTodayView.swift`** (644 lines, unchanged since the last review, act line 1,000), and **moving the toast lifetime to a plain type**: as on 2026-10-29.
