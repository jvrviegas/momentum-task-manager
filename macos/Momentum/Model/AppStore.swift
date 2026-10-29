import Foundation
import Observation

enum ViewName: String, CaseIterable, Identifiable, Hashable {
    case inbox, today, completed

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

/// What the app last learned about its server, as in the TUI.
enum ServerState: Equatable {
    case unknown, online, unreachable, signedOut
}

/// A device sign-in in progress, which the sign-in sheet shows.
struct SignInFlow: Identifiable, Equatable {
    enum Phase: Equatable {
        case starting
        case waiting(DeviceSignIn)
        case failed(String)
    }

    var server: String
    var phase = Phase.starting
    var id: String { server }
}

@MainActor
@Observable
final class AppStore {
    let bridge: any TaskBridge
    let signIns: any SignInStore
    /// The Planning and Calendar tabs of Settings.
    let settings = SettingsModel()
    /// The Calendar tab's macOS Calendar source, kept on this Mac.
    let macCalendar: MacCalendarModel
    /// The Projects tab: the catalog and the values found only on tasks.
    let catalog = ProjectsModel()
    /// The Diagnostics tab: the checks and the report to copy.
    let diagnostics = DiagnosticsModel()

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
    /// The Plan Today sheet; nil while it is closed.
    var planSheet: PlanSheet?
    private(set) var toast: Toast?
    /// How long the latest change can be undone (backend plan D7).
    private(set) var undoWindow = UndoWindow(delay: 0)
    private(set) var mutating = false
    /// Advances once a minute so relative dates stay current.
    private(set) var now = Date()
    private(set) var server = ServerState.unknown
    /// This Mac's sign-in to the server; nil when signed out.
    private(set) var account: SavedSignIn?
    private(set) var signIn: SignInFlow?

    @ObservationIgnored private var clock: Task<Void, Never>?
    @ObservationIgnored private var lastRefresh = Date.distantPast
    @ObservationIgnored private var signInTask: Task<Void, Never>?
    private let sleep: @Sendable (TimeInterval) async throws -> Void

    init(
        bridge: any TaskBridge, signIns: any SignInStore = KeychainSignInStore(),
        calendarSource: any CalendarEventSource = NoCalendarSource(), calendarDefaults: UserDefaults = .standard,
        sleep: @escaping @Sendable (TimeInterval) async throws -> Void = { try await Task.sleep(for: .seconds($0)) }
    ) {
        self.bridge = bridge
        self.signIns = signIns
        self.sleep = sleep
        macCalendar = MacCalendarModel(eventSource: calendarSource, defaults: calendarDefaults)
        settings.store = self
        macCalendar.store = self
        catalog.store = self
        diagnostics.store = self
    }

    // MARK: Derived state

