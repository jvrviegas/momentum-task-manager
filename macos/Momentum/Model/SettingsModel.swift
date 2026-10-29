import Foundation
import Observation

/// What the Planning and Calendar tabs show and do. Each change is written to
/// config.toml at once through the bridge, on the version of the file the tab
/// last read; a change the file would refuse comes back with a message for
/// the field that holds it.
@MainActor
@Observable
final class SettingsModel {
    private(set) var snapshot: SettingsSnapshot?
    /// Messages for refused values, by `<tab>.<field>`: `planning.dailyCapacity`,
    /// `planning.weekdayCapacity.monday`, `calendar.enabled`, `calendar.paths`.
    private(set) var errors: [String: String] = [:]
    /// A problem that belongs to no field: a failed write, or a file that
    /// changed under the tab.
    private(set) var notice: String?
    /// Bumped when the tab reloads over what a person was typing, so fields
    /// go back to the file's values.
    private(set) var reloads = 0
    @ObservationIgnored weak var store: AppStore?

    /// The tab's values again from config.toml, which the bridge reads first
    /// when it changed.
    func reload() async {
        guard let store else { return }
        do {
            snapshot = try await store.bridge.settings()
        } catch {
            notice = error.localizedDescription
        }
    }

    /// A time of day as HH:MM, on today, in this Mac's time zone, for a time field.
    nonisolated static func date(fromTime text: String, calendar: Calendar = .current, on day: Date = .now) -> Date {
        let parts = text.split(separator: ":").compactMap { Int($0) }
        guard parts.count == 2 else { return calendar.startOfDay(for: day) }
        return calendar.date(bySettingHour: parts[0], minute: parts[1], second: 0, of: day) ?? calendar.startOfDay(for: day)
    }

    /// The HH:MM the config keeps for a time field's value.
    nonisolated static func time(from date: Date, calendar: Calendar = .current) -> String {
        let at = calendar.dateComponents([.hour, .minute], from: date)
        return String(format: "%02d:%02d", at.hour ?? 0, at.minute ?? 0)
    }

    func error(_ tab: String, _ field: String) -> String? { errors["\(tab).\(field)"] }

    func clearError(_ tab: String, _ field: String) {
        errors["\(tab).\(field)"] = nil
    }

    // MARK: Planning

    func setPlanningEnabled(_ enabled: Bool) async {
        await savePlanning { PlanningChange(revision: $0, enabled: enabled) }
    }

    func setDailyCapacity(_ text: String) async {
        await savePlanning { PlanningChange(revision: $0, dailyCapacity: text) }
    }

    func setBuffer(_ text: String) async {
        await savePlanning { PlanningChange(revision: $0, buffer: text) }
    }

    /// Sets one weekday's capacity; empty text gives it the daily capacity.
    func setWeekdayCapacity(_ day: String, _ text: String) async {
        await savePlanning { PlanningChange(revision: $0, weekdayCapacity: [day: text]) }
    }

    private func savePlanning(_ change: @escaping (String) -> PlanningChange) async {
        await save(tab: "planning") { bridge, revision in try await bridge.savePlanning(change(revision)) }
    }

    // MARK: Calendar source

    /// What the Source picker shows: this Mac's own macOS Calendar choice,
    /// which counts over everything else, and otherwise what config.toml names.
    var calendarChoice: CalendarChoice {
        if store?.macCalendar.source == .mac { return .mac }
        return snapshot?.calendar.source == "google" ? .google : .files
    }

    /// Chooses where this Mac reads its meetings from. macOS Calendar is this
    /// Mac's alone. Google Calendar and calendar files are config.toml's
    /// `[calendar] source`, and choosing either clears a macOS Calendar choice,
    /// which would count over it (ADR 0003, D9).
    func chooseCalendarSource(_ choice: CalendarChoice) async {
        guard let store, choice != calendarChoice else { return }
        if choice == .mac {
            await store.macCalendar.choose(.mac)
            return
        }
        if store.macCalendar.source == .mac { await store.macCalendar.choose(.files) }
        let named = choice == .google ? "google" : "files"
        guard snapshot?.calendar.source != named else { return }
        await saveCalendar { CalendarChange(revision: $0, source: named) }
    }

    // MARK: Calendar files

    func setCalendarEnabled(_ enabled: Bool) async {
        await saveCalendar { CalendarChange(revision: $0, enabled: enabled) }
    }

    /// Adds files to the list. The bridge keeps a path under the home folder
    /// as ~/..., and a file already listed once.
    func addCalendarFiles(_ paths: [String]) async {
        guard let snapshot else { return }
        let all = snapshot.calendar.files.map(\.path) + paths
        await saveCalendar { CalendarChange(revision: $0, paths: all) }
    }

    func removeCalendarFile(_ path: String) async {
        guard let snapshot else { return }
        let rest = snapshot.calendar.files.map(\.path).filter { $0 != path }
        await saveCalendar { CalendarChange(revision: $0, paths: rest) }
    }

    /// Sets when the working day starts or ends, as HH:MM.
    func setWorkingStart(_ time: String) async {
        await saveCalendar { CalendarChange(revision: $0, workingStart: time) }
    }

    func setWorkingEnd(_ time: String) async {
        await saveCalendar { CalendarChange(revision: $0, workingEnd: time) }
    }

    func setIncludeAllDay(_ include: Bool) async {
        await saveCalendar { CalendarChange(revision: $0, includeAllDay: include) }
    }

    func setIncludeTransparent(_ include: Bool) async {
        await saveCalendar { CalendarChange(revision: $0, includeTransparent: include) }
    }

    /// Sets how long a calendar file may go unchanged before it is stale.
    func setStaleAfter(_ text: String) async {
        await saveCalendar { CalendarChange(revision: $0, staleAfter: text) }
    }

    private func saveCalendar(_ change: @escaping (String) -> CalendarChange) async {
        await save(tab: "calendar") { bridge, revision in try await bridge.saveCalendar(change(revision)) }
    }

    // MARK: Saving

    private func save(tab: String, _ write: (any TaskBridge, String) async throws -> SettingsSnapshot) async {
        guard let store, let revision = snapshot?.revision else { return }
        notice = nil
        do {
            snapshot = try await write(store.bridge, revision)
            errors = errors.filter { !$0.key.hasPrefix("\(tab).") }
        } catch let failure as BridgeError {
            if let field = failure.field {
                errors["\(tab).\(field)"] = failure.message
            } else if failure.code == "configConflict" {
                notice = failure.message
                errors = [:]
                reloads += 1
                await reload()
            } else {
                notice = failure.message
            }
            return
        } catch {
            notice = error.localizedDescription
            return
        }
        // The main window follows: the status carries the refresh interval and
        // the undo delay, and the next load the plan's meter and whether Plan
        // Today is offered.
        await store.refreshStatus()
        if store.account != nil {
            await store.refresh()
        }
    }
}
