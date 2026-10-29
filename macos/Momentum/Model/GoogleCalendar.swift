import AppKit
import Foundation
import Observation

/// What the Calendar tab's Source picker offers. macOS Calendar is this Mac's
/// own choice. The other two are config.toml's `[calendar] source`, which the
/// terminal app on this Mac follows too, and a macOS Calendar choice counts
/// over what config.toml says (ADR 0003, D9).
enum CalendarChoice: Hashable, Sendable {
    case files
    case mac
    case google
}

/// Where this account's Google Calendar stands: whether the server offers it,
/// whether it is connected and as whom, and the wait while a person approves
/// it in the browser. The connection belongs to the account, not to this Mac,
/// so a Mac that signs in later finds it connected already.
@MainActor
@Observable
final class GoogleCalendarModel {
    enum Phase: Equatable {
        /// Before the first answer.
        case checking
        case signedOut
        case notOffered
        case notConnected
        /// The link is open in the browser; the tab asks the server until the
        /// account is connected or the link expires.
        case waiting(CalendarConnectLink)
        case connected(email: String)
        /// Google no longer accepts the saved connection. Connecting again
        /// replaces it, so the tab calls that **Reconnect…**.
        case needsReconnecting(email: String)
        /// The wait ran out. reconnecting says which button comes back.
        case expired(reconnecting: Bool)
        case failed(String)
    }

    /// How often the tab asks the server while it waits for the browser.
    static let pollInterval: TimeInterval = 2

    private(set) var phase = Phase.checking
    /// The prompt that asks before a disconnect, which the button opens.
    var confirmingDisconnect = false
    /// A disconnect is with the server.
    private(set) var disconnecting = false
    /// Why the last disconnect failed. Nothing changed.
    private(set) var disconnectError: String?
    /// The last disconnect could not ask Google to revoke the access, which
    /// may still show in the Google account's settings. It is kept until the
    /// person connects again, and not past the app, since it is about
    /// something that already happened.
    private(set) var notRevoked = false
    /// Where the account stood the last time the server said, which Cancel and
    /// an expired link go back to.
    @ObservationIgnored private var resting = Phase.notConnected
    @ObservationIgnored weak var store: AppStore?
    @ObservationIgnored private let open: @MainActor (URL) -> Void
    @ObservationIgnored private let sleep: @Sendable (TimeInterval) async throws -> Void
    @ObservationIgnored private let now: @Sendable () -> Date
    @ObservationIgnored private var polling: Task<Void, Never>?

    init(
        open: @escaping @MainActor (URL) -> Void = { NSWorkspace.shared.open($0) },
        sleep: @escaping @Sendable (TimeInterval) async throws -> Void = { try await Task.sleep(for: .seconds($0)) },
        now: @escaping @Sendable () -> Date = { Date() }
    ) {
        self.open = open
        self.sleep = sleep
        self.now = now
    }

    /// Reads where the account's Google Calendar stands. Signed out, there is
    /// no account to ask about. A wait for the browser is left alone.
    func refresh() async {
        guard let store else { return }
        if case .waiting = phase { return }
        guard store.account != nil else {
            phase = .signedOut
            return
        }
        do {
            phase = Self.phase(for: try await store.bridge.calendarStatus())
            resting = phase
            disconnectError = nil
            if email != nil { notRevoked = false }
        } catch {
            phase = .failed(error.localizedDescription)
        }
    }

    /// Asks the server for the one-time link, opens it in the browser, and
    /// waits until the status says connected, the link expires, or the person
    /// cancels. Once connected, the app loads again so the sidebar's meter and
    /// the next Plan Today count the meetings.
    @discardableResult
    func connect() -> Task<Void, Never> {
        polling?.cancel()
        notRevoked = false
        let task = Task { await runConnect() }
        polling = task
        return task
    }

    /// The Google account the tab is about, while it is connected or needs
    /// reconnecting.
    var email: String? {
        switch phase {
        case .connected(let email), .needsReconnecting(let email): email
        default: nil
        }
    }

    /// Ends the connection, which belongs to the account, so it ends it for
    /// every Mac and the terminal app. The server deletes it even when Google
    /// could not be asked to revoke the access, and says so. A disconnect that
    /// fails changes nothing, and the button is the way to try again. The app
    /// loads again, so Plan Today and the meter give up the meetings at once.
    func disconnect() async {
        confirmingDisconnect = false
        guard let store, email != nil, !disconnecting else { return }
        disconnecting = true
        defer { disconnecting = false }
        disconnectError = nil
        do {
            let answer = try await store.bridge.disconnectCalendar()
            phase = .notConnected
            resting = .notConnected
            notRevoked = !answer.revoked
        } catch {
            disconnectError = error.localizedDescription
            return
        }
        await store.refresh()
    }

    /// Stops waiting for the browser. Nothing is connected, so the tab goes
    /// back to where it was: not connected, or needing to reconnect.
    func cancel() {
        polling?.cancel()
        polling = nil
        if case .waiting = phase { phase = resting }
    }

    /// Opens the link in the browser again, for one that did not open or was
    /// closed before the person finished.
    func openBrowserAgain() {
        if case .waiting(let link) = phase, let url = URL(string: link.url) { open(url) }
    }

    private func runConnect() async {
        guard let store else { return }
        do {
            let link = try await store.bridge.connectCalendar()
            try Task.checkCancellation()
            phase = .waiting(link)
            if let url = URL(string: link.url) { open(url) }
            while true {
                try await sleep(Self.pollInterval)
                try Task.checkCancellation()
                let connected = Self.phase(for: try await store.bridge.calendarStatus())
                try Task.checkCancellation()
                if case .connected = connected {
                    phase = connected
                    resting = connected
                    polling = nil
                    await store.refresh()
                    return
                }
                if now() >= link.expiresAt {
                    if case .needsReconnecting = resting {
                        phase = .expired(reconnecting: true)
                    } else {
                        phase = .expired(reconnecting: false)
                    }
                    polling = nil
                    return
                }
            }
        } catch {
            guard !Task.isCancelled else { return }
            phase = .failed(error.localizedDescription)
            polling = nil
        }
    }

    /// Where the server says the account stands. A connection Google has
    /// stopped accepting is not a connection that works, whoever it was made
    /// by, so it needs reconnecting.
    static func phase(for status: GoogleCalendarStatus) -> Phase {
        guard status.offered else { return .notOffered }
        guard status.connected, let email = status.email else { return .notConnected }
        return status.needsReconnect ? .needsReconnecting(email: email) : .connected(email: email)
    }
}
