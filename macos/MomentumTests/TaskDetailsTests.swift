import Foundation
import Testing
@testable import Momentum

/// Notes, dependencies and copies (M9, task details plan).
@MainActor
struct TaskDetailsTests {
    private func started(_ snapshots: Snapshot...) async -> (AppStore, FakeBridge) {
        let bridge = FakeBridge(snapshots: snapshots)
        let store = signedInStore(bridge)
        await store.start()
        return (store, bridge)
    }

    private func task(_ uuid: String, depends: [String] = [], status: String = "pending") -> TaskItem {
        var task = TaskItem.fixture(uuid, status: status)
        task.depends = depends
        return task
    }

    @Test func addingANoteLoadsAgainAndOffersUndo() async throws {
        let a = TaskItem.fixture("a")
        let (store, bridge) = await started(snapshot(inbox: [a]))
        try await store.addNote("Asked Ana for the quote", to: a)
        #expect(await bridge.calls.suffix(2) == ["addNote a Asked Ana for the quote", "load"])
        #expect(store.undoClosesAt != nil && store.toast?.offersUndo == true)
        #expect(store.toast?.message == "Added a note to “a”")
    }

    @Test func aRefusedNoteIsThrownAndChangesNothing() async {
        let a = TaskItem.fixture("a")
        let (store, bridge) = await started(snapshot(inbox: [a]))
        let refusal = BridgeError(message: "a note cannot be empty", field: "note")
        await bridge.setFailure(refusal)
        await #expect(throws: refusal) { try await store.addNote(" ", to: a) }
        #expect(await bridge.calls.last == "addNote a  ")
        #expect(store.undoClosesAt == nil && store.toast == nil)
    }

    @Test func theDraftIsSentTrimmedAndEmptiesOnceAdded() async {
        let a = TaskItem.fixture("a")
        let (store, bridge) = await started(snapshot(inbox: [a]))
        store.noteDraft.write("  Asked Ana  ", for: a)
        await store.noteDraft.add(to: a, in: store)
        #expect(await bridge.calls.suffix(2) == ["addNote a Asked Ana", "load"])
        #expect(store.noteDraft.text(for: a).isEmpty && store.noteDraft.error(for: a) == nil)
    }

    @Test func aRefusedDraftKeepsItsWordsAndSaysWhy() async {
        let a = TaskItem.fixture("a")
        let (store, bridge) = await started(snapshot(inbox: [a]))
        await bridge.setFailure(BridgeError(message: "cannot reach the Momentum server: connection refused", code: "unreachable"))
        store.noteDraft.write("Asked Ana", for: a)
        await store.noteDraft.add(to: a, in: store)
        #expect(store.noteDraft.text(for: a) == "Asked Ana")
        #expect(store.noteDraft.error(for: a) == "cannot reach the Momentum server: connection refused")
        // Typing again clears the reason.
        store.noteDraft.write("Asked Ana again", for: a)
        #expect(store.noteDraft.error(for: a) == nil)
    }

    @Test func aDraftBelongsToOneTaskAndNeedsWords() async {
        let a = TaskItem.fixture("a"), b = TaskItem.fixture("b")
        let (store, bridge) = await started(snapshot(inbox: [a, b]))
        store.noteDraft.write("For a", for: a)
        #expect(store.noteDraft.text(for: b).isEmpty && !store.noteDraft.canAdd(to: b))
        store.noteDraft.write(" \n ", for: a)
        #expect(!store.noteDraft.canAdd(to: a))
        await store.noteDraft.add(to: a, in: store)
        #expect(await !bridge.calls.contains { $0.hasPrefix("addNote") })
    }

    @Test func aMovesTheFocusToTheNoteFieldOfAPendingTask() async {
        let a = TaskItem.fixture("a")
        let (store, _) = await started(snapshot(inbox: [a]))
        store.showInspector = false
        _ = TaskKeyCommands.handle(key: "a", characters: "a", modifiers: [], store: store, focusSearch: {})
        #expect(store.showInspector && store.noteDraft.focusRequested)

        store.noteDraft.focusRequested = false
        store.view = .completed
        _ = TaskKeyCommands.handle(key: "a", characters: "a", modifiers: [], store: store, focusSearch: {})
        #expect(!store.noteDraft.focusRequested && store.toast?.message == "Completed tasks are read-only")
    }

    @Test func removingANoteSendsItsWordsAndOffersUndo() async {
        var a = TaskItem.fixture("a")
        let note = Annotation(description: "Asked Ana", entry: Date(timeIntervalSince1970: 1_792_000_000))
        a.annotations = [note]
        let (store, bridge) = await started(snapshot(inbox: [a]))
        await store.removeNote(note, from: a)
        #expect(await bridge.calls.suffix(2) == ["removeNote a Asked Ana", "load"])
        #expect(store.undoClosesAt != nil && store.toast?.message == "Removed a note from “a”")
    }

    @Test func addingAndRemovingADependencyOfferUndo() async {
        let a = TaskItem.fixture("a"), b = TaskItem.fixture("b")
        let (store, bridge) = await started(snapshot(inbox: [a, b]))
        await store.addDependency(a, on: b)
        #expect(await bridge.calls.suffix(2) == ["addDependency a b", "load"])
        #expect(store.undoClosesAt != nil && store.toast?.message == "“a” now depends on “b”")
        // A dependency the load no longer has is removed by its UUID.
        await store.removeDependency(a, on: "gone")
        #expect(await bridge.calls.suffix(2) == ["removeDependency a gone", "load"])
        #expect(store.toast?.offersUndo == true)
    }

