import Foundation
import Observation

/// Where this Mac reads its meetings from. The choice is kept on this Mac, not
/// in config.toml, which the terminal app shares.
enum CalendarSource: String, Sendable {
    case files
    case mac
}

enum CalendarAccess: Sendable, Equatable {
    case notDetermined
    /// Refused, restricted, or allowed to add events only.
    case denied
    case granted
}

struct MacCalendarInfo: Identifiable, Sendable, Equatable {
    let id: String
    let title: String
    let account: String
}

/// The Mac's calendars. EventKit is behind it so tests and snapshots read
/// no real calendar.
@MainActor
protocol CalendarEventSource {
    var access: CalendarAccess { get }
    func requestAccess() async -> CalendarAccess
    func calendars() -> [MacCalendarInfo]
    /// The events of the day in the given calendars, with their titles.
    func events(on day: Date, in calendarIDs: Set<String>) -> [CalendarEvent]
}

/// What a store has before it is given a calendar: nothing to read.
struct NoCalendarSource: CalendarEventSource {
    var access: CalendarAccess { .denied }
    func requestAccess() async -> CalendarAccess { .denied }
    func calendars() -> [MacCalendarInfo] { [] }
    func events(on day: Date, in calendarIDs: Set<String>) -> [CalendarEvent] { [] }
}

/// The Calendar tab's macOS Calendar source: the choice, whether the Mac
/// allows it, which calendars count, and what is handed to the bridge. It
/// never writes config.toml, so choosing it leaves `[calendar] enabled` and
/// `paths` as they are.
@MainActor
@Observable
final class MacCalendarModel {
    static let sourceKey = "calendar.source"
    static let offKey = "calendar.mac.off"

    private(set) var source: CalendarSource
    private(set) var access: CalendarAccess
    private(set) var calendars: [MacCalendarInfo] = []
    /// The calendars switched off; a calendar added later counts until it is.
    private(set) var off: Set<String>
    @ObservationIgnored weak var store: AppStore?
    @ObservationIgnored private let eventSource: any CalendarEventSource
    @ObservationIgnored private let defaults: UserDefaults

    init(eventSource: any CalendarEventSource, defaults: UserDefaults = .standard) {
        self.eventSource = eventSource
        self.defaults = defaults
        source = defaults.string(forKey: Self.sourceKey).flatMap(CalendarSource.init(rawValue:)) ?? .files
        off = Set(defaults.stringArray(forKey: Self.offKey) ?? [])
        access = eventSource.access
        if source == .mac { calendars = eventSource.calendars() }
    }

    /// The calendars by account, in the order the Mac lists them.
    var accounts: [(name: String, calendars: [MacCalendarInfo])] {
        var names: [String] = []
        for calendar in calendars where !names.contains(calendar.account) { names.append(calendar.account) }
        return names.map { name in (name, calendars.filter { $0.account == name }) }
    }

    /// Reads what the Mac allows now, without asking. A person who allowed
    /// access in System Settings sees it when they come back.
    func refresh() async {
        let before = access
        access = eventSource.access
        calendars = access == .granted ? eventSource.calendars() : []
        if source == .mac, access != before { await reloadTasks() }
    }

    /// Chooses where meetings come from. Choosing macOS Calendar asks the Mac
    /// for access the first time.
    func choose(_ choice: CalendarSource) async {
        source = choice
        defaults.set(choice.rawValue, forKey: Self.sourceKey)
        if choice == .mac { await requestAccess() }
        await reloadTasks()
    }

    func requestAccess() async {
        if eventSource.access == .notDetermined {
            access = await eventSource.requestAccess()
        } else {
            access = eventSource.access
        }
        calendars = access == .granted ? eventSource.calendars() : []
    }

    func setCalendar(_ id: String, on: Bool) async {
        if on { off.remove(id) } else { off.insert(id) }
        defaults.set(off.sorted(), forKey: Self.offKey)
        await reloadTasks()
    }

    /// What load and plan hand to the bridge: nil while the source is calendar
    /// files, the reason there are no events when the Mac does not allow
    /// them, and otherwise today's events in the calendars that are on.
    func input(titles: Bool, on day: Date = .now) -> CalendarInput? {
        guard source == .mac else { return nil }
        guard eventSource.access == .granted else { return CalendarInput(error: "no access", events: []) }
        let on = Set(eventSource.calendars().map(\.id)).subtracting(off)
        let events = eventSource.events(on: day, in: on).map { event -> CalendarEvent in
            var event = event
            if !titles { event.title = nil }
            return event
        }
        return CalendarInput(error: nil, events: events)
    }

    /// The meter and Plan Today use the new source on the next load.
    private func reloadTasks() async {
        if let store, store.account != nil {
            await store.refresh()
        }
    }
}
