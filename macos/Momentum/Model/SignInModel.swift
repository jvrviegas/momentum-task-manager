import Foundation
import Observation

/// The device sign-in (D1) that the sign-in sheet shows: asks the server for a
/// code, polls until the person finishes in the browser, saves the sign-in in
/// the Keychain and gives the bridge its token. What follows belongs to the
/// store, through onSignedIn.
@MainActor
@Observable
final class SignInModel {
    /// A sign-in in progress.
    struct Flow: Identifiable, Equatable {
        enum Phase: Equatable {
            case starting
            case waiting(DeviceSignIn)
            case failed(String)
        }

        var server: String
        var phase = Phase.starting
        var id: String { server }
    }

    /// The sign-in in progress; nil when there is none.
    private(set) var flow: Flow?
    /// Runs once the Keychain holds the new sign-in and the bridge has its
    /// token, with the sign-in it replaces, if any.
    @ObservationIgnored var onSignedIn: @MainActor (_ saved: SavedSignIn, _ previous: SavedSignIn?) async -> Void = { _, _ in }
    @ObservationIgnored private let bridge: any TaskBridge
    @ObservationIgnored private let signIns: any SignInStore
    @ObservationIgnored private let sleep: @Sendable (TimeInterval) async throws -> Void
    @ObservationIgnored private var task: Task<Void, Never>?

    init(bridge: any TaskBridge, signIns: any SignInStore, sleep: @escaping @Sendable (TimeInterval) async throws -> Void) {
        self.bridge = bridge
        self.signIns = signIns
        self.sleep = sleep
    }

    /// Starts a sign-in to server, replacing any in progress.
    @discardableResult
    func begin(server: String) -> Task<Void, Never> {
        task?.cancel()
        flow = Flow(server: server)
        let task = Task { await run(server) }
        self.task = task
        return task
    }

    func cancel() {
        task?.cancel()
        task = nil
        flow = nil
    }

    /// Polls until the person finishes in the browser, as `momentum login`
    /// does: every interval, five seconds longer after each slowDown.
    private func run(_ url: String) async {
        do {
            let start = try await bridge.startSignIn()
            try Task.checkCancellation()
            flow?.phase = .waiting(start)
            var interval = TimeInterval(start.intervalSeconds > 0 ? start.intervalSeconds : 5)
            while true {
                try await sleep(interval)
                let poll = try await bridge.pollSignIn(deviceCode: start.deviceCode)
                try Task.checkCancellation()
                switch poll.state {
                case "signedIn":
                    await finish(url, poll)
                    return
                case "slowDown":
                    interval += 5
                default:
                    continue
                }
            }
        } catch {
            guard !Task.isCancelled else { return }
            flow?.phase = .failed(error.localizedDescription)
        }
    }

    /// Saves the new sign-in and signs the bridge in.
    private func finish(_ url: String, _ poll: SignInPoll) async {
        guard let token = poll.token, let expiresAt = poll.expiresAt else {
            flow?.phase = .failed("The server answered without a sign-in.")
            return
        }
        let saved = SavedSignIn(server: url, token: token, email: poll.email ?? "", expiresAt: expiresAt)
        let previous = try? signIns.read(server: url)
        do {
            try signIns.save(saved)
        } catch {
            try? await bridge.revoke(token: token) // the server shows a token only once
            flow?.phase = .failed("Couldn’t save the sign-in in the Keychain: \(error.localizedDescription)")
            return
        }
        do {
            try await bridge.setToken(token)
        } catch {
            flow?.phase = .failed(error.localizedDescription) // the saved sign-in is used at the next launch
            return
        }
        flow = nil
        task = nil
        await onSignedIn(saved, previous)
    }
}
