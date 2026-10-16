import Foundation
import Security
import Testing
@testable import Momentum

let backendURL = "https://momentum.example"

/// Sign-ins in memory instead of the Keychain.
final class FakeSignInStore: SignInStore, @unchecked Sendable {
    var saved: [String: SavedSignIn] = [:]
    var refusesSave = false

    func read(server: String) throws -> SavedSignIn? { saved[server] }
    func save(_ signIn: SavedSignIn) throws {
        if refusesSave { throw KeychainError(status: errSecAuthFailed) }
        saved[signIn.server] = signIn
    }
    func remove(server: String) throws { saved[server] = nil }
}

/// The waits a sign-in asked for.
actor Waits {
    var seconds: [TimeInterval] = []
    func append(_ value: TimeInterval) { seconds.append(value) }
}

func signIn(_ token: String, expiresIn: TimeInterval = 3600) -> SavedSignIn {
    SavedSignIn(server: backendURL, token: token, email: "ana@example.com", expiresAt: .now.addingTimeInterval(expiresIn))
}

func signedIn(_ token: String) -> Result<SignInPoll, BridgeError> {
    .success(SignInPoll(state: "signedIn", token: token, email: "ana@example.com", expiresAt: .now.addingTimeInterval(90 * 86_400)))
}

let pending = Result<SignInPoll, BridgeError>.success(SignInPoll(state: "pending"))
let unreachable = BridgeError(message: "cannot reach the Momentum server: connection refused", code: "unreachable")
let unauthorized = BridgeError(message: "sign in again", code: "unauthorized")

