import Foundation
import Testing
@testable import Momentum

@MainActor
struct SettingsModelTests {
    private func planSummary(enabled: Bool) -> PlanSummary {
        PlanSummary(
            enabled: enabled, chosenCount: 0, chosenMinutes: 0, unestimatedChosen: 0, fixedMinutes: 0, unestimatedFixed: 0,
            capacityMinutes: enabled ? 480 : 0, bufferMinutes: enabled ? 60 : 0)
    }

    /// A signed-in store whose first load, at start, shows Plan Today on, and
    /// whose next loads, after each save, show it off and on again.
    private func storeWithAPlan() async -> (AppStore, FakeBridge) {
        var on = snapshot(today: [.fixture("a")]), off = snapshot(today: [.fixture("a")])
        on.plan = planSummary(enabled: true)
        off.plan = planSummary(enabled: false)
        let bridge = FakeBridge(snapshots: [on, off, on])
        let store = signedInStore(bridge)
        await store.start()
        await store.settings.reload()
        return (store, bridge)
    }

    @Test func theTabsShowWhatTheFileHas() async {
        let (store, bridge) = await storeWithAPlan()
        var value = await bridge.settingsValue
        value.planning.dailyCapacity = "7h30m"
        value.planning.weekdayCapacity = ["friday": "4h"]
        await bridge.setSettings(value)
        await store.settings.reload()
        #expect(store.settings.snapshot?.planning.dailyCapacity == "7h30m")
        #expect(store.settings.snapshot?.planning.weekdayCapacity["friday"] == "4h")
    }

    @Test func turningPlanningOffHidesPlanTodayAndOnBringsItBack() async {
        let (store, bridge) = await storeWithAPlan()
        #expect(store.planningEnabled)

        await store.settings.setPlanningEnabled(false)
        #expect(await bridge.saves == ["planning r1"], "the switch saves at once, on the revision it read")
        #expect(store.settings.snapshot?.planning.enabled == false)
        #expect(!store.planningEnabled, "the main window follows without a restart")

        await store.settings.setPlanningEnabled(true)
        #expect(store.planningEnabled)
    }

    @Test func aSaveWaitsForNoSignInButTheMainWindowOnlyReloadsWhenSignedIn() async {
        let bridge = FakeBridge(snapshots: [snapshot()])
        let signedOut = AppStore(bridge: bridge, signIns: FakeSignInStore())
        await signedOut.settings.reload()
        await signedOut.settings.setPlanningEnabled(false)
        #expect(await bridge.saves == ["planning r1"])
        #expect(!(await bridge.calls).contains("load"), "no tasks are asked for while signed out")
    }

    @Test func aRefusedLengthStaysOnItsFieldAndChangesNothing() async {
        let (store, bridge) = await storeWithAPlan()
        let message = "Enter a length of time, such as 8h or 7h30m."
        await bridge.setSaveFailure(BridgeError(message: message, field: "dailyCapacity"))

        await store.settings.setDailyCapacity("a day")
        #expect(store.settings.error("planning", "dailyCapacity") == message)
        #expect(store.settings.error("calendar", "dailyCapacity") == nil, "a message belongs to its tab")
        #expect(store.settings.snapshot?.planning.dailyCapacity == "8h")
        #expect(store.settings.notice == nil)

        store.settings.clearError("planning", "dailyCapacity")
        #expect(store.settings.error("planning", "dailyCapacity") == nil)

        await store.settings.setDailyCapacity("7h")
        #expect(store.settings.snapshot?.planning.dailyCapacity == "7h")
    }

    @Test func aGoodSaveClearsThatTabsMessagesOnly() async {
        let (store, bridge) = await storeWithAPlan()
        await bridge.setSaveFailure(BridgeError(message: "Add a calendar file first.", field: "enabled"))
        await store.settings.setCalendarEnabled(true)
        await bridge.setSaveFailure(BridgeError(message: "Buffer can’t be negative.", field: "buffer"))
        await store.settings.setBuffer("-1h")
        #expect(store.settings.error("calendar", "enabled") != nil && store.settings.error("planning", "buffer") != nil)

        await store.settings.setBuffer("30m")
        #expect(store.settings.error("planning", "buffer") == nil)
        #expect(store.settings.error("calendar", "enabled") != nil)
    }

    @Test func aFileThatChangedElsewhereReloadsTheTabAndSaysSo() async {
        let (store, bridge) = await storeWithAPlan()
        var elsewhere = await bridge.settingsValue
        elsewhere.planning.dailyCapacity = "6h"
        elsewhere.revision = "r2"
        await bridge.setSettings(elsewhere)
        await bridge.setSaveFailure(BridgeError(
            message: "config.toml changed outside Momentum. The tab now shows what it says; make the change again.", code: "configConflict"))

        await store.settings.setDailyCapacity("7h")
        #expect(store.settings.notice?.contains("changed outside Momentum") == true)
        #expect(store.settings.snapshot == elsewhere, "the tab shows what the file says now")
        #expect(store.settings.reloads == 1, "typed text goes back to the file's values")
        #expect(store.settings.errors.isEmpty)
    }

    @Test func aWriteThatFailedSaysWhy() async {
        let (store, bridge) = await storeWithAPlan()
        await bridge.setSaveFailure(BridgeError(message: "Unable to save config.toml. permission denied"))
        await store.settings.setPlanningEnabled(false)
        #expect(store.settings.notice == "Unable to save config.toml. permission denied")
        #expect(store.settings.snapshot?.planning.enabled == true, "the switch stays where it was")
    }

