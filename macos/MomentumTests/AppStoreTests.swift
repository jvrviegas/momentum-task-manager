import Foundation
import SwiftUI
import Testing
@testable import Momentum

/// An in-memory bridge: `snapshots` are returned by successive loads.
actor FakeBridge: TaskBridge {
    var statusValue: BridgeStatus
    var snapshots: [Snapshot]
    private(set) var calls: [String] = []
    /// What each load and plan was handed in place of the calendar files.
    private(set) var loadCalendars: [CalendarInput?] = []
    private(set) var planCalendars: [CalendarInput?] = []
    var failNext: BridgeError?
    var failLoad: BridgeError?
    var polls: [Result<SignInPoll, BridgeError>] = []
    var planValue = DailyPlan(
        date: "2026-10-14", tag: "momentum-plan-2026-10-14", capacityMinutes: 480, bufferMinutes: 60,
        obligations: [], rollover: [], candidates: [])
    /// Answers to successive applyPlan calls.
    var planResults: [PlanSaveResult] = []

    init(backendURL: String? = "https://momentum.example", mutationDelay: Int = 15, snapshots: [Snapshot]) {
        statusValue = BridgeStatus(
            configPath: "/tmp/config.toml", mutationDelaySeconds: mutationDelay, refreshIntervalSeconds: 0, backendUrl: backendURL)
        self.snapshots = snapshots
    }

    func setStatus(_ status: BridgeStatus) { statusValue = status }
    func setFailure(_ error: BridgeError?) { failNext = error }
    func setLoadFailure(_ error: BridgeError?) { failLoad = error }
    func setPolls(_ polls: [Result<SignInPoll, BridgeError>]) { self.polls = polls }
    func setPlan(_ plan: DailyPlan, results: [PlanSaveResult]) {
        planValue = plan
        planResults = results
    }

    private func record(_ call: String) throws {
        calls.append(call)
        if let error = failNext {
            failNext = nil
            throw error
        }
    }

    func status() async throws -> BridgeStatus { statusValue }
    func load(calendar: CalendarInput?) async throws -> Snapshot {
        calls.append("load")
        loadCalendars.append(calendar)
        if let error = failLoad {
            failLoad = nil
            throw error
        }
        return snapshots.count > 1 ? snapshots.removeFirst() : snapshots[0]
    }
    func interpret(_ input: String, literals: [TextSpan]) async throws -> Interpretation {
        Interpretation(token: nil, source: input, valid: true, requiresReview: false, description: input, candidates: [], diagnostics: [], reviewValues: [])
    }
    func suggest(_ input: String, cursor: Int) async throws -> SuggestResult { .none }
    func capture(_ input: String, literals: [TextSpan]) async throws -> CaptureResult {
        try record("capture \(input)")
        return CaptureResult(added: true, review: nil)
    }
    func captureReview(_ submission: ReviewSubmission) async throws { try record("review") }
    func captureExplicit(_ input: String) async throws { try record("explicit \(input)") }
    func modify(uuid: String, before: EditFields, after: EditFields) async throws -> Bool {
        try record("modify \(uuid)")
        return before != after
    }
    func perform(_ action: TaskAction, uuid: String) async throws { try record("\(action.rawValue) \(uuid)") }
    func undo() async throws { try record("undo") }
    func plan(calendar: CalendarInput?) async throws -> DailyPlan {
        try record("plan")
        planCalendars.append(calendar)
        return planValue
    }
    func applyPlan(date: String, uuids: [String]) async throws -> PlanSaveResult {
        try record("applyPlan \(date) \(uuids.joined(separator: ","))")
        return planResults.removeFirst()
    }
    var settingsValue = SettingsSnapshot(
        configPath: "/tmp/config.toml", revision: "r1",
        planning: PlanningSettings(enabled: true, dailyCapacity: "8h", buffer: "1h", weekdayCapacity: [:]),
        calendar: CalendarSettings(enabled: false, files: []))
    /// Answered to the next save instead of applying it.
    var saveFailure: BridgeError?
    private(set) var saves: [String] = []

    func setSettings(_ value: SettingsSnapshot) { settingsValue = value }
    func setSaveFailure(_ error: BridgeError?) { saveFailure = error }

    func settings() async throws -> SettingsSnapshot {
        calls.append("settings")
        return settingsValue
    }
    func savePlanning(_ change: PlanningChange) async throws -> SettingsSnapshot {
        saves.append("planning r\(change.revision.dropFirst())")
        if let error = saveFailure {
            saveFailure = nil
            throw error
        }
        if let enabled = change.enabled { settingsValue.planning.enabled = enabled }
        if let text = change.dailyCapacity { settingsValue.planning.dailyCapacity = text }
        if let text = change.buffer { settingsValue.planning.buffer = text }
        for (day, text) in change.weekdayCapacity ?? [:] { settingsValue.planning.weekdayCapacity[day] = text.isEmpty ? nil : text }
        settingsValue.revision += "+"
        return settingsValue
    }
    func saveCalendar(_ change: CalendarChange) async throws -> SettingsSnapshot {
        saves.append("calendar r\(change.revision.dropFirst())")
        if let error = saveFailure {
            saveFailure = nil
            throw error
        }
        if let source = change.source { settingsValue.calendar.source = source }
        if let enabled = change.enabled { settingsValue.calendar.enabled = enabled }
        if let time = change.workingStart { settingsValue.calendar.workingStart = time }
        if let time = change.workingEnd { settingsValue.calendar.workingEnd = time }
        if let include = change.includeAllDay { settingsValue.calendar.includeAllDay = include }
        if let include = change.includeTransparent { settingsValue.calendar.includeTransparent = include }
        if let text = change.staleAfter { settingsValue.calendar.staleAfter = text }
        if let paths = change.paths {
            settingsValue.calendar.files = paths.map { CalendarFile(path: $0, status: "available", detail: nil, modified: nil) }
        }
        settingsValue.revision += "+"
        return settingsValue
    }
    /// What the server says of Google Calendar, how a connect goes, and the
    /// failure the next status or connect answers instead.
    var googleStatus = GoogleCalendarStatus(offered: true, connected: false, email: nil, needsReconnect: false)
    var googleLink = CalendarConnectLink(url: "https://momentum.example/calendar/connect/abc", expiresAt: .now.addingTimeInterval(600))
    var googleFailure: BridgeError?

    func setGoogle(status: GoogleCalendarStatus? = nil, link: CalendarConnectLink? = nil, failure: BridgeError? = nil) {
        if let status { googleStatus = status }
        if let link { googleLink = link }
        googleFailure = failure
    }

    func calendarStatus() async throws -> GoogleCalendarStatus {
        calls.append("calendarStatus")
        if let error = googleFailure { throw error }
        return googleStatus
    }

    func connectCalendar() async throws -> CalendarConnectLink {
        calls.append("connectCalendar")
        if let error = googleFailure { throw error }
        return googleLink
    }

    var doctorValue = DoctorReport(
        configPath: "/tmp/config.toml", bridgeVersion: "v0.1.0-test",
        checks: [
            DoctorCheck(name: "Backend server", status: "ok", detail: "https://momentum.example answers; version 1.4.0"),
            DoctorCheck(name: "Sign-in", status: "fail", detail: "unauthorized"),
            DoctorCheck(name: "Calendar source", status: "skip", detail: "disabled; task-only planning remains available"),
        ],
        ready: false)
    var doctorFailure: BridgeError?

    func setDoctor(_ value: DoctorReport) { doctorValue = value }
    func setDoctorFailure(_ error: BridgeError?) { doctorFailure = error }
    func doctor() async throws -> DoctorReport {
        calls.append("doctor")
        if let error = doctorFailure { throw error }
        return doctorValue
    }

    var projectsValue = ProjectsSnapshot(revision: "p1", entries: [], tasksError: nil)
    var previewValue = ProjectPreview(
        value: "", valueChanged: false, catalogDescendants: 0, exactTasks: [], subprojectTasks: [], destinationTaskOnly: false)
    var projectSaveValue = ProjectSaveResult(revision: "p2", changed: 0)
    /// Answered to the next preview or save instead of applying it.
    var projectFailure: BridgeError?
    /// The previews and saves asked for, in order.
    private(set) var previews: [ProjectEdit] = []
    private(set) var projectSaves: [ProjectEdit] = []

    func setProjects(_ value: ProjectsSnapshot) { projectsValue = value }
    func setPreview(_ value: ProjectPreview) { previewValue = value }
    func setProjectSave(_ value: ProjectSaveResult) { projectSaveValue = value }
    func setProjectFailure(_ error: BridgeError?) { projectFailure = error }

    func projects() async throws -> ProjectsSnapshot {
        calls.append("projects")
        return projectsValue
    }
    func previewMigration(_ edit: ProjectEdit) async throws -> ProjectPreview {
        previews.append(edit)
        if let error = projectFailure {
            projectFailure = nil
            throw error
        }
        return previewValue
    }
    func saveProject(_ edit: ProjectEdit) async throws -> ProjectSaveResult {
        projectSaves.append(edit)
        if let error = projectFailure {
            projectFailure = nil
            throw error
        }
        return projectSaveValue
    }
    func setToken(_ token: String) async throws { try record("setToken \(token)") }
    func startSignIn() async throws -> DeviceSignIn {
        try record("signInStart")
        return DeviceSignIn(
            deviceCode: "dc", userCode: "BCDFGHJK", verificationUrl: "https://momentum.example/device",
            verificationUrlComplete: "https://momentum.example/device?code=BCDFGHJK", expiresAt: .now.addingTimeInterval(600), intervalSeconds: 5)
    }
    func pollSignIn(deviceCode: String) async throws -> SignInPoll {
        calls.append("poll \(deviceCode)")
        return try polls.removeFirst().get()
    }
    func revoke(token: String) async throws { try record("revoke \(token)") }
}

