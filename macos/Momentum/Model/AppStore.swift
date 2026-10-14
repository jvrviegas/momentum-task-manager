import Foundation
import Observation

enum ViewName: String, CaseIterable, Identifiable, Hashable {
    case today, inbox, completed

    var id: Self { self }
    var title: String {
        switch self {
        case .today: "Today"
        case .inbox: "Inbox"
        case .completed: "Completed"
        }
    }
    var symbol: String {
        switch self {
        case .today: "sun.max"
        case .inbox: "tray"
        case .completed: "checkmark.circle"
        }
    }
}

struct ResolvedSection: Identifiable, Hashable {
    var title: String
    var project: String?
    var tasks: [TaskItem]
    var id: String { "\(title)|\(project ?? "")" }
}

struct Toast: Identifiable, Equatable {
    enum Kind { case info, success, error }
    let id = UUID()
    var message: String
    var kind: Kind = .info
    var offersUndo = false
    /// nil while the toast should stay, e.g. during the undo grace period.
    var expiresAt: Date?
}

enum EditField: Hashable, CaseIterable {
    case description, project, priority, due, scheduled, tags, estimate, recurrence
}

struct EditRequest: Identifiable, Equatable {
    var task: TaskItem
    var focus: EditField
    var id: String { task.uuid }
}

@MainActor
@Observable
final class AppStore {
    let bridge: any TaskBridge

    private(set) var status: BridgeStatus?
    private(set) var startupError: String?
    private(set) var snapshot: Snapshot?
    private(set) var loadError: String?
    private(set) var tasksByUUID: [String: TaskItem] = [:]
    var view: ViewName = .today {
        didSet { if view != oldValue { repairSelection() } }
    }
    var query = "" {
        didSet { if query != oldValue { repairSelection() } }
    }
    var selection: String?
    var showInspector = true
    var showCapture = false
    var showShortcuts = false
    var editing: EditRequest?
    var pendingDelete: TaskItem?
    var pendingStopRecurrence: TaskItem?
    private(set) var toast: Toast?
    private(set) var sync = SyncState.localOnly
    private(set) var lastSyncAt: Date?
    private(set) var syncError: String?
    private(set) var mutating = false
    /// Advances once a minute so relative dates stay current.
    private(set) var now = Date()

    @ObservationIgnored private var clock: Task<Void, Never>?
    @ObservationIgnored private var lastRefresh = Date.distantPast

    init(bridge: any TaskBridge) {
        self.bridge = bridge
    }

    // MARK: Derived state

    var visibleSections: [ResolvedSection] { sections(for: view) }
    var visibleTasks: [TaskItem] { visibleSections.flatMap(\.tasks) }
    var selectedTask: TaskItem? { selection.flatMap { tasksByUUID[$0] } }
    var isLocalOnly: Bool { !sync.enabled }
    var estimatesWritable: Bool { status?.estimatesWritable ?? true }
    var projects: [ProjectEntry] { snapshot?.projects ?? [] }
    var hasOverlay: Bool {
        showCapture || showShortcuts || editing != nil || pendingDelete != nil || pendingStopRecurrence != nil
    }
    var overdueCount: Int { snapshot?.today.first { $0.title == "Overdue" }?.uuids.count ?? 0 }
    var needsQuitPrompt: Bool { sync.enabled && sync.unsynced && sync.shutdown }

    func count(_ view: ViewName) -> Int {
        rawSections(view).reduce(0) { $0 + $1.uuids.count }
    }

    func sections(for view: ViewName) -> [ResolvedSection] {
        let query = query.trimmingCharacters(in: .whitespaces)
        return rawSections(view).compactMap { section in
            var tasks = section.uuids.compactMap { tasksByUUID[$0] }
            if !query.isEmpty {
                tasks = tasks.filter { TaskSearch.matches($0, query: query) }
            }
            return tasks.isEmpty ? nil : ResolvedSection(title: section.title, project: section.project, tasks: tasks)
        }
    }

    private func rawSections(_ view: ViewName) -> [TaskSection] {
        guard let snapshot else { return [] }
        switch view {
        case .today: return snapshot.today
        case .inbox: return snapshot.inbox
        case .completed: return snapshot.completed
        }
    }

    // MARK: Lifecycle

    func start() async {
        startupError = nil
        do {
            let status = try await bridge.status()
            self.status = status
            sync = SyncState(settings: status.sync, configured: status.syncConfigured, now: .now)
        } catch {
            startupError = error.localizedDescription
            return
        }
        await refresh()
        // Open Today when it has work, otherwise Inbox (VIEW-08).
        if count(.today) == 0 {
            view = .inbox
        }
        perform(sync.apply(.startup, at: .now))
        startClock()
    }

    func refresh(announce: Bool = false) async {
        lastRefresh = .now
        do {
            let snapshot = try await bridge.load()
            let previousOrder = visibleTasks.map(\.uuid)
            self.snapshot = snapshot
            tasksByUUID = Dictionary(snapshot.tasks.map { ($0.uuid, $0) }, uniquingKeysWith: { first, _ in first })
            loadError = nil
            repairSelection(previousOrder: previousOrder)
            if announce {
                show("Refreshed from Taskwarrior", .success)
            }
        } catch {
            if snapshot == nil {
                loadError = error.localizedDescription
            } else {
                show(error.localizedDescription, .error)
            }
        }
    }

