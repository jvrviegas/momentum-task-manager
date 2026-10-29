import AppKit
import SwiftUI

/// The Google Calendar source in Settings ▸ Calendar: where this account's
/// connection stands, and the way to make it. The server keeps the connection,
/// so it is the server that says.
struct GoogleCalendarSection: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        let google = store.googleCalendar
        Section {
            content(google)
        } header: {
            Text("Google Calendar")
        } footer: {
            if case .connected = google.phase {
                Text("The connection belongs to your Momentum account, not to this Mac. Choose Google Calendar on each Mac that should use it.")
            }
        }
        .task { await google.refresh() }
        .onChange(of: store.account) { Task { await google.refresh() } }
        .onChange(of: google.phase) {
            if case .connected(let email) = google.phase {
                NSAccessibility.post(
                    element: NSApp as Any, notification: .announcementRequested,
                    userInfo: [.announcement: "Connected as \(email)"])
            }
        }
    }

    @ViewBuilder
    private func content(_ google: GoogleCalendarModel) -> some View {
        switch google.phase {
        case .checking:
            HStack(spacing: 8) {
                ProgressView().controlSize(.small)
                Text("Checking Google Calendar…").foregroundStyle(Theme.text2)
            }
        case .signedOut:
            notice(
                "Sign in to connect Google Calendar",
                "Google Calendar belongs to your Momentum account. Choose Momentum Task Manager ▸ Sign In… first.",
                symbol: "lock.fill")
        case .notOffered:
            notice(
                "Google Calendar isn’t available on this server", "Ask whoever runs the server to turn it on.",
                symbol: "exclamationmark.triangle.fill")
        case .notConnected:
            description
            Button("Connect…") { google.connect() }
        case .waiting:
            HStack(spacing: 8) {
                ProgressView().controlSize(.small).accessibilityLabel("Waiting for your browser")
                Text("Waiting for your browser").fontWeight(.semibold)
            }
            Text("Approve Momentum in the browser. This tab updates when you’re done. The link works for 10 minutes.")
                .foregroundStyle(Theme.text2)
            HStack {
                Button("Open Browser") { google.openBrowserAgain() }
                Button("Cancel") { google.cancel() }
            }
        case .connected(let email):
            Label {
                Text("Connected as \(email)")
            } icon: {
                Image(systemName: "checkmark.circle.fill").foregroundStyle(Theme.statusOK)
            }
        case .needsReconnecting(let email):
            notice(
                "Needs reconnecting",
                "Google no longer accepts the saved connection to \(email). Plan Today counts no meetings until you reconnect.",
                symbol: "exclamationmark.triangle.fill")
            Button("Reconnect…") { google.connect() }
        case .expired(let reconnecting):
            let verb = reconnecting ? "Reconnect" : "Connect"
            ErrorLine(text: "The link expired. Choose \(verb)… to try again.")
            Button("\(verb)…") { google.connect() }
        case .failed(let reason):
            ErrorLine(text: "Unable to check Google Calendar: \(reason)")
            Button("Try Again") { Task { await google.refresh() } }
        }
    }

    private var description: some View {
        Text("The server reads today’s meetings in your primary Google calendar: their times, titles and whether you declined them. It never changes your calendar and never keeps the events.")
            .foregroundStyle(Theme.text2)
    }

    /// A symbol and words, never color alone.
    private func notice(_ title: String, _ detail: String, symbol: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(title, systemImage: symbol).fontWeight(.semibold)
            Text(detail).foregroundStyle(Theme.text2)
        }
        .accessibilityElement(children: .combine)
    }
}