func snapshot(today: [TaskItem] = [], inbox: [TaskItem] = []) -> Snapshot {
    Snapshot(
        tasks: today + inbox,
        today: today.isEmpty ? [] : [TaskSection(title: "Due Today", project: nil, uuids: today.map(\.uuid))],
        inbox: inbox.isEmpty ? [] : [TaskSection(title: "No project", project: nil, uuids: inbox.map(\.uuid))],
        completed: [], completedError: nil, projects: [])
}

/// A store signed in to the bridge's server.
@MainActor
func signedInStore(_ bridge: FakeBridge) -> AppStore {
    let signIns = FakeSignInStore()
    signIns.saved[backendURL] = signIn("mtk_test")
    return AppStore(bridge: bridge, signIns: signIns)
}

@MainActor
struct AppStoreTests {
    @Test func startupOpensInboxWhenTodayIsEmpty() async {
        let store = signedInStore(FakeBridge(snapshots: [snapshot(inbox: [.fixture("a")])]))
        await store.start()
        #expect(store.view == .inbox && store.selection == "a")
    }

    @Test func startupOpensTodayWhenItHasWork() async {
        let store = signedInStore(FakeBridge(snapshots: [snapshot(today: [.fixture("t")], inbox: [.fixture("a")])]))
        await store.start()
        #expect(store.view == .today && store.selection == "t")
    }

