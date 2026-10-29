import EventKit

/// The Mac's Calendar through EventKit. It only reads: events are never
/// created or changed, and nothing it reads is stored.
@MainActor
final class EventKitSource: CalendarEventSource {
    private let store = EKEventStore()

    var access: CalendarAccess {
        switch EKEventStore.authorizationStatus(for: .event) {
        case .fullAccess: .granted
        case .notDetermined: .notDetermined
        default: .denied
        }
    }

    func requestAccess() async -> CalendarAccess {
        _ = try? await store.requestFullAccessToEvents()
        return access
    }

    func calendars() -> [MacCalendarInfo] {
        store.calendars(for: .event)
            .map { MacCalendarInfo(id: $0.calendarIdentifier, title: $0.title, account: $0.source?.title ?? "Other") }
            .sorted { ($0.account, $0.title) < ($1.account, $1.title) }
    }

    func events(on day: Date, in calendarIDs: Set<String>) -> [CalendarEvent] {
        let chosen = store.calendars(for: .event).filter { calendarIDs.contains($0.calendarIdentifier) }
        guard !chosen.isEmpty else { return [] }
        let start = Calendar.current.startOfDay(for: day)
        let end = Calendar.current.date(byAdding: .day, value: 1, to: start) ?? start
        let predicate = store.predicateForEvents(withStart: start, end: end, calendars: chosen)
        return store.events(matching: predicate)
            // A cancelled meeting takes no time.
            .filter { $0.status != .canceled }
            .map { CalendarEvent(start: $0.startDate, end: $0.endDate, allDay: $0.isAllDay, free: $0.availability == .free, title: $0.title) }
    }
}
