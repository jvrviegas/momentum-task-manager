import AppKit
import SwiftUI

/// Menu bar commands. Single-key shortcuts live on the task list; menu items
/// only use ⌘ shortcuts so they never steal keys from text fields.
struct MomentumCommands: Commands {
    let store: AppStore

    private var actionable: TaskItem? {
        guard !store.hasOverlay, let task = store.selectedTask, task.isPending else { return nil }
        return task
    }

    var body: some Commands {
        CommandGroup(replacing: .newItem) {
            Button("New Task") { store.showCapture = true }
                .keyboardShortcut("n")
                .disabled(store.status == nil || store.hasOverlay)
        }
        CommandGroup(replacing: .undoRedo) {
            Button("Undo") { undo() }
                .keyboardShortcut("z")
            Button("Redo") { NSApp.sendAction(Selector(("redo:")), to: nil, from: nil) }
                .keyboardShortcut("z", modifiers: [.command, .shift])
        }
        CommandGroup(before: .sidebar) {
            ForEach(Array(ViewName.allCases.enumerated()), id: \.element) { index, view in
                Button(view.title) { store.view = view }
                    .keyboardShortcut(KeyEquivalent(Character("\(index + 1)")))
            }
            Button("Refresh") { Task { await store.refresh(announce: true) } }
                .keyboardShortcut("r", modifiers: [.command, .option])
            Divider()
        }
        SidebarCommands()
        InspectorCommands()
        CommandMenu("Task") {
            Button("Complete") { if let task = actionable { Task { await store.complete(task) } } }
                .keyboardShortcut(.return)
                .disabled(actionable == nil)
            Button(actionable?.start == nil ? "Start" : "Stop") { if let task = actionable { Task { await store.toggleStart(task) } } }
                .keyboardShortcut("t")
                .disabled(actionable == nil)
            Button("Edit…") { if let task = actionable { store.editing = EditRequest(task: task, focus: .description) } }
                .keyboardShortcut("e")
                .disabled(actionable == nil)
            Button("Stop Repeating…") { store.pendingStopRecurrence = actionable }
                .disabled(actionable?.recurrenceTarget == nil)
            Button("Delete…") { store.pendingDelete = actionable }
                .disabled(actionable == nil)
            Divider()
            Button("Sync Now") { store.syncNow() }
                .keyboardShortcut("r")
                .disabled(store.status == nil)
        }
        CommandGroup(replacing: .help) {
            Button("Keyboard Shortcuts") { store.showShortcuts = true }
                .keyboardShortcut("/", modifiers: [.command, .shift])
        }
    }

    /// ⌘Z undoes typing inside a text field, and otherwise undoes the last
    /// Taskwarrior change while undo is still available.
    private func undo() {
        if let editor = NSApp.keyWindow?.firstResponder as? NSTextView, editor.isEditable {
            NSApp.sendAction(Selector(("undo:")), to: nil, from: nil)
        } else {
            Task { await store.undo() }
        }
    }
}