    @Test func completionMovesSelectionToTheNextTaskThenThePrevious() async {
        let (a, b, c) = (TaskItem.fixture("a"), TaskItem.fixture("b"), TaskItem.fixture("c"))
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a, b, c]), snapshot(inbox: [a, c]), snapshot(inbox: [a])])
        let store = signedInStore(bridge)
        await store.start()
        store.selection = "b"
        await store.complete(b)
        #expect(store.selection == "c")
        await store.complete(c)
        #expect(store.selection == "a")
        #expect(await bridge.calls.filter { $0.hasPrefix("complete") } == ["complete b", "complete c"])
    }

    @Test func mutationOpensUndo() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a])])
        let store = signedInStore(bridge)
        await store.start()
        await store.toggleStart(a)
        #expect(store.undoWindow.isAvailable(at: .now) && store.toast?.offersUndo == true)
        await store.undo()
        #expect(!store.undoWindow.isAvailable(at: .now))
        #expect(await bridge.calls.contains("undo"))
    }

    @Test func aZeroMutationDelayOffersNoUndo() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(mutationDelay: 0, snapshots: [snapshot(inbox: [a])])
        let store = signedInStore(bridge)
        await store.start()
        await store.delete(a)
        await store.undo()
        #expect(await !bridge.calls.contains("undo"))
        #expect(store.toast?.message == "Nothing to undo")
    }

    @Test func withoutAServerNothingLoads() async {
        let bridge = FakeBridge(backendURL: nil, snapshots: [snapshot(inbox: [.fixture("a")])])
        let store = AppStore(bridge: bridge, signIns: FakeSignInStore())
        await store.start()
        #expect(store.status != nil && store.backendURL == nil && store.snapshot == nil && store.server == .unknown)
        #expect(await bridge.calls.isEmpty)
    }

    @Test func failedMutationShowsErrorAndKeepsUndoClosed() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a])])
        let store = signedInStore(bridge)
        await store.start()
        await bridge.setFailure(BridgeError(message: "complete failed (exit 1): hook rejected"))
        await store.complete(a)
        #expect(store.toast?.kind == .error && store.toast?.message.contains("hook rejected") == true)
        #expect(!store.undoWindow.isAvailable(at: .now))
    }

    @Test func searchFiltersSectionsAndRepairsSelection() async {
        let store = signedInStore(FakeBridge(snapshots: [snapshot(inbox: [.fixture("Renew cert"), .fixture("Buy milk")])]))
        await store.start()
        store.selection = "Renew cert"
        store.query = "milk"
        #expect(store.visibleTasks.map(\.uuid) == ["Buy milk"] && store.selection == "Buy milk")
    }

    @Test func editFieldErrorsReachTheEditor() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a])])
        let store = signedInStore(bridge)
        await store.start()
        await bridge.setFailure(BridgeError(message: "description cannot be empty", field: "description"))
        var edits = a.edit
        edits.description = " "
        await #expect(throws: BridgeError(message: "description cannot be empty", field: "description")) {
            try await store.save(a, edits: edits)
        }
        #expect(!store.undoWindow.isAvailable(at: .now))
    }
}

