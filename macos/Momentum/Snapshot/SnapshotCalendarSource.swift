#if DEBUG
import Foundation

/// The Mac's Calendar for a snapshot run: a fixed set of calendars and a
/// chosen answer from the Mac, so no real calendar is read and no permission
/// prompt appears.
@MainActor
final class SnapshotCalendarSource: CalendarEventSource {
    private(set) var access: CalendarAccess
    private let all: [MacCalendarInfo]

    init(access: CalendarAccess = .granted, calendars: [MacCalendarInfo] = SnapshotCalendarSource.defaultCalendars) {
        self.access = access
        all = calendars
    }

    static let defaultCalendars = [
        MacCalendarInfo(id: "icloud-home", title: "Home", account: "iCloud"),
        MacCalendarInfo(id: "icloud-family", title: "Family", account: "iCloud"),
        MacCalendarInfo(id: "google-main", title: "joao@example.com", account: "Google"),
        MacCalendarInfo(id: "google-team", title: "Team", account: "Google"),
    ]

    func requestAccess() async -> CalendarAccess {
        if access == .notDetermined { access = .granted }
        return access
    }

    func calendars() -> [MacCalendarInfo] { all }
    func events(on day: Date, in calendarIDs: Set<String>) -> [CalendarEvent] { [] }
}
#endif
