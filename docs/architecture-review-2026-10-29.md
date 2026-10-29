# Architecture review, 2026-10-29

Scope: the Mac app in `macos/` and the Go bridge it runs (`tui/internal/bridge`). Taken at the end of M8, with `main` clean at `b3add46`. The survey read the code and the git history; it did not build the app or run the tests, so the "before" run in each plan is still to be recorded.

**Overall: the code is in good shape.** The Mac app is about 11,000 lines of Swift 6 with strict concurrency, no file over 650 lines, no drifted `private struct` copies, and a `TaskBridge` seam that already has three adapters (the real process, the test fake, the snapshot bridge). The cards below are small. Only the first one closes a gap that no test closes today.

Not assessable: `docs/FIXLOG.md` does not exist, so the "same area in 3 fix-log entries" signal could not be checked. There is no `ARCHITECTURE.md` either; `CONTRIBUTING.md` names `macos/` in one line. Step 4 of the refactor would add `macos/ARCHITECTURE.md`.

Hot spots (commits touching the file in the last 30 days): `Bridge/Wire.swift` 13, `Model/AppStore.swift` 11, `Snapshot/SnapshotScenes.swift` 11, `Bridge/ProcessBridge.swift` 10; on the Go side `bridge/server.go` 12, `bridge/ops.go` 9, `bridge/wire.go` 8. One feature, Google Calendar, touched 26 non-doc files on the Mac side (`SnapshotScenes.swift` and `Wire.swift` six times each) and 10 more in the Go bridge.

---

## The bridge's wire format has no proof that both sides agree  [Strong]

Files: `macos/Momentum/Bridge/Wire.swift` (462 lines, 43 types), `tui/internal/bridge/wire.go` (31 structs), `core/api/*.go`, `macos/MomentumTests/GoogleCalendarTests.swift`, `ModelTests.swift`

Signal: one feature touched more than 2 targets, and the file that holds the hand-mirrored contract is the most-changed Swift file.

Problem: `Wire.swift` begins "Mirrors internal/bridge/wire.go", and that comment is the only link. The Swift tests decode JSON typed by hand (`GoogleCalendarTests.swift:309` decodes `{"offered":true,"connected":true,...}`), and the Go tests assert Go values, never the JSON that leaves the bridge. Measured while doing this card: renaming `completedError` in `wire.go` left every existing test green, the bridge's Go tests and all 146 Swift tests, while the app would have stopped showing that error. (Renaming `needsReconnect` in `core/api` does fail `TestGoogleCalendarJSON`, but that test pins the server's contract and is updated along with the rename; nothing then flags the Mac app.) I compared the names: all 157 Swift property names match a Go tag, a computed property, or a `CodingKeys` rename (`number = "id"`), so there is no drift today. Nothing keeps it that way.

Proposal: no change to behavior. A Go test (`wire_golden_test.go`) sends each op through the bridge's real `Serve` path and keeps the answer as a file in `tui/internal/bridge/testdata/wire/`, failing when a file would change unless run with `-update`. A Swift test (`WireContractTests.swift`) decodes every file with the app's own decoder, and also walks the JSON against the decoded value to check that every key reached a property: an optional property whose key was renamed decodes as `nil` without an error, so decoding alone would miss it. The only production edit is making `ProcessBridge.decoder` and `Changed` internal so the test can use them.

Benefits: locality, since a wire change now fails in one named test on the side that did not change; leverage, since each new bridge op adds one fixture and no new test logic. The tests that only decode hand-typed copies of Go's answers can then go.

Proof of same behavior: no behavior changes. The proof is that the new test fails when a Go tag is renamed (try it once, then revert) and the existing suites still pass.

Plan:
1. `test(bridge)`: the golden files, then the Go test that writes and checks them.
2. `refactor(macos)`: `ProcessBridge.decoder` and `Changed` internal.
3. `test(macos)`: the Swift contract test reading those files.
4. `test(macos)`: delete the tests that only decode hand-typed copies of answers that now exist as files.
5. `docs`: how to update the files, in `CONTRIBUTING.md` and `macos/README.md`.

