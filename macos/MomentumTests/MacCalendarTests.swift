import Foundation
import Testing
@testable import Momentum

/// The Mac's Calendar, with a chosen answer to the permission question and
/// events kept by calendar.
@MainActor
final class FakeCalendarSource: CalendarEventSource {
    var access: CalendarAccess
    /// What the Mac answers when asked.
    var answer: CalendarAccess
    private(set) var asked = 0
    let all = [
        MacCalendarInfo(id: "home", title: "Home", account: "iCloud"),
        MacCalendarInfo(id: "work", title: "Work", account: "Google"),
        MacCalendarInfo(id: "team", title: "Team", account: "Google"),
    ]
    var events: [String: [CalendarEvent]] = [:]

    init(access: CalendarAccess, answer: CalendarAccess = .granted) {
        self.access = access
        self.answer = answer
    }

    func requestAccess() async -> CalendarAccess {
        asked += 1
        access = answer
        return access
    }

    func calendars() -> [MacCalendarInfo] { all }
    func events(on day: Date, in calendarIDs: Set<String>) -> [CalendarEvent] { calendarIDs.sorted().flatMap { events[$0] ?? [] } }
}

private let nine = Date(timeIntervalSince1970: 1_792_000_000)

private func event(_ title: String, hours: Double = 0) -> CalendarEvent {
    CalendarEvent(start: nine.addingTimeInterval(hours * 3600), end: nine.addingTimeInterval((hours + 1) * 3600), allDay: false, free: false, title: title)
}

@MainActor
struct MacCalendarTests {
    private func makeStore(access: CalendarAccess, answer: CalendarAccess = .granted) async -> (AppStore, FakeBridge, FakeCalendarSource, UserDefaults) {
        let bridge = FakeBridge(snapshots: [snapshot(today: [.fixture("a")])])
        let source = FakeCalendarSource(access: access, answer: answer)
        source.events = ["home": [event("Design review")], "work": [event("Stand-up", hours: 1)], "team": [event("Retro", hours: 2)]]
        let defaults = UserDefaults(suiteName: "momentum-tests-\(UUID().uuidString)")!
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_test")
        let store = AppStore(bridge: bridge, signIns: signIns, calendarSource: source, calendarDefaults: defaults)
        await store.start()
        await store.settings.reload()
        return (store, bridge, source, defaults)
    }

    @Test func theFilesSourceHandsNothingToTheBridge() async {
        let (store, bridge, _, _) = await makeStore(access: .granted)
        #expect(store.macCalendar.source == .files)
        #expect(await bridge.loadCalendars == [nil])
        await store.openPlan()
        #expect(await bridge.planCalendars == [nil])
    }

    @Test func choosingMacOSCalendarAsksTheMacOnceAndLoadsAgain() async {
        let (store, bridge, source, defaults) = await makeStore(access: .notDetermined)
        await store.macCalendar.choose(.mac)
        #expect(source.asked == 1)
        #expect(store.macCalendar.access == .granted)
        #expect(store.macCalendar.accounts.map(\.name) == ["iCloud", "Google"])
        #expect(defaults.string(forKey: MacCalendarModel.sourceKey) == "mac")
        let load = await bridge.loadCalendars.last
        #expect(load??.error == nil && load??.events.count == 3, "the next load uses the Mac's events")

        await store.macCalendar.choose(.files)
        await store.macCalendar.choose(.mac)
        #expect(source.asked == 1, "the Mac is asked the first time only")
    }

    @Test func aMacThatSaysNoLeavesNoAccessForLoadAndPlan() async {
        let (store, bridge, _, _) = await makeStore(access: .notDetermined, answer: .denied)
        await store.macCalendar.choose(.mac)
        #expect(store.macCalendar.access == .denied && store.macCalendar.calendars.isEmpty)
        let none = CalendarInput(error: "no access", events: [])
        #expect(await bridge.loadCalendars.last == none)
        await store.openPlan()
        #expect(await bridge.planCalendars.last == none)
    }

    @Test func titlesGoOnlyWithAPlan() async {
        let (store, bridge, _, _) = await makeStore(access: .granted)
        await store.macCalendar.choose(.mac)
        let load = await bridge.loadCalendars.last
        #expect(load??.events.map(\.title) == [nil, nil, nil], "a load needs no titles")
        await store.openPlan()
        let plan = await bridge.planCalendars.last
        #expect(plan??.events.compactMap(\.title) == ["Design review", "Retro", "Stand-up"])
    }

    @Test func aCalendarSwitchedOffSendsNoEventsAndIsRemembered() async {
        let (store, _, source, defaults) = await makeStore(access: .granted)
        await store.macCalendar.choose(.mac)
        await store.macCalendar.setCalendar("work", on: false)
        #expect(store.macCalendar.input(titles: true)?.events.compactMap(\.title) == ["Design review", "Retro"])

        let later = MacCalendarModel(eventSource: source, defaults: defaults)
        #expect(later.source == .mac && later.off == ["work"], "the choices stay on this Mac")
        #expect(later.input(titles: false)?.events.count == 2)

        await store.macCalendar.setCalendar("work", on: true)
        #expect(store.macCalendar.input(titles: false)?.events.count == 3)
    }

    @Test func choosingMacOSCalendarNeverChangesTheConfigFile() async {
        let (store, bridge, _, _) = await makeStore(access: .notDetermined)
        let before = store.settings.snapshot?.calendar
        await store.macCalendar.choose(.mac)
        await store.macCalendar.setCalendar("home", on: false)
        await store.macCalendar.choose(.files)
        #expect(await bridge.saves.isEmpty, "no [calendar] key is written")
        #expect(store.settings.snapshot?.calendar == before)
    }

    @Test func comingBackFromSystemSettingsWithAccessLoadsAgain() async {
        let (store, bridge, source, _) = await makeStore(access: .denied)
        await store.macCalendar.choose(.mac)
        #expect(await bridge.loadCalendars.last??.error == "no access")

        source.access = .granted
        await store.macCalendar.refresh()
        #expect(store.macCalendar.access == .granted && store.macCalendar.calendars.count == 3)
        let load = await bridge.loadCalendars.last
        #expect(load??.error == nil && load??.events.count == 3)
    }

    @Test func eventTimesAreSentAsISO8601() throws {
        let input = CalendarInput(error: nil, events: [CalendarEvent(start: nine, end: nine.addingTimeInterval(1800), allDay: false, free: true, title: nil)])
        let json = String(decoding: try ProcessBridge.encoder.encode(input), as: UTF8.self)
        #expect(json.contains("\"start\":\"2026-10-14T17:46:40Z\"") && json.contains("\"end\":\"2026-10-14T18:16:40Z\""))
        #expect(json.contains("\"free\":true") && !json.contains("title") && !json.contains("error"))
    }
}