    var visibleSections: [ResolvedSection] { sections(for: view) }
    var visibleTasks: [TaskItem] { visibleSections.flatMap(\.tasks) }
    var selectedTask: TaskItem? { selection.flatMap { tasksByUUID[$0] } }
    var projects: [ProjectEntry] { snapshot?.projects ?? [] }
    var hasOverlay: Bool {
        showCapture || showShortcuts || editing != nil || pendingDelete != nil || pendingStopRecurrence != nil || signIn != nil
            || planSheet != nil
    }
    var overdueCount: Int { snapshot?.today.first { $0.title == "Overdue" }?.uuids.count ?? 0 }
    /// Whether Plan Today is offered: [planning] enabled, as the TUI's
    /// default is, until a load says otherwise.
    var planningEnabled: Bool { snapshot?.plan?.enabled ?? true }
    /// The [backend] server; nil until config.toml names one.
    var backendURL: String? {
        status?.backendUrl.map { $0.hasSuffix("/") ? String($0.dropLast()) : $0 }
    }
    /// Why config.toml's newest version was not applied; nil while it was.
    var configProblem: String? { status?.configError }
    /// Whether a changed [backend] url waits for a restart.
    var restartRequired: Bool { status?.restartRequired ?? false }
    /// When the latest change's undo window closes; nil while it is closed.
    var undoClosesAt: Date? {
        undoWindow.isAvailable(at: .now) ? undoWindow.until : nil
    }

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
            undoWindow = UndoWindow(delay: TimeInterval(status.mutationDelaySeconds))
        } catch {
            startupError = error.localizedDescription
            return
        }
        // Without a server, the window shows how to add one.
        guard let url = backendURL else { return }
        guard let saved = savedSignIn(url) else {
            server = .signedOut
            startClock()
            return
        }
        do {
            try await bridge.setToken(saved.token)
        } catch {
            startupError = error.localizedDescription
            return
        }
        account = saved
        await open()
    }

    /// Loads tasks. The first load opens Today when it has work, otherwise
    /// Inbox (VIEW-08).
    private func open() async {
        let first = snapshot == nil
        await refresh()
        if first && count(.today) == 0 {
            view = .inbox
        }
        startClock()
    }

    func refresh(announce: Bool = false) async {
        lastRefresh = .now
        do {
            let snapshot = try await bridge.load(calendar: macCalendar.input(titles: false))
            noteServer(nil)
            let previousOrder = visibleTasks.map(\.uuid)
            self.snapshot = snapshot
            tasksByUUID = Dictionary(snapshot.tasks.map { ($0.uuid, $0) }, uniquingKeysWith: { first, _ in first })
            loadError = nil
            repairSelection(previousOrder: previousOrder)
            await refreshStatus()
            if announce {
                show("Refreshed from the server", .success)
            }
        } catch {
            noteServer(error)
            if snapshot == nil {
                loadError = error.localizedDescription
            } else {
                show(error.localizedDescription, .error)
            }
        }
    }

    /// Asks the bridge for its settings. It re-reads config.toml when the file
    /// changed, so an edit made elsewhere applies without a restart: the
    /// refresh interval and the undo delay follow it here, and the planning
    /// and calendar settings apply to the next load.
    func refreshStatus() async {
        guard let status = try? await bridge.status() else { return }
        self.status = status
        undoWindow.delay = max(0, TimeInterval(status.mutationDelaySeconds))
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
        if let toast {
            if let expiry = toast.expiresAt, expiry <= date {
                self.toast = nil
            } else if toast.offersUndo, toast.expiresAt == nil, !undoWindow.isAvailable(at: date) {
                self.toast?.expiresAt = date.addingTimeInterval(3)
            }
        }
        let interval = TimeInterval(status?.refreshIntervalSeconds ?? 0)
        if interval > 0, date.timeIntervalSince(lastRefresh) >= interval, !mutating, !hasOverlay, account != nil {
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

    /// Runs one change at a time (MUT-05). Errors reach the caller.
    func exclusive<T: Sendable>(_ operation: () async throws -> T) async throws -> T {
        guard !mutating else {
            throw BridgeError(message: "Another change is still being saved")
        }
        mutating = true
        defer { mutating = false }
        do {
            return try await operation()
        } catch {
            noteServer(error) // a success reloads, which notes it
            throw error
        }
    }

    /// Reloads after a successful mutation and opens the undo window.
    func didMutate(_ message: String) async {
        await refresh()
        undoWindow.open(at: .now)
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
        guard undoWindow.isAvailable(at: .now) else {
            if undoWindow.delay > 0 {
                show("Undo is only available for \(Duration.seconds(undoWindow.delay).formatted(.units(width: .wide))) after a change")
            } else {
                show("Nothing to undo")
            }
            return
        }
        do {
            try await exclusive { try await bridge.undo() }
        } catch {
            show(error.localizedDescription, .error)
            return
        }
        undoWindow.close()
        await refresh()
        show("Last change undone", .success)
    }

    // MARK: Plan Today

    /// Opens Plan Today with the plan the bridge builds now.
    func openPlan() async {
        guard planningEnabled, account != nil, !hasOverlay else { return }
        do {
            let plan = try await bridge.plan(calendar: macCalendar.input(titles: true))
            noteServer(nil)
            planSheet = PlanSheet(plan: plan)
        } catch {
            noteServer(error)
            show(error.localizedDescription, .error)
        }
    }

    /// Saves the plan's tags. A saved plan offers no undo and closes the
    /// undo window, which would revert only one task's tag (D2). When a change
    /// fails, the sheet stays open with its ticks and confirming again
    /// finishes, since the bridge applies only what is left (D4).
    func confirmPlan() async {
        guard let sheet = planSheet, !sheet.saving else { return }
        planSheet?.saving = true
        planSheet?.notice = nil
        let result: PlanSaveResult
        do {
            result = try await exclusive { try await bridge.applyPlan(date: sheet.plan.date, uuids: sheet.selectedUUIDs) }
        } catch {
            planSheet?.saving = false
            planSheet?.notice = PlanSheet.Notice(message: error.localizedDescription)
            return
        }
        guard result.total > 0 else {
            planSheet = nil
            show("Daily plan unchanged")
            return
        }
        undoWindow.close()
        if let error = result.error {
            await refresh()
            planSheet?.saving = false
            planSheet?.notice = PlanSheet.Notice(
                message: "Saved \(result.applied) of \(result.total) changes. Confirm again to finish — nothing is applied twice.",
                reason: error)
            return
        }
        planSheet = nil
        await refresh()
        let count = sheet.chosen.count
        show("Daily plan saved · \(count) \(count == 1 ? "task" : "tasks")", .success)
    }

    func cancelPlan() {
        planSheet = nil
    }

    // MARK: Messages

    /// A project migration moves many tasks, which one undo cannot take back.
    func closeUndoWindow() {
        undoWindow.close()
    }

    func show(_ message: String, _ kind: Toast.Kind = .info, offersUndo: Bool = false) {
        let lifetime: TimeInterval = kind == .error ? 7 : offersUndo ? 8 : 3.2
        let stays = offersUndo && undoClosesAt != nil
        toast = Toast(message: message, kind: kind, offersUndo: offersUndo, expiresAt: stays ? nil : .now.addingTimeInterval(lifetime))
    }

    func dismissToast() {
        toast = nil
    }

    // MARK: Sign-in

    /// Records what a request's outcome says about the server. Other failures
    /// leave it alone: the bridge refuses some requests itself.
    private func noteServer(_ error: (any Error)?) {
        guard let error else {
            server = .online
            return
        }
        switch (error as? BridgeError)?.code {
        case "unreachable": server = .unreachable
        case "unauthorized": server = .signedOut
        default: break
        }
    }

    /// The Keychain's sign-in to url, unless it has expired.
    private func savedSignIn(_ url: String) -> SavedSignIn? {
        do {
            guard let saved = try signIns.read(server: url), saved.expiresAt > .now else { return nil }
            return saved
        } catch {
            show("Couldn’t read the sign-in from the Keychain: \(error.localizedDescription)", .error)
            return nil
        }
    }

    /// Starts the device sign-in (D1) that the sign-in sheet shows, replacing
    /// any in progress.
    @discardableResult
    func beginSignIn() -> Task<Void, Never> {
        signInTask?.cancel()
        guard let url = backendURL else { return Task {} }
        signIn = SignInFlow(server: url)
        let task = Task { await runSignIn(url) }
        signInTask = task
        return task
    }

    func cancelSignIn() {
        signInTask?.cancel()
        signInTask = nil
        signIn = nil
    }

    /// Polls until the person finishes in the browser, as `momentum login`
    /// does: every interval, five seconds longer after each slowDown.
    private func runSignIn(_ url: String) async {
        do {
            let start = try await bridge.startSignIn()
            try Task.checkCancellation()
            signIn?.phase = .waiting(start)
            var interval = TimeInterval(start.intervalSeconds > 0 ? start.intervalSeconds : 5)
            while true {
                try await sleep(interval)
                let poll = try await bridge.pollSignIn(deviceCode: start.deviceCode)
                try Task.checkCancellation()
                switch poll.state {
                case "signedIn":
                    await finishSignIn(url, poll)
                    return
                case "slowDown":
                    interval += 5
                default:
                    continue
                }
            }
        } catch {
            guard !Task.isCancelled else { return }
            signIn?.phase = .failed(error.localizedDescription)
        }
    }

    /// Saves the new sign-in, signs the bridge in, and revokes the sign-in it
    /// replaces, so that token stops working.
    private func finishSignIn(_ url: String, _ poll: SignInPoll) async {
        guard let token = poll.token, let expiresAt = poll.expiresAt else {
            signIn?.phase = .failed("The server answered without a sign-in.")
            return
        }
        let saved = SavedSignIn(server: url, token: token, email: poll.email ?? "", expiresAt: expiresAt)
        let previous = try? signIns.read(server: url)
        do {
            try signIns.save(saved)
        } catch {
            try? await bridge.revoke(token: token) // the server shows a token only once
            signIn?.phase = .failed("Couldn’t save the sign-in in the Keychain: \(error.localizedDescription)")
            return
        }
        do {
            try await bridge.setToken(token)
        } catch {
            signIn?.phase = .failed(error.localizedDescription) // the saved sign-in is used at the next launch
            return
        }
        account = saved
        signIn = nil
        signInTask = nil
        show("Signed in as \(saved.email)", .success)
        if let previous, previous.token != token {
            do {
                try await bridge.revoke(token: previous.token)
            } catch {
                show("Couldn’t revoke the previous sign-in, which works until \(previous.expiresAt.formatted(date: .abbreviated, time: .omitted)): \(error.localizedDescription)", .error)
            }
        }
        await open()
    }

    /// Signs this Mac out of the backend server. The sign-in leaves the
    /// Keychain even when the server cannot revoke it.
    func signOut() async {
        guard let url = backendURL else { return }
        cancelSignIn()
        var failure: String?
        let saved = account ?? (try? signIns.read(server: url))
        if let saved {
            do {
                try await bridge.revoke(token: saved.token)
            } catch {
                failure = error.localizedDescription
            }
        }
        do {
            try signIns.remove(server: url)
        } catch {
            show("Couldn’t remove the sign-in from the Keychain: \(error.localizedDescription)", .error)
            return
        }
        try? await bridge.setToken("")
        account = nil
        server = .signedOut
        snapshot = nil
        tasksByUUID = [:]
        selection = nil
        loadError = nil
        undoWindow.close()
        if let failure, let saved {
            show("Signed out. The server couldn’t revoke the sign-in, so its token works until \(saved.expiresAt.formatted(date: .abbreviated, time: .omitted)): \(failure)", .error)
        } else {
            show("Signed out")
        }
    }
}
