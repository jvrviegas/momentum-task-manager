import SwiftUI

/// The TUI's single-key shortcuts. Attached to the sidebar and the task list
/// so they work whichever pane has focus, but never inside text fields.
struct TaskKeyCommands: ViewModifier {
    @Environment(AppStore.self) private var store
    let focusSearch: () -> Void

    func body(content: Content) -> some View {
        content.onKeyPress(phases: .down, action: handle)
    }

    private func handle(_ press: KeyPress) -> KeyPress.Result {
        guard press.modifiers.subtracting([.shift, .numericPad]).isEmpty, !store.hasOverlay else { return .ignored }
        let task = store.selectedTask
        let pending = task?.isPending == true ? task : nil
        switch press.key {
        case .space:
            if let pending { Task { await store.complete(pending) } }
            return .handled
        case .return:
            if task != nil { store.showInspector = true }
            return .handled
        case .escape:
            guard !store.query.isEmpty else { return .ignored }
            store.query = ""
            return .handled
        default:
            break
        }
        switch press.characters {
        case "j": move(by: 1)
        case "k": move(by: -1)
        case "g": select(store.visibleTasks.first)
        case "G": select(store.visibleTasks.last)
        case "1": store.view = .inbox
        case "2": store.view = .today
        case "3": store.view = .completed
        case "c": store.showCapture = true
        case "/": focusSearch()
        case "?": store.showShortcuts = true
        case "r": Task { await store.refresh(announce: true) }
        case "u": Task { await store.undo() }
        case "s": if let pending { Task { await store.toggleStart(pending) } }
        case "d": store.pendingDelete = pending
        case "x", "X":
            if pending?.recurrenceTarget != nil {
                store.pendingStopRecurrence = pending
            } else if pending != nil {
                store.show("This task doesn’t repeat")
            }
        default:
            guard let field = Self.editKeys[press.characters] else { return .ignored }
            if let pending { store.editing = EditRequest(task: pending, focus: field) }
        }
        return .handled
    }

    static let editKeys: [String: EditField] = [
        "e": .description, "E": .estimate, "R": .recurrence, "p": .project,
        "!": .priority, "D": .due, "S": .scheduled, "t": .tags,
    ]

    private func move(by delta: Int) {
        let tasks = store.visibleTasks
        guard !tasks.isEmpty else { return }
        let index = tasks.firstIndex { $0.uuid == store.selection } ?? -1
        select(tasks[max(0, min(tasks.count - 1, index + delta))])
    }

    private func select(_ task: TaskItem?) {
        if let task { store.selection = task.uuid }
    }
}

extension View {
    func taskKeyCommands(focusSearch: @escaping () -> Void) -> some View {
        modifier(TaskKeyCommands(focusSearch: focusSearch))
    }
}
