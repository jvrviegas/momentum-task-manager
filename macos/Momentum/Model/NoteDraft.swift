import Foundation
import Observation

/// The note being written in the inspector's Notes field (S-065), for one
/// task at a time: choosing another task starts a new draft. It lives outside
/// the view, so `a` can move the focus to the field and a snapshot page can
/// show a note typed.
@MainActor
@Observable
final class NoteDraft {
    /// The task the words are for.
    private(set) var uuid: String?
    private(set) var text = ""
    /// Why the server refused the words; nil until it does.
    private(set) var error: String?
    private(set) var saving = false
    /// Asks the field to take the focus, as `a` and Task ▸ Add Note do.
    var focusRequested = false

    func text(for task: TaskItem) -> String { uuid == task.uuid ? text : "" }
    func error(for task: TaskItem) -> String? { uuid == task.uuid ? error : nil }

    func write(_ text: String, for task: TaskItem) {
        uuid = task.uuid
        self.text = text
        error = nil
    }

    /// Whether the draft for task has more than spaces, so Add can send it.
    func canAdd(to task: TaskItem) -> Bool {
        !saving && !text(for: task).trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    /// Adds the draft to task. Added, the draft empties; refused, it keeps its
    /// words and says why.
    func add(to task: TaskItem, in store: AppStore) async {
        guard canAdd(to: task) else { return }
        saving = true
        defer { saving = false }
        do {
            try await store.addNote(text.trimmingCharacters(in: .whitespacesAndNewlines), to: task)
            if uuid == task.uuid { text = "" }
        } catch {
            if uuid == task.uuid { self.error = error.localizedDescription }
        }
    }
}