    @Test func calendarFilesAreAddedAndRemovedAsAWholeList() async {
        let (store, bridge) = await storeWithAPlan()
        await store.settings.addCalendarFiles(["~/Calendars/work.ics"])
        await store.settings.addCalendarFiles(["/Volumes/Shared/team.ics"])
        #expect(store.settings.snapshot?.calendar.files.map(\.path) == ["~/Calendars/work.ics", "/Volumes/Shared/team.ics"])

        await store.settings.removeCalendarFile("~/Calendars/work.ics")
        #expect(store.settings.snapshot?.calendar.files.map(\.path) == ["/Volumes/Shared/team.ics"])
        #expect(await bridge.saves.count == 3)
    }

    @Test func switchingCalendarFilesOnWithNoneIsRefusedOnTheSwitch() async {
        let (store, bridge) = await storeWithAPlan()
        await bridge.setSaveFailure(BridgeError(message: "Add a calendar file first.", field: "enabled"))
        await store.settings.setCalendarEnabled(true)
        #expect(store.settings.error("calendar", "enabled") == "Add a calendar file first.")
        #expect(store.settings.snapshot?.calendar.enabled == false, "the switch stays off")
    }

    @Test func workingHoursEventChoicesAndTheStaleLimitSaveAtOnce() async {
        let (store, bridge) = await storeWithAPlan()
        await store.settings.setWorkingStart("08:30")
        await store.settings.setWorkingEnd("16:00")
        await store.settings.setIncludeAllDay(true)
        await store.settings.setIncludeTransparent(true)
        await store.settings.setStaleAfter("90m")

        let calendar = store.settings.snapshot?.calendar
        #expect(calendar?.workingStart == "08:30" && calendar?.workingEnd == "16:00")
        #expect(calendar?.includeAllDay == true && calendar?.includeTransparent == true)
        #expect(calendar?.staleAfter == "90m")
        #expect(await bridge.saves.count == 5, "each change is its own save, on the revision it read")
        #expect(await bridge.calls.filter { $0 == "load" }.count >= 6, "the main window loads again after each save")
    }

    @Test func hoursThatEndBeforeTheyStartAreRefusedAndTheFieldsKeepTheFilesValues() async {
        let (store, bridge) = await storeWithAPlan()
        let message = "Working hours must end after they start."
        await bridge.setSaveFailure(BridgeError(message: message, field: "workingHours"))

        await store.settings.setWorkingEnd("08:00")
        #expect(store.settings.error("calendar", "workingHours") == message)
        #expect(store.settings.snapshot?.calendar.workingEnd == "17:00")
        #expect(store.settings.notice == nil)

        await store.settings.setWorkingEnd("18:00")
        #expect(store.settings.error("calendar", "workingHours") == nil)
        #expect(store.settings.snapshot?.calendar.workingEnd == "18:00")
    }

    @Test func aTimeFieldAndTheConfigTradeInHHMM() {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "Europe/Lisbon")!
        let day = calendar.date(from: DateComponents(year: 2026, month: 10, day: 14, hour: 12))!
        for text in ["09:00", "00:05", "13:30", "23:59"] {
            let date = SettingsModel.date(fromTime: text, calendar: calendar, on: day)
            #expect(SettingsModel.time(from: date, calendar: calendar) == text)
        }
        let nine = SettingsModel.date(fromTime: "09:00", calendar: calendar, on: day)
        #expect(calendar.component(.day, from: nine) == 14, "a time of day sits on the given day")
        #expect(SettingsModel.time(from: SettingsModel.date(fromTime: "nonsense", calendar: calendar, on: day), calendar: calendar) == "00:00")
    }

    @Test func fileStatusesDecodeWithTheirDates() throws {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let settings = try decoder.decode(SettingsSnapshot.self, from: Data("""
        {"configPath":"/tmp/config.toml","revision":"abc",
         "planning":{"enabled":true,"dailyCapacity":"8h","buffer":"1h","weekdayCapacity":{}},
         "calendar":{"source":"files","enabled":true,"workingStart":"09:00","workingEnd":"17:00","includeAllDay":false,
           "includeTransparent":false,"staleAfter":"24h","files":[
           {"path":"~/a.ics","status":"available","modified":"2026-10-14T09:00:00+01:00"},
           {"path":"~/b.ics","status":"missing"},
           {"path":"~/c","status":"error","detail":"is a directory"}]}}
        """.utf8))
        #expect(settings.calendar.files.map(\.status) == ["available", "missing", "error"])
        #expect(settings.calendar.files[0].modified != nil && settings.calendar.files[1].modified == nil)
        #expect(settings.calendar.files[2].detail == "is a directory")
    }

    @Test func theCalendarHoursAndEventChoicesDecode() throws {
        let settings = try JSONDecoder().decode(SettingsSnapshot.self, from: Data("""
        {"configPath":"/tmp/config.toml","revision":"abc",
         "planning":{"enabled":true,"dailyCapacity":"8h","buffer":"1h","weekdayCapacity":{}},
         "calendar":{"source":"google","enabled":false,"files":[],"workingStart":"08:30","workingEnd":"16:00",
                     "includeAllDay":true,"includeTransparent":false,"staleAfter":"0m"}}
        """.utf8))
        #expect(settings.calendar.source == "google")
        #expect(settings.calendar.workingStart == "08:30" && settings.calendar.workingEnd == "16:00")
        #expect(settings.calendar.includeAllDay && !settings.calendar.includeTransparent)
        #expect(settings.calendar.staleAfter == "0m")
    }
}