    @Test func aRefusedDependencyShowsWhy() async {
        let a = TaskItem.fixture("a"), b = TaskItem.fixture("b")
        let (store, bridge) = await started(snapshot(inbox: [a, b]))
        await bridge.setFailure(BridgeError(message: "a task cannot depend on a task that depends on it", field: "depends"))
        await store.addDependency(a, on: b)
        #expect(store.toast?.kind == .error && store.toast?.message == "a task cannot depend on a task that depends on it")
        #expect(store.undoClosesAt == nil)
    }

    @Test func thePickerOffersPendingTasksThatCloseNoLoop() {
        // a depends on b; c on a; f on c, so f waits on a through c.
        let a = task("a", depends: ["b"])
        let tasks = [a, task("b"), task("c", depends: ["a"]), task("d", status: "completed"), task("e"), task("f", depends: ["c"])]
        let dependencies = TaskDependencies(tasks: Dictionary(uniqueKeysWithValues: tasks.map { ($0.uuid, $0) }), order: tasks)
        #expect(dependencies.candidates(for: a).map(\.uuid) == ["e"])
        #expect(dependencies.candidates(for: tasks[4]).map(\.uuid) == ["a", "b", "c", "f"])
    }

    @Test func theSheetNarrowsTheCandidatesAndChoosesTheFirstMatch() {
        let a = TaskItem.fixture("Send the invoice", uuid: "a")
        let tasks = [a, TaskItem.fixture("Get the quote", uuid: "q"), TaskItem.fixture("Read the contract", uuid: "r")]
        let dependencies = TaskDependencies(tasks: Dictionary(uniqueKeysWithValues: tasks.map { ($0.uuid, $0) }), order: tasks)
        var picker = DependencyPicker(task: a)
        #expect(picker.choices(in: dependencies).map(\.uuid) == ["q", "r"] && picker.chosen(in: dependencies)?.uuid == "q")
        picker.selection = "r"
        #expect(picker.chosen(in: dependencies)?.uuid == "r")
        // A choice the search hides gives way to the first match.
        picker.query = "quote"
        #expect(picker.choices(in: dependencies).map(\.uuid) == ["q"] && picker.chosen(in: dependencies)?.uuid == "q")
        picker.query = "nothing like it"
        #expect(picker.chosen(in: dependencies) == nil)
    }

    @Test func addingTheChosenDependencyClosesTheSheet() async {
        let a = TaskItem.fixture("a"), b = TaskItem.fixture("b")
        let (store, bridge) = await started(snapshot(inbox: [a, b]))
        store.dependencyPicker = DependencyPicker(task: a)
        #expect(store.hasOverlay)
        await store.addChosenDependency()
        #expect(store.dependencyPicker == nil && !store.hasOverlay)
        #expect(await bridge.calls.suffix(2) == ["addDependency a b", "load"])
    }

    @Test func aTaskIsBlockedWhileADependencyIsPending() {
        let a = task("a", depends: ["b", "d", "gone"])
        let tasks = [a, task("b"), task("d", status: "completed"), task("e", depends: ["b"])]
        let dependencies = TaskDependencies(tasks: Dictionary(uniqueKeysWithValues: tasks.map { ($0.uuid, $0) }), order: tasks)
        #expect(dependencies.blockers(of: a).map(\.uuid) == ["b"])
        #expect(dependencies.blockers(of: tasks[1]).isEmpty)
        #expect(dependencies.dependents(of: tasks[1]).map(\.uuid) == ["a", "e"])
    }

    @Test func duplicatingSelectsTheCopyAndUndoRemovesIt() async {
        let a = TaskItem.fixture("a"), copy = TaskItem.fixture("Copy of a", uuid: "copy")
        let (store, bridge) = await started(snapshot(inbox: [a]), snapshot(inbox: [a, copy]), snapshot(inbox: [a]))
        await store.duplicate(a)
        #expect(await bridge.calls.suffix(2) == ["duplicate a", "load"])
        #expect(store.selection == "copy" && store.toast?.message == "Duplicated “a”" && store.toast?.offersUndo == true)
        await store.undo()
        #expect(await bridge.calls.suffix(2) == ["undo", "load"])
        #expect(store.selection == "a")
    }

    @Test func aCopyTheServerRefusesShowsWhy() async {
        let a = TaskItem.fixture("a")
        let (store, bridge) = await started(snapshot(inbox: [a]))
        await bridge.setFailure(BridgeError(message: "the task is no longer pending"))
        await store.duplicate(a)
        #expect(store.toast?.kind == .error && store.selection == "a" && store.undoClosesAt == nil)
    }

    @Test func oneChangeAtATime() async {
        let a = TaskItem.fixture("a")
        let (store, bridge) = await started(snapshot(inbox: [a]))
        await #expect(throws: BridgeError(message: "Another change is still being saved")) {
            try await store.exclusive { try await store.addNote("Later", to: a) }
        }
        #expect(await !bridge.calls.contains { $0.hasPrefix("addNote") })
    }
}
