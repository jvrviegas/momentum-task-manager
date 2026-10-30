import Foundation

/// What waits on what among the loaded tasks. A pending dependency blocks a
/// task; a completed, deleted or unloaded one blocks nothing (task details
/// plan C11). The server refuses a dependency that would close a loop (C7),
/// and the picker never offers one.
struct TaskDependencies {
    /// The loaded tasks by UUID, and in the order the load gave them.
    let tasks: [String: TaskItem]
    let order: [TaskItem]

    /// The pending tasks task waits on; it is blocked while there is one.
    func blockers(of task: TaskItem) -> [TaskItem] {
        task.depends.compactMap { tasks[$0] }.filter(\.isPending)
    }

    /// The pending tasks that wait on task.
    func dependents(of task: TaskItem) -> [TaskItem] {
        order.filter { $0.isPending && $0.depends.contains(task.uuid) }
    }

    /// The tasks task may come to depend on: pending ones, other than task,
    /// than those it depends on already, and than those that wait on task,
    /// directly or through others.
    func candidates(for task: TaskItem) -> [TaskItem] {
        order.filter { candidate in
            candidate.isPending && candidate.uuid != task.uuid && !task.depends.contains(candidate.uuid)
                && !leads(from: candidate.uuid, to: task.uuid)
        }
    }

    /// Whether following dependencies from start reaches target.
    private func leads(from start: String, to target: String) -> Bool {
        var seen: Set<String> = []
        var queue = [start]
        while let next = queue.popLast() {
            if next == target { return true }
            guard seen.insert(next).inserted else { continue }
            queue += tasks[next]?.depends ?? []
        }
        return false
    }
}

/// The Add Dependency sheet (S-067): the task that will wait, the search, and
/// the candidate chosen with the pointer or ↑ ↓.
struct DependencyPicker: Identifiable {
    var task: TaskItem
    var query = ""
    /// The chosen candidate; the first match when nil or no longer matching.
    var selection: String?
    var id: String { task.uuid }

    /// The candidates the search matches, in order.
    func choices(in dependencies: TaskDependencies) -> [TaskItem] {
        let query = query.trimmingCharacters(in: .whitespaces)
        let candidates = dependencies.candidates(for: task)
        return query.isEmpty ? candidates : candidates.filter { TaskSearch.matches($0, query: query) }
    }

    /// The candidate Return adds.
    func chosen(in dependencies: TaskDependencies) -> TaskItem? {
        let choices = choices(in: dependencies)
        return choices.first { $0.uuid == selection } ?? choices.first
    }
}