Done on 2026-10-29. Results: 30 files (25 answers, 5 more for the keys Go leaves out when there is no trouble: `configError`, `restartRequired`, `completedError`, `calendarError`, `estimateWarning`, `estimateRaw`, `tasksError`, `taskError`, `stale`). The files are identical under four time zones and with `-race`. Sabotage check, then reverted: renaming `needsReconnect` fails the Swift test with `keyNotFound`, and renaming the optional `completedError` fails it with `keys the app did not read: /completedErr`. Only two tests were deleted, the two in `GoogleCalendarTests.swift` that decoded hand-typed Go answers and nothing else. Seven hand-typed decode tests stay because they also assert values (a date's instant, a `CodingKeys` mapping) that the contract test does not.

Limits: this covers the bridge's answers, not the parameters the app sends (`ProjectEdit`, `ReviewSubmission`, `CalendarInput`, `CalendarChange`), which is the same gap the other way. Reading the files by `#filePath` works from a checkout; a test bundle run on another machine would need them as resources in `project.yml`. One thing the files show: a calendar file's `modified` in the settings answer is in the machine's own UTC offset, while every other timestamp is UTC. It decodes fine, so it was left alone.

---

## Sign-in is a state machine inside the store  [Worth exploring]

Files: `macos/Momentum/Model/AppStore.swift` (610 lines; sign-in is lines 467–610), `macos/MomentumTests/ServerTests.swift`

Signal: a type over 400 lines.

Problem: `AppStore` holds the task list and selection, mutations and undo, Plan Today, toasts, and sign-in. The sign-in part is 144 lines with a state of its own (`signIn`, `signInTask`, the polling loop with `slowDown`, the Keychain save and its rollback, revoking the previous token) and touches the rest of the store at only a few points: `account`, `open()`, `show`, and `signOut` clearing the snapshot. `ServerTests.swift` builds a whole `AppStore` 12 times, mostly for its nine sign-in and sign-out tests. A change to sign-in means reading 610 lines to find the 144 that matter.

Proposal: a sixth sub-model next to `settings`, `catalog`, `diagnostics`, `macCalendar` and `googleCalendar`, but with explicit dependencies instead of the `weak var store` the others use (see the next card).

```swift
@MainActor @Observable
final class SignInModel {
    private(set) var flow: SignInFlow?
    init(bridge: any TaskBridge, signIns: any SignInStore,
         sleep: @escaping @Sendable (TimeInterval) async throws -> Void)
    @discardableResult func begin(server: String) -> Task<Void, Never>
    func cancel()
    /// Runs once the Keychain holds the new sign-in and the bridge has its token.
    var onSignedIn: (_ saved: SavedSignIn, _ previous: SavedSignIn?) async -> Void = { _, _ in }
}
```

`AppStore` keeps `account`, `open()` and `signOut`'s reset of the task state; `SignInFlow` and the views that read `store.signIn` read `store.signInModel.flow` instead.

Benefits: `AppStore` drops to about 470 lines; the polling and Keychain rules are tested with a fake bridge, a fake store and a fake `sleep`, with no `AppStore`.

Proof of same behavior: the nine sign-in and sign-out tests in `ServerTests.swift` pass unchanged before and after (they go through `AppStore`); the `sign-in` and `sign-in-failed` snapshot pages are byte-identical.

Plan:
1. `refactor(macos)`: add `SignInModel` in `Model/SignInModel.swift` with its own tests.
2. `refactor(macos)`: move `AppStore.beginSignIn`, `cancelSignIn`, `runSignIn`, `finishSignIn` to it and forward from the store.
3. `refactor(macos)`: point the views at the model and delete the forwards.
4. `test(macos)`: move the flow tests off `AppStore`; keep one end-to-end test through it.

Why not Strong: the gain is file size and reading time, not a bug it prevents.

---

## Sub-models reach back into the whole store  [Speculative]

Files: `Model/SettingsModel.swift`, `ProjectsModel.swift`, `GoogleCalendar.swift`, `MacCalendar.swift`, `DiagnosticsModel.swift`

Signal: a dependency that cannot be built without its owner.

Problem: five models hold `weak var store: AppStore?` and use it for five things: `bridge`, `account != nil`, `refresh()`, `show(…)` and `closeUndoWindow()` (plus `macCalendar` from `SettingsModel` and `DiagnosticsModel`). 15 sites guard it, and every one returns silently when the store is gone. No bug follows today, because the store outlives the models it owns.

Proposal: a `ModelHost` protocol with those five members, `AppStore` as the one adapter.

Benefits: the models could be built in a test without an `AppStore`.

Proof of same behavior: the existing model tests, unchanged.

Plan: not worth a plan yet. With one adapter and tests that already pass through the real store (19 `AppStore(` constructions), this is a hypothetical seam. It becomes real if the `SignInModel` card is done and a second model wants a fake host, or if a model is ever used outside the store.

---

## Plan Today's capacity views share a file with the sheet  [Speculative]

Files: `macos/Momentum/Views/PlanTodayView.swift` (644 lines)

Signal: a Swift file over 600 lines. The act line is 1,000.

Problem: seven views and `PlanKeyCommands` in one file. The last 276 lines (`CapacityPanel`, `CapacityBar`, `CalendarCommitments`, `BufferStripes`) draw the capacity table and the meetings strip and do not touch the sheet's list or keys.

Proposal: move those four to `Views/PlanCapacityViews.swift`, unchanged, making `private` types file-internal.

Benefits: navigation only.

Proof of same behavior: the `plan-today*` snapshot pages are byte-identical.

Plan: one `refactor(macos)` commit. Leave it until the file passes 1,000 lines or a feature next changes the capacity strip.

---

## Top recommendation

**The bridge's wire format has no proof that both sides agree**, the only `Strong` card. It is the hottest area (`Wire.swift`, `bridge/server.go`, `bridge/ops.go`), the change is tests and fixtures only, and it closes the one failure that neither the Swift nor the Go suite can see. If you also want to shrink `AppStore`, do the sign-in card second; it needs no decision from the first.

## Considered, not recommended

- **Split `TaskBridge` (26 methods) into one protocol per feature.** The width is the feature count. One `FakeBridge` and one `SnapshotBridge` would still conform to every slice, so each new method costs the same; the split spreads the interface instead of concentrating it. `ProcessBridge`'s one-line methods are typed adapters at the JSON boundary and pass the deletion test.
- **Share one in-memory bridge between `FakeBridge` (223 lines) and `SnapshotBridge` (470 lines).** The first records calls and injects failures for tests; the second feeds seeded scenes to the snapshot pages. Different jobs.
- **Move `repairSelection` and the toast lifetime out of `AppStore` into plain types.** About 30 lines, already covered by `AppStoreTests` through the store.
- **Stop views taking the whole `AppStore` (19 views).** `@Observable` tracks reads per property, and the only recurring change is `now`, once a minute.
- **Split `Wire.swift` (462 lines).** Only decoding types, in the order of `wire.go`; the contract test above is the better guard.
- **`CaptureView.swift` (513 lines).** Under the 600-line signal.
