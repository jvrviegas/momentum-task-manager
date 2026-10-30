# Implementation plan — Task details: notes, dependencies and duplicating

**Created:** 2026-10-29  
**Status:** D1–D5 approved on 2026-10-29, each as recommended. N01–N11 are done: stories S-065 to S-070 are `done` by their tests and snapshot pages. What is left is not code: deploying the server (C9), which is the owner's, and the stories' manual checks against a real server.  
**Baseline:** `c131b3d` on local `main` when the plan was written, with M9 added to `docs/STORIES.md` and not yet committed. `origin/main` is at `29f856a`, so none of this work is pushed. Recheck HEAD and the working tree before starting.  
**Stories:** M9 in [`docs/STORIES.md`](../STORIES.md), S-065 to S-070. Its proposed rules R1–R4 are D1–D4 here.  
**Related:** [backend plan](./backend.md) D7 (undo), [ADR 0002](../adr/0002-go-postgres-backend.md) (the server is the source of truth).

## Next-agent instructions

1. Read this plan, [`CONTRIBUTING.md`](../../CONTRIBUTING.md), M9 in [`docs/STORIES.md`](../STORIES.md), the non-goals of [`tui/DESIGN.md`](../../tui/DESIGN.md) (§3), and the local PostgreSQL steps in [`backend/README.md`](../../backend/README.md).
2. D1–D5 are the owner's calls, approved on 2026-10-29; do not reopen them. If one proves wrong in code, stop and report it.
3. Build in tracker order. N01–N06 (server, client, bridge, the Mac's bridge) serve every story; N07–N11 follow the stories. Before each Mac task, write the story's `UX:` block with the `ux-design` skill, as `improve-loop` does: this plan settles behavior, not surfaces, words or layout.
4. Mark a task complete only after its tests and gates pass, and record its commits in the tracker.
5. Commit as [`AGENTS.md`](../../AGENTS.md) says: one commit per file, a Conventional Commit message, only the intended file staged. Work on `main`, as M8 did. Do not push or deploy; before any Git operation that reaches `origin`, switch `gh` to `jvrviegas` and back, as `AGENTS.md` says.
6. Tests never reach a shared server or Neon: server tests run on the local PostgreSQL from `compose.yaml`, the TUI's client and the bridge use fake servers, and the Mac uses `FakeBridge` and `SnapshotBridge`.

## Current state (verified at planning time)

| Area | Where | What it means |
|---|---|---|
| Storage | `backend/internal/store/ent/schema/task.go:43`, `:44` | Every task already has `annotations` (JSON, entry and description) and `depends` (UUID strings). No schema change and no migration are needed. |
| Change request | `core/api/task.go:79` (`ModifyTaskRequest`), `:116` (`TagChange`) | A change can set the eight edit fields and add or remove tags. It cannot touch notes or dependencies. |
| Applying a change | `backend/internal/tasks/mutate.go:80` (`Modify`), `:209` (`updatePending`), `:218` (`update`) | A change edits an `api.Task` copy, saves it and records a `task_events` row with the task before and after. Only pending tasks change (`:211`). |
| Saving | `mutate.go:234` (`save`) | Writes the fields a change can touch. It never writes `annotations` or `depends`, so today they survive every change untouched. |
| Undo | `backend/internal/tasks/undo.go:103` (`revert`), `:116` | Undo puts back the task as it was before the latest change, through the same `save`. It would not bring back a note or a dependency until `save` writes them. Undoing a create removes the task (`:106`). |
| "Changes nothing" | `mutate.go:400` (`validateModify`), `:435` (`onlyRecurrence`) | A request that sets none of today's fields is refused as "the request changes nothing" (`:402`), so a notes-only request would be refused until `onlyRecurrence` counts the new fields. |
| Request decoding | `backend/internal/httpapi/tasks.go:153` | Unknown fields are ignored, so an older server reads a notes-only request as empty and answers "the request changes nothing". |
| Series | `backend/internal/tasks/recur.go:179` | A new instance copies a series' fields but not its notes or dependencies, unlike Taskwarrior. |
| Blocked tasks | `backend/internal/tasks/tasks.go:112` (`withUrgency`), `core/domain/urgency.go:31` | Urgency already counts a task blocked by a pending task (waiting ones included) and the task that blocks it. |
| Create | `core/api/task.go:62`, `mutate.go:36` | `POST /v1/tasks` takes description, project, priority, due, scheduled, recurrence, estimate and tags. It has no wait, until, notes or dependencies. |
| Plan tags | `core/domain/planner.go:12`, `:100` (`IsPlanTag`) | Today's plan is a `momentum-plan-YYYY-MM-DD` tag on each chosen task. |
| TUI client | `tui/internal/backend/client.go:67` (`Add`), `:75` (`Modify`), `:129` (`change`) | Builds requests from a `domain.NewTask` or `domain.TaskDiff` and drops the server's answer, so `Add` cannot say which task it created. |
| Bridge | `tui/internal/bridge/server.go:56` (`Client`), `:78` (`apiFields`), `ops.go:205` (`modify`), `:326` (`taskAction`) | `modify` sends the editor's diff; `complete`, `start`, `stop` and `delete` answer `{}`. `apiFields` names the edit field a server error belongs to. |
| Times on the wire | `tui/internal/bridge/wire.go:159` (`annotationWire`), `:423` (`stamp`) | The bridge sends a note's time as RFC 3339 to the second. |
| Load | `tui/internal/bridge/ops.go:30`, `backend/internal/tasks/tasks.go:38` | A load carries the pending tasks that are not waiting (the server's pending list leaves waiting ones out) and the last 30 days of completed tasks. |
| Wire contract | `tui/internal/bridge/wire_golden_test.go:130`, `macos/MomentumTests/WireContractTests.swift:23` | Each answer the Mac decodes is kept as a file and decoded by the Swift test; `{}` answers have no file. |
| Mac bridge | `macos/Momentum/Bridge/ProcessBridge.swift:15` (`TaskBridge`), `:108`; `MomentumTests/AppStoreTests.swift:7` (`FakeBridge`); `Snapshot/SnapshotBridge.swift:240` | Each new method lands in the protocol and its three adapters. |
| Mac changes | `macos/Momentum/Model/AppStore.swift:301` (`exclusive`), `:316` (`didMutate`), `:322` (`mutate`) | One change at a time; a success loads again and opens the undo window with a toast. |
| Inspector | `macos/Momentum/Views/InspectorView.swift:89` (Depends on), `:101` (Notes), `:103` | Shows dependencies by description (a link that selects the task) or by UUID when not loaded, and notes with their time; an empty list says "No annotations.". |
| Task commands | `macos/Momentum/Views/TaskListView.swift:164` (`TaskMenu`), `macos/Momentum/App/MomentumCommands.swift:50` (Task menu), `Views/TaskKeyCommands.swift:62` | The context menu and the Task menu offer Complete, Start, Edit…, Stop Repeating… and Delete…; a bare `d` is Delete…; ⌘D is free. A completed task's menu has only Show Details. |
| Seed | `macos/Momentum/Snapshot/SnapshotSeed.swift:87`, `:102` | The seed already has a task with a note and one that depends on another. |
| TUI details | `tui/internal/ui/details.go:208`, `:235` | The terminal app shows notes, and dependencies as raw UUIDs. |
| Docs | `tui/DESIGN.md:35`, `macos/README.md:102`, `:104` | Editing notes and dependencies is a v1 non-goal; the README's deferred table says No for both apps on editing and on duplicating. |

## Decisions (2026-10-29, approved by João Viegas 2026-10-29)

Approved each as recommended.

| ID | Decision | Recommended | Trade-off and basis |
|---|---|---|---|
| D1 | Which apps edit notes and dependencies (M9's R1) | **The Mac app only.** The terminal app keeps showing them and gets every change from the server. | Keeps M9 to one UI. The terminal app stays read-only for these, so a note can be added on the Mac only, and `tui/DESIGN.md:35` keeps the non-goal for the TUI. Rejected: both apps at once, as M8's D6 did for Google, which would double the UI work for a feature the TUI's design rules out. |
| D2 | What a task may depend on | **Any of the account's tasks that is pending or completed.** The Mac's picker offers pending tasks only. | Taskwarrior allows a completed dependency and imports carry them, so refusing them would make a server answer differ from an imported task. A completed dependency blocks nothing (C11). Rejected: pending only on the server, which is stricter but refuses what the data already holds. |
| D3 | How notes change (R3) | **Add and remove only.** A note keeps the time it was added; changing a note's words waits in Later. | Matches Taskwarrior's annotate and denotate and keeps the change small. Correcting a typo means removing the note and adding it again, with a new time. |
| D4 | What a duplicate is (R4) | **A new task with the original's description, project, priority, due, scheduled, estimate and tags, less the plan tag.** No notes, dependencies, start, repeat, wait or until. The description is the same words, without "copy". | The copy is a fresh start: not in today's plan unless planned, not repeating, not started. Keeping the words lets you change only what differs; Finder's "copy" suffix would need deleting each time. Rejected: copying notes and dependencies, which would need the create route to take them (a server change). |
| D5 | Which tasks offer Duplicate | **Pending tasks only.** | Keeps S-016's rule that a completed task's menu offers only Show Details. "Do this again" from Completed is a real use, and allowing it later only adds the menu item, since the copy is made from the task either way. |

## Consequences (settled by the stories, the code or a decision above; not for approval)

| ID | Source | Consequence | What it means in practice |
|---|---|---|---|
| C1 | `core/api/task.go:116` | `ModifyTaskRequest` gains `annotations` (`add`: the words of new notes; `remove`: notes by `entry` and `description`) and `depends` (`add` and `remove`: task UUIDs), shaped like `tags`. Each is optional and left out when unused. | One route, `PATCH /v1/tasks/{uuid}`, keeps its idempotency key and its `task_events` row. Requests the TUI sends today are unchanged byte for byte. |
| C2 | `mutate.go:234`, `undo.go:116` | `save` writes `annotations` and `depends`. | Undo takes back an added or removed note or dependency, as it takes back any edit. Every other change keeps them, since the task before and after carries the same ones. |
| C3 | `mutate.go:435` | `onlyRecurrence` counts the new fields. | A request that only adds a note is a change, not "changes nothing", and a recurring template still takes no field but its recurrence. |
| C4 | `wire.go:423` | A new note's time is the server's, truncated to the second, as Taskwarrior's are. A note to remove is found by its time to the second and its exact words. | The Mac sends back the time it was given, and it matches. Two notes with the same words in the same second are one note for removal: removing removes one. |
| C5 | `mutate.go:211` | Only pending tasks take a note or a dependency; the server answers the existing conflict "the task is completed; only pending tasks can change". | Completed stays read-only; the Mac offers nothing there (S-065). |
| C6 | `recur.go:179` | Notes and dependencies stay on the instance they were added to. | A note on this week's "Water the plants" is not on next week's. |
| C7 | `schema/task.go` (`Policy`), `mutate.go:526` | A dependency on the task itself, a malformed or unknown UUID, another account's task (unknown to this one), a deleted task, a recurring template, or one that closes a loop is refused as `invalid` on `depends` with a reason, and nothing changes. The loop check walks the account's dependencies inside the change's transaction. | Another account's task gets the same "no such task" as a UUID that never existed, so nothing leaks. |
| C8 | S-066, S-068 | Removing a note or dependency the task no longer has is a `conflict`. | Another device removed it first; the Mac's toast gives the reason, and the next refresh shows the task as it is. Corrected while building S-066: the plan first said the Mac loads again at once, which no refused change in the app does. |
| C9 | `httpapi/tasks.go:153`, `mutate.go:402` | The server must be deployed before a Mac with M9 uses it. An older server answers "the request changes nothing" and changes nothing. | No silent loss; the deployed server (`backend/DEPLOY.md`) is updated first, by the owner. |
| C10 | `bridge/server.go:56`, `ops.go:326` | The bridge gains five operations: `addNote`, `removeNote`, `addDependency` and `removeDependency` answer `{}` like `complete`; `duplicate` answers the copy's UUID. `apiFields` maps the server's `annotations` to `note` and `depends` to `depends`. | Only `duplicate` needs a wire file (`duplicate.json`) and a Swift fixture; a refused note or dependency arrives on its own field through the existing `error-field` shape. |
| C11 | `tasks.go:112`, `tasks.go:38`, `ops.go:30` | The Mac counts a task blocked while one of its dependencies is a pending task in the load. The load leaves out waiting tasks, which the server's urgency counts. | The row's symbol and the server's order agree, except for a dependency on a waiting task, which only an import can make: the server counts it, and the Mac shows no symbol. A completed, deleted or unloaded dependency blocks nothing. Corrected while building N04: the plan first said the load carries waiting tasks. |
| C12 | `client.go:129`, D4 | Duplicating reads the task from the server's pending list and creates the copy with its exact times through `POST /v1/tasks`, so the server does not change. The TUI client gains `Duplicate`, which returns the new UUID. | The copy's due keeps its seconds (an imported `23:59:59` stays so). A task that stopped being pending since the last load is refused with a conflict. |
| C13 | `undo.go:106` | Undo after Duplicate removes the copy. | Same as undoing New Task. |
| C14 | `AppStore.swift:301`, `:316` | Each change goes through `exclusive` and `didMutate`: one at a time, then a load and the undo window with a toast. After Duplicate, the store selects the copy once the load has it. | Behaves like every other change, including MUT-05's "another change is still being saved". |
| C15 | `TaskKeyCommands.swift:62`, Mac convention | Duplicate is ⌘D in the Task menu; a bare `d` stays Delete…, as in the terminal app. | No key changes meaning. |
| C16 | `InspectorView.swift:103`, row help at `TaskRow.swift:27` | "No annotations." becomes "No notes." | One name, Note, as the row's hover text already says; the glossary gains Note with S-065's `UX:` block. |
| C17 | `SnapshotSeed.swift:87`, `:102` | The seed already has a note and a dependency; M9 adds snapshot pages, not seed tasks. S-067's line saying the seed "gains" one is corrected with this plan. | Snapshot pages show only what M9 lets anyone make. |
| C18 | D1, `tui/DESIGN.md:35`, `macos/README.md:102`, `:104` | The non-goal line says the Mac app edits notes and dependencies and the terminal app does not; the README's table says Yes for macOS on both rows. | The docs say what the apps do. |

Rejected while planning: separate routes such as `POST /v1/tasks/{uuid}/notes` (a second way to change a task, with its own idempotency and undo path, for no gain over C1); identifying notes by their position in the list (a concurrent removal shifts it and removes the wrong note); adding IDs to notes (a schema change and a migration for what C4 already does).

## Contracts

**Server (`core/api`).** `ModifyTaskRequest` gains:

```go
Annotations *AnnotationChange `json:"annotations,omitempty"`
Depends     *DependencyChange `json:"depends,omitempty"`

// AnnotationChange adds notes, by their words, and removes notes, by their
// time and words.
type AnnotationChange struct {
	Add    []string     `json:"add,omitempty"`
	Remove []Annotation `json:"remove,omitempty"`
}

// DependencyChange adds and removes the tasks a task depends on, by UUID.
type DependencyChange struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}
```

The answer is the changed task, as for every modify. Refusals: `invalid` on `annotations` (an empty note) or `depends` (C7); `conflict` for a task that isn't pending (C5) or a note or dependency it no longer has (C8).

**TUI client (`tui/internal/backend`).** `AddNote(ctx, uuid, text)`, `RemoveNote(ctx, uuid, entry time.Time, text)`, `AddDependency(ctx, uuid, on)`, `RemoveDependency(ctx, uuid, on)`, and `Duplicate(ctx, uuid) (string, error)`.

**Bridge.** `addNote {uuid, text}`, `removeNote {uuid, entry, text}` (the `entry` the load sent), `addDependency {uuid, on}` and `removeDependency {uuid, on}` answer `{}`; `duplicate {uuid}` answers `{uuid}`.

**Mac (`TaskBridge`).** `addNote(uuid:text:)`, `removeNote(uuid:note:)`, `addDependency(uuid:on:)`, `removeDependency(uuid:on:)`, and `duplicate(uuid:) -> String`.

## Tracker

| Status | ID | Task | Depends on | Stories | Verification |
|---|---|---|---|---|---|
| [x] | N01 | `core/api`: `AnnotationChange`, `DependencyChange` and the two fields (C1) | Approval | S-065–S-068 | `core/api` cases A1–A2. Go gates in `core/`. Done: `290998c`, `0a7328a`. A2 is the existing round trips, whose exact strings did not change. |
| [x] | N02 | Backend `internal/tasks`: apply notes and dependencies in `Modify`, with C3, C4, C5, C7 and C8; `save` writes both (C2) | N01 | S-065–S-068 | Server cases S1–S17 on local PostgreSQL. Go gates in `backend/`; `docker build -f backend/Dockerfile .`. Done: `e8ac558`–`b1f3dab`. Five new tests (`TestModifyAddsAndRemovesNotes`, `TestModifyAddsAndRemovesDependencies`, `TestUndoTakesBackNotesAndDependencies`, `TestOtherChangesKeepNotesAndDependencies`, `TestNotesAndDependenciesStayOnTheirInstance`); all gates and the Docker build pass. Four deliberate breakages (`save` skipping notes, `onlyRecurrence` ignoring notes, no loop check, an exact note time) were each caught. The server's clock is already UTC to the second (`tasks.go:34`), so C4 needed no new code. Refusals: "a note cannot be empty", "a task cannot depend on itself", "no such task", "a task cannot depend on a deleted task", "a task cannot depend on a recurring template; choose one of its tasks", "a task cannot depend on a task that depends on it"; conflicts "the task has no such note" and "the task does not depend on that task". |
| [x] | N03 | TUI client: the four change methods and `Duplicate` (C12) | N01 | S-065–S-068, S-070 | Client cases K1–K6 against a fake server. Go gates in `tui/`. Done: `96f3cfa`, `d7f7a86`. |
| [x] | N04 | Bridge: the five operations, `apiFields`, the `Client` interface; `duplicate.json` from `TestWireTasksAndCapture` (C10) | N03 | S-065–S-068, S-070 | Bridge cases B1–B5; `go test ./internal/bridge -run TestWire -update` writes one new file. Go gates and `go build ./cmd/momentum` in `tui/`. Done: `9cdc226`–`668314e`, ordered so each commit builds and passes. `-update` wrote only `duplicate.json`. B6 needed no case: bad parameters already answer "invalid parameters" through `decode`, and the server refuses a missing note or task. |
| [x] | N05 | Mac bridge: the five `TaskBridge` methods in `ProcessBridge`, `FakeBridge` and `SnapshotBridge`; wire types; the `duplicate` fixture | N04 | all | Swift case W1 and the full Swift suite. Done: `15c01f2`–`a4586a4`. The parameter types and `Duplicated` sit with the others in `ProcessBridge.swift`, so `Wire.swift` did not change. Between `d5fd5a6` (the wire file) and `a4586a4` (its fixture), `everyFileHasATypeHereAndEveryTypeAFile` failed, since one file per commit allows no order without that gap. |
| [x] | N06 | Mac store: `addNote`, `removeNote`, `addDependency`, `removeDependency`, `duplicate`, the dependency candidates and `isBlocked` (C11, C14) | N05 | all | Swift cases M1–M10. Done: `b2e2c7a`, `d2179fb`, `4137ec7`. The candidates, blockers and dependents are a plain `TaskDependencies` (new `Model/TaskDependencies.swift`), which keeps `AppStore` at 581 lines. The toasts' words wait for each story's `UX:` block. 158 Swift tests pass. |
| [x] | N07 | Mac inspector, notes: add and remove on a pending task; "No notes." (C16) | N06, S-065 and S-066 `UX:` | S-065, S-066 | Pages `inspector`, `inspector-add-note`, light and dark. Done: UX `23531d2`, `e94956f`; S-065 `2594044`–`769966f`; S-066 `bf80238`, `619fdcb`. The draft is `NoteDraft` (new `Model/NoteDraft.swift`), owned by the store, so `a`, Task ▸ Add Note and the pages reach it; the pane focus gained `.note`, which Escape hands back to the list. Two more pages, `inspector-note-error` and `completed` ("No notes."). `7fe05a1` (`ContentView` passing the focus) does not build alone: the inspector's new `focus` parameter comes with `e035304`, and one file per commit allows no other order. |
| [x] | N08 | Mac dependencies: the picker and Remove under Depends on | N06, S-067 and S-068 `UX:` | S-067, S-068 | Pages `depends-picker`, `inspector-depends`. Done: UX `4873487`, `e4b4fc5`; S-067 `aebb61f`–`f8de4c3`; S-068 `709003e`, `6b1f8db`. The picker is a sheet (`Views/DependencyPickerView.swift`, state `DependencyPicker`), since a popover is a window of its own that no snapshot page can show. Depends on now shows on every pending task. A wrapped link under it was centred by the link button style, and is now left-aligned. |
| [x] | N09 | Mac blocked: the row's symbol and the inspector's list of tasks waiting on this one | N06, S-069 `UX:` | S-069 | Pages `inbox`, `inspector-blocks`. Done: UX `0fb9514`, `b7e5902`; `60ba2cd`–`7a267b6`. The seed's only blocked task is in Inbox, so `inbox` shows the hourglass and the new `inspector-blocks` the Blocks list. |
| [x] | N10 | Mac Duplicate: `TaskMenu`, the Task menu with ⌘D, the Keyboard Shortcuts sheet (C15, D5) | N06, S-070 `UX:` | S-070 | Pages `duplicate`, `shortcuts`; the menus are a manual check, since no page can show a menu. Done: UX `e68d37c`, `58b9596`; `3a7abc5`–`b94ea7f`. The snapshot bridge keeps its copies, so `duplicate` shows a real one. Found, not changed: the Keyboard Shortcuts sheet runs past the bottom of the 720-point window, as it did before. |
| [x] | N11 | Docs: `tui/DESIGN.md:35`, `macos/README.md` (the table and a section), `backend/README.md` (the two fields, and that the server goes out before a Mac that uses them, C9), the glossary's new terms (C18) | N07–N10 | last story | Every field, operation and term above appears once. Done: `82a008a`, `63ffc94`, `f73573c`; the glossary's Note, Remove, Dependency, Blocked, Blocks and Duplicate came with each story's `UX:` block. |
| [x] | — | Full gates green; each story's `Status:` set; this tracker and the Progress log updated | N11 | — | The gates below. Done: `gofmt`, `go test`, `go test -race` and `go vet` pass in `core/`, `backend/` (local PostgreSQL) and `tui/`, `go build ./cmd/momentum` and the server's Docker build pass, 164 Swift tests pass, and `tools/snap.sh` took all 75 pages in light and dark. |

## Test cases

**`core/api` (N01), `api_test.go`:**

| # | Case | Asserts | Stories |
|---|---|---|---|
| A1 | A request with only `annotations` or only `depends` | Encodes the field alone; decodes back the same | S-065, S-067 |
| A2 | A request with neither | Encodes exactly as before N01 (no new keys) | regression |

**Server (N02), `tasks_test.go` and `undo_test.go`, on local PostgreSQL:**

| # | Case | Asserts | Stories |
|---|---|---|---|
| S1 | Add a note to a task with one | Stored after it, with the server's time to the second (C4); one `task_events` row | S-065 |
| S2 | Add an empty or blank note | `invalid` on `annotations`; task and events unchanged | S-065 |
| S3 | Add a note to a completed task, and to a recurring template | `conflict` (C5) | S-065 |
| S4 | Undo after adding a note | The note is gone | S-065 |
| S5 | Remove a note by time and words | Only that note goes; another with the same words at another time stays | S-066 |
| S6 | Remove a note the task no longer has | `conflict` (C8); nothing changes | S-066 |
| S7 | Undo after removing a note | The note is back in its place with its time | S-066 |
| S8 | Add a dependency on a pending and on a completed task (D2) | Both stored; one event each; the blocked task's urgency drops | S-067 |
| S9 | Depend on itself, a malformed UUID, an unknown UUID, another user's task, a deleted task, a template | Each `invalid` on `depends` with a reason; no event; another user's task gets the same answer as an unknown one | S-067 |
| S10 | Close a loop: A on B then B on A; A on B, B on C, then C on A | `invalid` on `depends`; nothing changes | S-067 |
| S11 | Undo after adding a dependency | It is gone | S-067 |
| S12 | Remove a dependency the task has, and one it doesn't | Removed; then `conflict` (C8) | S-068 |
| S13 | Undo after removing a dependency | It is back | S-068 |
| S14 | A notes-only request | Not "the request changes nothing" (C3) | S-065 |
| S15 | Change a description on a task with notes and dependencies, then undo | Both survive the change and the undo (C2) | regression |
| S16 | Retry an add-note with the same idempotency key | Applied once | S-065 |
| S17 | Complete a repeating instance that has a note and a dependency | The next instance has neither (C6) | regression |

**TUI client (N03), `tui/internal/backend/client_test.go`, fake server:**

| # | Case | Asserts | Stories |
|---|---|---|---|
| K1 | `AddNote` | One `PATCH` with `annotations.add` only | S-065 |
| K2 | `RemoveNote` | `annotations.remove` with the time in RFC 3339 and the words | S-066 |
| K3 | `AddDependency`, `RemoveDependency` | `depends.add` or `depends.remove` only | S-067, S-068 |
| K4 | `Duplicate` of a planned, repeating, started task with notes | `POST /v1/tasks` with the D4 fields and exact times; no plan tag, recurrence, notes or dependencies; returns the new UUID | S-070 |
| K5 | `Duplicate` of a task not in the pending list | A conflict error; nothing created | S-070 |
| K6 | A server field error on `depends` | Reaches the caller with its field | S-067 |

**Bridge (N04), `tui/internal/bridge`:**

| # | Case | Asserts | Stories |
|---|---|---|---|
| B1 | Each of the four change operations | Calls the client with the uuid and values; answers `{}` | S-065–S-068 |
| B2 | `removeNote` with the `entry` the load sent | The client gets the same instant | S-066 |
| B3 | A server error on `annotations` or `depends` | Answers a field error on `note` or `depends` | S-065, S-067 |
| B4 | `duplicate` | Answers the copy's UUID | S-070 |
| B5 | `TestWireTasksAndCapture` with `duplicate` | Writes `testdata/wire/duplicate.json`; fails when its answer changes | S-070 |
| B6 | Unknown uuid or missing parameters | The bridge's usual invalid-request error | all |

**Mac (N05, N06), `MomentumTests`:**

| # | Case | Asserts | Stories |
|---|---|---|---|
| W1 | `WireContractTests` with the `duplicate` fixture | Every key is read | S-070 |
| M1 | Add a note | `addNote` on the selected task, a load, the undo window and a toast naming the task | S-065 |
| M2 | A refused note | The field error reaches the caller; no load; the undo window stays closed | S-065 |
| M3 | Remove a note | `removeNote` with the note's time and words; a load; the undo window | S-066 |
| M4 | Add and remove a dependency | The bridge calls; a load; the undo window | S-067, S-068 |
| M5 | Remove a dependency shown only by its UUID | Allowed | S-068 |
| M6 | Dependency candidates | Pending tasks only, never the task itself or one that would close a loop through the loaded dependencies | S-067 |
| M7 | `isBlocked` | True while a dependency is a loaded pending task; false for completed, deleted or unloaded ones | S-069 |
| M8 | Duplicate | `duplicate`; a load; the copy selected; the undo window and a toast | S-070 |
| M9 | Undo after Duplicate | Calls `undo`, which removes the copy on the server (C13); the selection moves as after any change | S-070 |
| M10 | Any of these while another change is saving | "Another change is still being saved" (C14) | all |

## Acceptance criteria

| Criterion (from `docs/STORIES.md`) | Proved by |
|---|---|
| S-065: a note is stored after the others with the server's time; an empty one is refused; a non-pending task refuses it; Undo removes it | S1–S4, S14, S16 |
| S-065: the Mac sends it, loads, opens the undo window; a refusal stays on its field | M1, M2, B3 |
| S-065: the Notes section offers adding on a pending task; "No notes." | N07 pages `inspector-add-note`, `inspector` |
| S-065: the terminal app shows the note after a refresh | Manual, against the local server from `compose.yaml` |
| S-066: removes that note only; a missing note is a conflict; Undo restores it | S5–S7, K2, B2, M3 |
| S-067: refusals for itself, unknown, deleted, another account's and loops; Undo removes it | S8–S11, K6 |
| S-067: the picker offers pending tasks, never itself or a loop | M6, N08 page `depends-picker` |
| S-068: a missing dependency is a conflict; Undo restores it; a UUID-only entry can be removed | S12, S13, M4, M5 |
| S-069: blocked while a dependency is pending; the row's symbol; the inspector's list | M7, N09 pages |
| S-070: the D4 fields, no plan tag or repeat; the copy selected; Undo removes it | K4, K5, B4, B5, W1, M8, M9 |
| S-070: Duplicate after Edit…; ⌘D; `d` stays Delete… | N10 pages `context-menu`, `menu`, `shortcuts` |
| Nothing else changes: edits, undo and the TUI's requests | A2, S15, the existing suites |

**Gates.** In `core/`, `backend/` and `tui/`: `test -z "$(gofmt -l .)" && go test ./... && go test -race ./... && go vet ./...`, with `MOMENTUM_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable` and `docker compose up -d --wait postgres` for `backend/`; then `go build ./cmd/momentum` in `tui/` and `docker build -f backend/Dockerfile .` from the root. In `macos/`: `xcodegen generate && xcodebuild -scheme Momentum -destination 'platform=macOS' test`, and `tools/snap.sh` for the pages each Mac task names.

## Files touched

| Where | Change |
|---|---|
| `core/api/task.go`, `api_test.go` | Modified: the two change types and fields (N01) |
| `backend/internal/tasks/mutate.go`, `tasks_test.go`, `undo_test.go` | Modified: applying, checking and saving notes and dependencies (N02) |
| `tui/internal/backend/client.go`, `convert.go`, `client_test.go` | Modified: four change methods and `Duplicate` (N03) |
| `tui/internal/bridge/server.go`, `ops.go`, `wire.go`, their tests, `wire_golden_test.go`, `testdata/wire/duplicate.json` | Modified and new: five operations (N04) |
| `macos/Momentum/Bridge/ProcessBridge.swift`, `Snapshot/SnapshotBridge.swift`, `MomentumTests/AppStoreTests.swift`, `WireContractTests.swift` | Modified: the methods and their adapters (N05) |
| `macos/Momentum/Model/AppStore.swift`, `Model/TaskDependencies.swift`, `MomentumTests/TaskDetailsTests.swift` | Modified and new: the store's actions and the dependency rules (N06) |
| `macos/Momentum/Model/NoteDraft.swift`, `Views/DependencyPickerView.swift` | New: the note draft and the Add Dependency sheet (N07, N08) |
| `macos/Momentum/Views/InspectorView.swift`, `ContentView.swift`, `TaskRow.swift`, `TaskListView.swift`, `TaskKeyCommands.swift`, `ToastView.swift`, `App/MomentumCommands.swift`, `Snapshot/SnapshotScenes.swift`, `Snapshot/SnapshotBridge.swift` | Modified: the Mac UI and its pages (N07–N10) |
| `tui/DESIGN.md`, `macos/README.md`, `backend/README.md`, `docs/GLOSSARY.md`, `docs/STORIES.md` | Modified: docs (N11) |

Unchanged on purpose: the database schema and migrations (C1), `POST /v1/tasks` (C12), the series code `recur.go` (C6), and the terminal app's UI (D1).

## Out of scope

- Changing a note's words (D3).
- Editing notes or dependencies in the terminal app (D1).
- Duplicating a completed task (D5).
- Copying notes or dependencies to a duplicate or to a series' next instance (D4, C6).
- The terminal app showing dependencies by description instead of UUID (`tui/internal/ui/details.go:235`); worth its own task.
- Deploying the server (C9), which is the owner's.

## Progress log

- 2026-10-29 — plan written against local `main` `c131b3d`. No code yet. M9 in `docs/STORIES.md` now names D1–D5 instead of R1–R4, and S-065's and S-067's lines follow C10 and C17.
- 2026-10-29 — D1–D5 approved by the owner, each as recommended, after the review page. No consequence was disputed.
- 2026-10-30 — N01–N06 done (`290998c`–`4137ec7`), every gate green. C11 corrected: the load leaves out waiting tasks. Next: S-065's `UX:` block, then N07.
- 2026-10-30 — N07–N11 done (`23531d2`–`f73573c`): S-065 to S-070 are `done`. C8 corrected: a refused removal shows its reason and does not load again at once. Not tried by hand: the menus, ⌘D, `a`, Return in the note field and the picker's keys, and every story's check against a real server, which needs the server deployed (C9).
