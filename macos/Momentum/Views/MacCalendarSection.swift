import AppKit
import SwiftUI

/// The macOS Calendar source in Settings ▸ Calendar: what the Mac allows, and
/// the calendars by account with a switch each.
struct MacCalendarSection: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        let mac = store.macCalendar
        switch mac.access {
        case .granted:
            Section {
                if mac.accounts.isEmpty {
                    Text("No calendars found").foregroundStyle(Theme.text2)
                }
                ForEach(mac.accounts, id: \.name) { account in
                    Text(account.name).font(.callout).fontWeight(.semibold).foregroundStyle(Theme.text2)
                    ForEach(account.calendars) { calendar in
                        Toggle(calendar.title, isOn: Binding(get: { !mac.off.contains(calendar.id) }, set: { value in
                            Task { await mac.setCalendar(calendar.id, on: value) }
                        }))
                    }
                }
            } header: {
                Text("Calendars")
            } footer: {
                Text("Meetings in the calendars that are on take time off each day’s capacity in Plan Today. Momentum Task Manager only reads today’s events; it never changes your calendar.")
            }
        case .denied:
            Section {
                VStack(alignment: .leading, spacing: 6) {
                    Label("Momentum Task Manager can’t read your calendars", systemImage: "lock.fill").fontWeight(.semibold)
                    Text("Allow it in System Settings ▸ Privacy & Security ▸ Calendars. Until then, Plan Today shows “Calendar unavailable: no access”.")
                        .foregroundStyle(Theme.text2)
                }
                Button("Open System Settings…") {
                    if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars") {
                        NSWorkspace.shared.open(url)
                    }
                }
            } header: {
                Text("Calendars")
            }
        case .notDetermined:
            Section {
                Text("Momentum Task Manager asks your Mac for access to your calendars. It only reads the times of today’s meetings.")
                    .foregroundStyle(Theme.text2)
                Button("Allow Access…") { Task { await store.macCalendar.requestAccess() } }
            } header: {
                Text("Calendars")
            }
        }
    }
}