    /// Keeps the selected task when it is still visible (VIEW-10); otherwise
    /// selects the task now at its old position, which is the next task or,
    /// at the end of the list, the previous one (MUT-10).
    private func repairSelection(previousOrder: [String] = []) {
        let visible = visibleTasks
        if let selection, visible.contains(where: { $0.uuid == selection }) {
            return
        }
        guard !visible.isEmpty else {
            selection = nil
            return
        }
        if let selection, let index = previousOrder.firstIndex(of: selection) {
            self.selection = visible[min(index, visible.count - 1)].uuid
        } else {
            selection = visible[0].uuid
        }
    }

    func tick(at date: Date) {
        if !Calendar.current.isDate(date, equalTo: now, toGranularity: .minute) {
            now = date
        }
        if let next = sync.nextAt, next <= date {
            perform(sync.apply(.timer, at: date))
        }
        if let toast {
            if let expiry = toast.expiresAt, expiry <= date {
                self.toast = nil
            } else if toast.offersUndo, toast.expiresAt == nil, sync.phase != .grace {
                self.toast?.expiresAt = date.addingTimeInterval(3)
            }
        }
        let interval = TimeInterval(status?.refreshIntervalSeconds ?? 0)
        if interval > 0, date.timeIntervalSince(lastRefresh) >= interval, !mutating, !hasOverlay {
            Task { await refresh() }
        }
    }

    private func startClock() {
        guard clock == nil else { return }
        clock = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(1))
                self?.tick(at: .now)
            }
        }
    }

    // MARK: Mutations

    /// Runs one Taskwarrior mutation at a time (MUT-05). Errors reach the caller.
    func exclusive<T: Sendable>(_ operation: () async throws -> T) async throws -> T {
        guard !mutating else {
            throw BridgeError(message: "Another change is still being saved")
        }
        mutating = true
        defer { mutating = false }
        return try await operation()
    }

    /// Reloads after a successful mutation and opens the undo window.
    func didMutate(_ message: String) async {
        await refresh()
        perform(sync.apply(.mutation, at: .now))
        show(message, offersUndo: true)
    }

    private func mutate(_ message: String, _ operation: () async throws -> Void) async {
        do {
            try await exclusive(operation)
        } catch {
            show(error.localizedDescription, .error)
            return
        }
        await didMutate(message)
    }

    func complete(_ task: TaskItem) async {
        guard task.isPending else { return }
        await mutate("Completed “\(task.description)”") { try await bridge.perform(.complete, uuid: task.uuid) }
    }

    func toggleStart(_ task: TaskItem) async {
        guard task.isPending else { return }
        let starting = task.start == nil
        await mutate("\(starting ? "Started" : "Stopped") “\(task.description)”") {
            try await bridge.perform(starting ? .start : .stop, uuid: task.uuid)
        }
    }

    func delete(_ task: TaskItem) async {
        await mutate("Deleted “\(task.description)”") { try await bridge.perform(.delete, uuid: task.uuid) }
    }

    func stopRecurrence(_ task: TaskItem) async {
        guard let target = task.recurrenceTarget else { return }
        await mutate("Stopped repeating “\(task.description)”") { try await bridge.perform(.stopRecurrence, uuid: target) }
    }

    /// Saves an edit; field errors are thrown so the editor can show them.
    func save(_ task: TaskItem, edits: EditFields) async throws {
        let changed = try await exclusive { try await bridge.modify(uuid: task.uuid, before: task.edit, after: edits) }
        if changed {
            await didMutate("Updated “\(edits.description.trimmingCharacters(in: .whitespaces))”")
        }
    }

    func undo() async {
        guard sync.undoAvailable else {
            show(isLocalOnly ? "Nothing to undo" : "Undo is only available until changes sync")
            return
        }
        do {
            try await exclusive { try await bridge.undo() }
        } catch {
            show(error.localizedDescription, .error)
            return
        }
        let effect = sync.apply(.undo, at: .now)
        await refresh()
        show(effect.message.isEmpty ? "Last change undone" : effect.message, .success)
    }

    // MARK: Sync

    func syncNow() {
        guard sync.enabled else {
            show("Local only — Taskwarrior sync is not configured")
            return
        }
        perform(sync.apply(.manual, at: .now))
    }

    private func perform(_ effect: SyncState.Effect) {
        switch effect.action {
        case .run: Task { await runSync() }
        case .refresh: Task { await refresh() }
        default: break
        }
    }

    private func runSync() async {
        do {
            try await bridge.sync()
            lastSyncAt = .now
            syncError = nil
            perform(sync.apply(.succeeded, at: .now))
        } catch {
            syncError = error.localizedDescription
            perform(sync.apply(.failed, at: .now))
        }
    }

    /// Syncs before quitting. Returns the failure message when sync failed.
    func syncBeforeQuit() async -> String? {
        _ = sync.apply(.quitRequested, at: .now)
        _ = sync.apply(.syncAndQuit, at: .now)
        do {
            try await bridge.sync()
            _ = sync.apply(.succeeded, at: .now)
            return nil
        } catch {
            // Like the TUI, a failed sync stays in retrying; the quit prompt that
            // follows decides whether to quit anyway.
            syncError = error.localizedDescription
            _ = sync.apply(.failed, at: .now)
            sync.quitPending = false
            return error.localizedDescription
        }
    }

    // MARK: Messages

    func show(_ message: String, _ kind: Toast.Kind = .info, offersUndo: Bool = false) {
        let lifetime: TimeInterval = kind == .error ? 7 : offersUndo ? 8 : 3.2
        let stays = offersUndo && sync.phase == .grace
        toast = Toast(message: message, kind: kind, offersUndo: offersUndo, expiresAt: stays ? nil : .now.addingTimeInterval(lifetime))
    }

    func dismissToast() {
        toast = nil
    }
}