@MainActor
struct BackendModeTests {
    @Test func startsSignedOutWithoutASavedSignIn() async {
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot(inbox: [.fixture("a")])])
        let store = AppStore(bridge: bridge, signIns: FakeSignInStore())
        await store.start()
        #expect(store.isBackend && store.server == .signedOut && store.account == nil && store.snapshot == nil)
        #expect(await bridge.calls.isEmpty)
    }

    @Test func anExpiredSignInCountsAsSignedOut() async {
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_old", expiresIn: -1)
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot(inbox: [.fixture("a")])])
        let store = AppStore(bridge: bridge, signIns: signIns)
        await store.start()
        #expect(store.server == .signedOut && store.account == nil)
        #expect(await bridge.calls.isEmpty)
    }

    @Test func restoresTheKeychainSignInForTheServer() async {
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_saved")
        // The config may end the address with a slash.
        let bridge = FakeBridge(backendURL: backendURL + "/", snapshots: [snapshot(inbox: [.fixture("a")])])
        let store = AppStore(bridge: bridge, signIns: signIns)
        await store.start()
        #expect(await bridge.calls == ["setToken mtk_saved", "load"])
        #expect(store.backendURL == backendURL && store.server == .online && store.account?.email == "ana@example.com")
        #expect(store.view == .inbox && store.selection == "a")
        #expect(store.sync.backend && !store.sync.enabled && !store.needsQuitPrompt)
    }

    @Test func signInPollsUntilThePersonSignsInAndRevokesThePreviousSignIn() async {
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_old", expiresIn: -1)
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot(today: [.fixture("t")])])
        await bridge.setPolls([pending, .success(SignInPoll(state: "slowDown")), pending, signedIn("mtk_new")])
        let waits = Waits()
        let store = AppStore(bridge: bridge, signIns: signIns, sleep: { await waits.append($0) })
        await store.start()

        await store.beginSignIn().value
        #expect(await waits.seconds == [5, 5, 10, 10])
        #expect(await bridge.calls == ["signInStart", "poll dc", "poll dc", "poll dc", "poll dc", "setToken mtk_new", "revoke mtk_old", "load"])
        #expect(signIns.saved[backendURL]?.token == "mtk_new" && store.account?.token == "mtk_new")
        #expect(store.signIn == nil && store.server == .online && store.view == .today && store.selection == "t")
        #expect(store.toast?.message == "Signed in as ana@example.com")
    }

    @Test func aRefusedSignInOffersToTryAgain() async {
        let signIns = FakeSignInStore()
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot()])
        await bridge.setPolls([pending, .failure(BridgeError(message: "sign-in refused: this account may not use this server"))])
        let store = AppStore(bridge: bridge, signIns: signIns, sleep: { _ in })
        await store.start()
        await store.beginSignIn().value
        #expect(store.signIn?.phase == .failed("sign-in refused: this account may not use this server"))
        #expect(signIns.saved.isEmpty && store.account == nil && store.server == .signedOut)
        #expect(await !bridge.calls.contains { $0.hasPrefix("setToken") })
    }

    @Test func aSignInTheKeychainRefusesIsRevoked() async {
        let signIns = FakeSignInStore()
        signIns.refusesSave = true
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot()])
        await bridge.setPolls([signedIn("mtk_new")])
        let store = AppStore(bridge: bridge, signIns: signIns, sleep: { _ in })
        await store.start()
        await store.beginSignIn().value
        #expect(await bridge.calls == ["signInStart", "poll dc", "revoke mtk_new"])
        guard case .failed(let message) = store.signIn?.phase else {
            Issue.record("phase \(String(describing: store.signIn?.phase))")
            return
        }
        #expect(message.hasPrefix("Couldn’t save the sign-in in the Keychain: "))
        #expect(store.account == nil && store.server == .signedOut)
    }

    @Test func cancellingStopsPolling() async throws {
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot()])
        let store = AppStore(bridge: bridge, signIns: FakeSignInStore(), sleep: { try await Task.sleep(for: .seconds($0 * 60)) })
        await store.start()
        let flow = store.beginSignIn()
        for _ in 0..<200 where store.signIn?.phase == .starting {
            try await Task.sleep(for: .milliseconds(5))
        }
        guard case .waiting(let start) = store.signIn?.phase else {
            Issue.record("phase \(String(describing: store.signIn?.phase))")
            return
        }
        #expect(start.userCode == "BCDFGHJK")
        store.cancelSignIn()
        await flow.value
        #expect(store.signIn == nil)
        #expect(await bridge.calls == ["signInStart"])
    }

    @Test func signOutRevokesAndForgetsTheSignIn() async {
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_saved")
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot(inbox: [.fixture("a")])])
        let store = AppStore(bridge: bridge, signIns: signIns)
        await store.start()
        await store.signOut()
        #expect(await bridge.calls.suffix(2) == ["revoke mtk_saved", "setToken "])
        #expect(signIns.saved.isEmpty && store.account == nil && store.server == .signedOut)
        #expect(store.snapshot == nil && store.visibleTasks.isEmpty && store.selection == nil)
        #expect(store.toast?.message == "Signed out")
    }

    @Test func signOutForgetsTheSignInWhenTheServerCannotRevokeIt() async {
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_saved")
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot(inbox: [.fixture("a")])])
        let store = AppStore(bridge: bridge, signIns: signIns)
        await store.start()
        await bridge.setFailure(unreachable)
        await store.signOut()
        #expect(signIns.saved.isEmpty && store.account == nil && store.server == .signedOut)
        #expect(store.toast?.kind == .error)
        #expect(store.toast?.message.hasPrefix("Signed out. The server couldn’t revoke the sign-in, so its token works until ") == true)
        #expect(store.toast?.message.hasSuffix(unreachable.message) == true)
    }

    @Test func serverStateFollowsWhatTheServerAnswers() async {
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_saved")
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge, signIns: signIns)
        await store.start()
        #expect(store.server == .online)

        await bridge.setLoadFailure(unreachable)
        await store.refresh()
        #expect(store.server == .unreachable && store.snapshot != nil && store.toast?.kind == .error)
        await store.refresh()
        #expect(store.server == .online)

        // The bridge refuses some changes itself; they say nothing about the server.
        await bridge.setFailure(BridgeError(message: "unsupported date", field: "due"))
        await store.complete(a)
        #expect(store.server == .online)
        await bridge.setFailure(unauthorized)
        await store.complete(a)
        #expect(store.server == .signedOut && store.toast?.message == "sign in again")
        // The tasks stay on screen, as in the TUI, and the saved sign-in stays until a new one replaces it.
        #expect(store.snapshot != nil && signIns.saved[backendURL] != nil)
    }

    @Test func undoLastsForTheMutationDelay() async {
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_saved")
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(backendURL: backendURL, snapshots: [snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge, signIns: signIns)
        await store.start()

        let before = Date.now
        await store.complete(a)
        let closes = store.undoClosesAt
        #expect(closes != nil && closes! >= before.addingTimeInterval(15) && closes! <= .now.addingTimeInterval(15))
        #expect(store.toast?.offersUndo == true && store.toast?.expiresAt == nil)
        store.tick(at: .now.addingTimeInterval(14))
        #expect(store.sync.undoAvailable)
        store.tick(at: .now.addingTimeInterval(16))
        #expect(!store.sync.undoAvailable && store.undoClosesAt == nil && store.toast?.expiresAt != nil)
        await store.undo()
        #expect(store.toast?.message == "Undo is only available for 15 seconds after a change")

        await store.toggleStart(a)
        await store.undo()
        let calls = await bridge.calls
        #expect(calls.filter { $0 == "undo" }.count == 1 && !calls.contains("sync") && !store.needsQuitPrompt)
    }

    @Test func undoClosesAWindowWhoseTickIsLate() async {
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_saved")
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(backendURL: backendURL, mutationDelay: 1, snapshots: [snapshot(inbox: [a])])
        let store = AppStore(bridge: bridge, signIns: signIns)
        await store.start()
        await store.complete(a)
        // Blocks the main actor, so the clock's tick cannot close the window first.
        usleep(1_100_000)
        await store.undo()
        #expect(await !bridge.calls.contains("undo"))
        #expect(store.toast?.message == "Undo is only available for 1 second after a change")
    }
}