@MainActor
struct TaskKeyCommandTests {
    /// Inbox holds a, b and a weekly task; Completed holds one task.
    private func startedStore() async -> AppStore {
        var tasks = snapshot(inbox: [.fixture("a"), .fixture("b"), .fixture("plants", recur: "weekly")])
        tasks.tasks.append(.fixture("done", status: "completed"))
        tasks.completed = [TaskSection(title: "Today", project: nil, uuids: ["done"])]
        let store = signedInStore(FakeBridge(snapshots: [tasks]))
        await store.start()
        return store
    }

    @discardableResult
    private func press(
        _ characters: String, _ key: KeyEquivalent? = nil, modifiers: EventModifiers = [], in store: AppStore,
        focusSearch: () -> Void = {}
    ) -> Bool {
        let result = TaskKeyCommands.handle(
            key: key ?? KeyEquivalent(Character(characters)), characters: characters, modifiers: modifiers, store: store,
            focusSearch: focusSearch)
        if case .handled = result { return true }
        return false
    }

    @Test func numberKeysOpenTheViewsInTheTUIOrder() async {
        let store = await startedStore()
        press("3", in: store)
        #expect(store.view == .completed)
        press("2", in: store)
        #expect(store.view == .today)
        press("1", in: store)
        #expect(store.view == .inbox)
    }

    @Test func movementKeysStayInsideTheList() async {
        let store = await startedStore()
        #expect(store.selection == "a")
        press("j", in: store)
        #expect(store.selection == "b")
        press("j", in: store)
        press("j", in: store)
        #expect(store.selection == "plants")
        press("k", in: store)
        #expect(store.selection == "b")
        press("g", in: store)
        #expect(store.selection == "a")
        press("G", modifiers: .shift, in: store)
        #expect(store.selection == "plants")
    }

    @Test func returnShowsAndHidesTheInspector() async {
        let store = await startedStore()
        #expect(store.showInspector)
        press("\r", .return, in: store)
        #expect(!store.showInspector)
        press("\r", .return, in: store)
        #expect(store.showInspector)
    }

    @Test func actionKeysSayCompletedTasksAreReadOnly() async {
        let store = await startedStore()
        press("3", in: store)
        #expect(store.selection == "done")
        for (characters, key) in [(" ", KeyEquivalent.space), ("s", nil), ("d", nil), ("x", nil), ("e", nil), ("E", nil)] {
            store.dismissToast()
            #expect(press(characters, key, in: store))
            #expect(store.toast?.message == "Completed tasks are read-only")
        }
        #expect(store.pendingDelete == nil && store.pendingStopRecurrence == nil && store.editing == nil)
    }

    @Test func actionKeysOpenTheirDialogsAndEditors() async {
        let store = await startedStore()
        press("d", in: store)
        #expect(store.pendingDelete?.uuid == "a")
        store.pendingDelete = nil
        press("x", in: store)
        #expect(store.pendingStopRecurrence == nil && store.toast?.message == "This task doesn’t repeat")
        press("G", modifiers: .shift, in: store)
        press("x", in: store)
        #expect(store.pendingStopRecurrence?.uuid == "plants")
        store.pendingStopRecurrence = nil
        press("E", modifiers: .shift, in: store)
        #expect(store.editing?.task.uuid == "plants" && store.editing?.focus == .estimate)
    }

    @Test func keysWaitForOverlaysAndLeaveCommandShortcutsToTheMenus() async {
        let store = await startedStore()
        #expect(!press("1", modifiers: .command, in: store))
        store.showCapture = true
        #expect(!press("3", in: store))
        #expect(store.view == .inbox)
    }

    @Test func slashFocusesSearchAndEscClearsIt() async {
        let store = await startedStore()
        var searching = false
        press("/", in: store, focusSearch: { searching = true })
        #expect(searching)
        store.query = "b"
        #expect(press("\u{1b}", .escape, in: store))
        #expect(store.query.isEmpty)
        #expect(!press("\u{1b}", .escape, in: store))
    }
}
