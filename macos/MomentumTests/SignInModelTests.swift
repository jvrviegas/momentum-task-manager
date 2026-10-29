import Foundation
import Testing
@testable import Momentum

@MainActor
struct SignInModelTests {
    @Test func pollsUntilThePersonSignsInThenSavesTheSignInAndSendsTheToken() async {
        let signIns = FakeSignInStore()
        let old = signIn("mtk_old", expiresIn: -1)
        signIns.saved[backendURL] = old
        let bridge = FakeBridge(snapshots: [snapshot()])
        await bridge.setPolls([pending, .success(SignInPoll(state: "slowDown")), pending, signedIn("mtk_new")])
        let waits = Waits()
        let model = SignInModel(bridge: bridge, signIns: signIns, sleep: { await waits.append($0) })
        var handed: [(saved: SavedSignIn, previous: SavedSignIn?)] = []
        model.onSignedIn = { handed.append(($0, $1)) }

        await model.begin(server: backendURL).value
        #expect(await waits.seconds == [5, 5, 10, 10])
        #expect(await bridge.calls == ["signInStart", "poll dc", "poll dc", "poll dc", "poll dc", "setToken mtk_new"])
        #expect(signIns.saved[backendURL]?.token == "mtk_new" && model.flow == nil)
        #expect(handed.map(\.saved.token) == ["mtk_new"] && handed.first?.previous == old)
    }

    @Test func aRefusedSignInFailsWithTheServersReason() async {
        let signIns = FakeSignInStore()
        let bridge = FakeBridge(snapshots: [snapshot()])
        await bridge.setPolls([pending, .failure(BridgeError(message: "sign-in refused: this account may not use this server"))])
        let model = SignInModel(bridge: bridge, signIns: signIns, sleep: { _ in })
        var handed = 0
        model.onSignedIn = { _, _ in handed += 1 }

        await model.begin(server: backendURL).value
        #expect(model.flow == SignInModel.Flow(server: backendURL, phase: .failed("sign-in refused: this account may not use this server")))
        #expect(signIns.saved.isEmpty && handed == 0)
        #expect(await !bridge.calls.contains { $0.hasPrefix("setToken") })
    }

    @Test func anAnswerWithoutATokenFails() async {
        let signIns = FakeSignInStore()
        let bridge = FakeBridge(snapshots: [snapshot()])
        await bridge.setPolls([.success(SignInPoll(state: "signedIn"))])
        let model = SignInModel(bridge: bridge, signIns: signIns, sleep: { _ in })

        await model.begin(server: backendURL).value
        #expect(model.flow?.phase == .failed("The server answered without a sign-in."))
        #expect(signIns.saved.isEmpty)
        #expect(await bridge.calls == ["signInStart", "poll dc"])
    }

    @Test func aSignInTheKeychainRefusesIsRevoked() async {
        let signIns = FakeSignInStore()
        signIns.refusesSave = true
        let bridge = FakeBridge(snapshots: [snapshot()])
        await bridge.setPolls([signedIn("mtk_new")])
        let model = SignInModel(bridge: bridge, signIns: signIns, sleep: { _ in })
        var handed = 0
        model.onSignedIn = { _, _ in handed += 1 }

        await model.begin(server: backendURL).value
        #expect(await bridge.calls == ["signInStart", "poll dc", "revoke mtk_new"])
        guard case .failed(let message) = model.flow?.phase else {
            Issue.record("phase \(String(describing: model.flow?.phase))")
            return
        }
        #expect(message.hasPrefix("Couldn’t save the sign-in in the Keychain: "))
        #expect(handed == 0)
    }

    @Test func cancellingStopsPolling() async throws {
        let bridge = FakeBridge(snapshots: [snapshot()])
        let model = SignInModel(bridge: bridge, signIns: FakeSignInStore(), sleep: { try await Task.sleep(for: .seconds($0 * 60)) })
        let flow = model.begin(server: backendURL)
        for _ in 0..<200 where model.flow?.phase == .starting {
            try await Task.sleep(for: .milliseconds(5))
        }
        guard case .waiting(let start) = model.flow?.phase else {
            Issue.record("phase \(String(describing: model.flow?.phase))")
            return
        }
        #expect(start.userCode == "BCDFGHJK")
        model.cancel()
        await flow.value
        #expect(model.flow == nil)
        #expect(await bridge.calls == ["signInStart"])
    }
}
