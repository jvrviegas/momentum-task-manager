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

    init(backendURL: String? = "https://momentum.example", mutationDelay: Int = 15, snapshots: [Snapshot]) {
        statusValue = BridgeStatus(
            configPath: "/tmp/config.toml", mutationDelaySeconds: mutationDelay, refreshIntervalSeconds: 0, backendUrl: backendURL)
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
