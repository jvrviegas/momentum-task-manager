import Foundation
import Testing
@testable import Momentum

/// An in-memory bridge: `snapshots` are returned by successive loads.
actor FakeBridge: TaskBridge {
    var statusValue: BridgeStatus
    var snapshots: [Snapshot]
    private(set) var calls: [String] = []
    var failNext: BridgeError?
    var failLoad: BridgeError?
    var polls: [Result<SignInPoll, BridgeError>] = []

    init(syncConfigured: Bool = true, backendURL: String? = nil, mutationDelay: Int = 15, snapshots: [Snapshot]) {
        statusValue = BridgeStatus(
            taskwarriorVersion: backendURL == nil ? "3.5.0" : "", syncConfigured: syncConfigured && backendURL == nil,
            estimateUDA: "configured", configPath: "/tmp/config.toml",
            sync: SyncSettings(enabled: true, startup: false, shutdown: true, intervalSeconds: 300, mutationDelaySeconds: mutationDelay),
            refreshIntervalSeconds: 0, backendUrl: backendURL)
        self.snapshots = snapshots
    }

    func setFailure(_ error: BridgeError?) { failNext = error }
    func setLoadFailure(_ error: BridgeError?) { failLoad = error }
    func setPolls(_ polls: [Result<SignInPoll, BridgeError>]) { self.polls = polls }

    private func record(_ call: String) throws {
        calls.append(call)
        if let error = failNext {
            failNext = nil
            throw error
        }
    }

    func status() async throws -> BridgeStatus { statusValue }
    func load() async throws -> Snapshot {
        calls.append("load")
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
    func sync() async throws { try record("sync") }
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
        completed: [], completedError: nil, context: nil, projects: [])
}

@MainActor
struct AppStoreTests {
    @Test func startupOpensInboxWhenTodayIsEmpty() async {
        let store = AppStore(bridge: FakeBridge(snapshots: [snapshot(inbox: [.fixture("a")])]))
        await store.start()
        #expect(store.view == .inbox && store.selection == "a")
    }

    @Test func startupOpensTodayWhenItHasWork() async {
        let store = AppStore(bridge: FakeBridge(snapshots: [snapshot(today: [.fixture("t")], inbox: [.fixture("a")])]))
        await store.start()
        #expect(store.view == .today && store.selection == "t")
    }

    @Test func completionMovesSelectionToTheNextTaskThenThePrevious() async {
        let (a, b, c) = (TaskItem.fixture("a"), TaskItem.fixture("b"), TaskItem.fixture("c"))
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a, b, c]), snapshot(inbox: [a, c]), snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge)
        await store.start()
        store.selection = "b"
        await store.complete(b)
        #expect(store.selection == "c")
        await store.complete(c)
        #expect(store.selection == "a")
        #expect(await bridge.calls.filter { $0.hasPrefix("complete") } == ["complete b", "complete c"])
    }

    @Test func mutationOpensUndoUntilSyncRuns() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge)
        await store.start()
        await store.toggleStart(a)
        #expect(store.sync.phase == .grace && store.sync.undoAvailable && store.toast?.offersUndo == true)
        await store.undo()
        #expect(!store.sync.undoAvailable && store.sync.phase == .waiting)
        #expect(await bridge.calls.contains("undo"))
    }

    @Test func undoAfterSyncIsRefused() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge)
        await store.start()
        await store.delete(a)
        store.tick(at: .now.addingTimeInterval(16))
        try? await Task.sleep(for: .milliseconds(100))
        #expect(!store.sync.undoAvailable)
        await store.undo()
        #expect(await !bridge.calls.contains("undo"))
        #expect(store.toast?.message == "Undo is only available until changes sync")
    }

    @Test func localOnlyKeepsUndoWithoutSyncing() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(syncConfigured: false, snapshots: [snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge)
        await store.start()
        await store.complete(a)
        store.tick(at: .now.addingTimeInterval(3600))
        #expect(store.isLocalOnly && store.sync.undoAvailable)
        await store.undo()
        let calls = await bridge.calls
        #expect(calls.contains("undo") && !calls.contains("sync"))
    }

    @Test func failedMutationShowsErrorAndKeepsUndoClosed() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge)
        await store.start()
        await bridge.setFailure(BridgeError(message: "complete failed (exit 1): hook rejected"))
        await store.complete(a)
        #expect(store.toast?.kind == .error && store.toast?.message.contains("hook rejected") == true)
        #expect(!store.sync.undoAvailable)
    }

    @Test func searchFiltersSectionsAndRepairsSelection() async {
        let store = AppStore(bridge: FakeBridge(snapshots: [snapshot(inbox: [.fixture("Renew cert"), .fixture("Buy milk")])]))
        await store.start()
        store.selection = "Renew cert"
        store.query = "milk"
        #expect(store.visibleTasks.map(\.uuid) == ["Buy milk"] && store.selection == "Buy milk")
    }

    @Test func failedSyncBeforeQuitKeepsRetrying() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge)
        await store.start()
        await store.toggleStart(a)
        #expect(store.needsQuitPrompt)
        await bridge.setFailure(BridgeError(message: "sync failed (exit 1): connection refused"))
        #expect(await store.syncBeforeQuit() == "sync failed (exit 1): connection refused")
        #expect(store.sync.phase == .retrying && !store.sync.quitPending && store.needsQuitPrompt)
    }

    @Test func editFieldErrorsReachTheEditor() async {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge)
        await store.start()
        await bridge.setFailure(BridgeError(message: "description cannot be empty", field: "description"))
        var edits = a.edit
        edits.description = " "
        await #expect(throws: BridgeError(message: "description cannot be empty", field: "description")) {
            try await store.save(a, edits: edits)
        }
        #expect(!store.sync.undoAvailable)
    }
}
