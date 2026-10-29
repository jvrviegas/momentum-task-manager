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
        case expired
        case failed(String)
    }

    /// How often the tab asks the server while it waits for the browser.
    static let pollInterval: TimeInterval = 2

    private(set) var phase = Phase.checking
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
        let task = Task { await runConnect() }
        polling = task
        return task
    }

    /// Stops waiting for the browser. Nothing is connected.
    func cancel() {
        polling?.cancel()
        polling = nil
        if case .waiting = phase { phase = .notConnected }
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
                    polling = nil
                    await store.refresh()
                    return
                }
                if now() >= link.expiresAt {
                    phase = .expired
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

    /// A connection Google has stopped accepting counts as none here, since
    /// connecting again replaces it; the words for it come with its own story.
    static func phase(for status: GoogleCalendarStatus) -> Phase {
        guard status.offered else { return .notOffered }
        if status.connected, !status.needsReconnect, let email = status.email { return .connected(email: email) }
        return .notConnected
    }
}
