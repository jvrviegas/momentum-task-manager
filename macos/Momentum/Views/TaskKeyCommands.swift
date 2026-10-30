import SwiftUI

/// The TUI's single-key shortcuts. Attached to the sidebar and the task list
/// so they work whichever pane has focus, but never inside text fields.
struct TaskKeyCommands: ViewModifier {
    @Environment(AppStore.self) private var store
    let focusSearch: () -> Void

    func body(content: Content) -> some View {
        content.onKeyPress(phases: .down) { press in
            Self.handle(key: press.key, characters: press.characters, modifiers: press.modifiers, store: store, focusSearch: focusSearch)
        }
    }

    /// Handles one key press. It takes the press's parts rather than a
    /// KeyPress, which tests cannot make.
    @MainActor
    static func handle(
        key: KeyEquivalent, characters: String, modifiers: EventModifiers, store: AppStore, focusSearch: () -> Void
    ) -> KeyPress.Result {
        guard modifiers.subtracting([.shift, .numericPad]).isEmpty, !store.hasOverlay else { return .ignored }
        // The task an action key acts on. Completed tasks are read-only, and
        // the app says so instead of doing nothing, as the TUI does.
        func actionable() -> TaskItem? {
            guard store.view != .completed else {
                store.show("Completed tasks are read-only")
                return nil
            }
            return store.selectedTask?.isPending == true ? store.selectedTask : nil
        }
        switch key {
        case .space:
            if let task = actionable() { Task { await store.complete(task) } }
            return .handled
        case .return:
            if store.selectedTask != nil { store.showInspector.toggle() }
            return .handled
        case .escape:
            guard !store.query.isEmpty else { return .ignored }
            store.query = ""
            return .handled
        default:
            break
        }
        switch characters {
        case "j": move(by: 1, in: store)
        case "k": move(by: -1, in: store)
        case "g": select(store.visibleTasks.first, in: store)
        case "G": select(store.visibleTasks.last, in: store)
        case "1": store.view = .inbox
        case "2": store.view = .today
        case "3": store.view = .completed
        case "c": store.showCapture = true
        case "P":
            guard store.planningEnabled else { return .ignored }
            Task { await store.openPlan() }
        case "/": focusSearch()
        case "?": store.showShortcuts = true
        case "r": Task { await store.refresh(announce: true) }
        case "u": Task { await store.undo() }
        case "s": if let task = actionable() { Task { await store.toggleStart(task) } }
        case "d": if let task = actionable() { store.pendingDelete = task }
        case "a":
            guard actionable() != nil else { break }
            store.showInspector = true
            store.noteDraft.focusRequested = true
        case "x", "X":
            guard let task = actionable() else { break }
            if task.recurrenceTarget != nil {
                store.pendingStopRecurrence = task
            } else {
                store.show("This task doesn’t repeat")
            }
        default:
            guard let field = editKeys[characters] else { return .ignored }
            if let task = actionable() { store.editing = EditRequest(task: task, focus: field) }
        }
        return .handled
    }

    static let editKeys: [String: EditField] = [
        "e": .description, "E": .estimate, "R": .recurrence, "p": .project,
        "!": .priority, "D": .due, "S": .scheduled, "t": .tags,
    ]

    @MainActor
    private static func move(by delta: Int, in store: AppStore) {
        let tasks = store.visibleTasks
        guard !tasks.isEmpty else { return }
        let index = tasks.firstIndex { $0.uuid == store.selection } ?? -1
        select(tasks[max(0, min(tasks.count - 1, index + delta))], in: store)
    }

    @MainActor
    private static func select(_ task: TaskItem?, in store: AppStore) {
        if let task { store.selection = task.uuid }
    }
}

extension View {
    func taskKeyCommands(focusSearch: @escaping () -> Void) -> some View {
        modifier(TaskKeyCommands(focusSearch: focusSearch))
    }
}