struct KeychainSignInStoreTests {
    /// Uses the login Keychain under a service of its own, and removes what it saves.
    @Test func savesReplacesAndRemovesASignIn() throws {
        let store = KeychainSignInStore(service: "dev.jvrviegas.momentum.tests.\(UUID().uuidString)")
        let server = "https://momentum.example"
        defer { try? store.remove(server: server) }
        let first = SavedSignIn(server: server, token: "mtk_first", email: "ana@example.com", expiresAt: Date(timeIntervalSince1970: 1_800_000_000))
        #expect(try store.read(server: server) == nil)
        try store.save(first)
        #expect(try store.read(server: server) == first)
        var second = first
        second.token = "mtk_second"
        try store.save(second)
        #expect(try store.read(server: server) == second)
        #expect(try store.read(server: "https://other.example") == nil)
        try store.remove(server: server)
        #expect(try store.read(server: server) == nil)
        try store.remove(server: server)
    }
}

struct ProcessBridgeTests {
    /// A stand-in bridge that logs each launch and request next to itself,
    /// answers every request, and exits after answering undo.
    static let script = """
    #!/bin/sh
    log="$0.log"
    echo launch >> "$log"
    while IFS= read -r line; do
        echo "$line" >> "$log"
        id=$(echo "$line" | sed -E 's/.*"id":([0-9]+).*/\\1/')
        echo "{\\"id\\":$id,\\"result\\":{}}"
        case "$line" in *'"op":"undo"'*) exit 0 ;; esac
    done
    """

    @Test func sendsTheTokenAgainWhenTheBridgeRestarts() async throws {
        let directory = FileManager.default.temporaryDirectory.appending(path: "momentum-bridge-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let executable = directory.appending(path: "bridge")
        try Data(Self.script.utf8).write(to: executable)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: executable.path)

        let bridge = ProcessBridge(executable: executable)
        try await bridge.setToken("mtk_saved")
        try await bridge.undo()
        try await Task.sleep(for: .milliseconds(300)) // the stand-in exits
        try await bridge.perform(.complete, uuid: "a")
        try await bridge.setToken("")
        try await bridge.undo()
        try await Task.sleep(for: .milliseconds(300))
        try await bridge.perform(.start, uuid: "a")

        let log = try String(contentsOf: directory.appending(path: "bridge.log"), encoding: .utf8)
        let ops = log.split(separator: "\n").map { line in
            line == "launch" ? "launch" : String(line.firstMatch(of: /"op":"(\w+)"/)!.1)
                + (line.contains("mtk_saved") ? " mtk_saved" : line.contains(#""token":"""#) ? " none" : "")
        }
        #expect(ops == [
            "launch", "setToken mtk_saved", "undo",
            "launch", "setToken mtk_saved", "complete", "setToken none", "undo",
            "launch", "start",
        ])
    }
}
