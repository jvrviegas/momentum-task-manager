import Foundation
import SwiftUI
import Testing
@testable import Momentum

func planItem(_ name: String, minutes: Int? = nil, selected: Bool = false, plannedDate: String? = nil) -> PlanItem {
    var task = TaskItem.fixture(name)
    task.estimateMinutes = minutes
    return PlanItem(task: task, selected: selected, plannedDate: plannedDate)
}

/// 8h capacity and a 1h buffer; obligations of 30m and unknown time;
/// rollover r (45m) and candidates a (5h), b (unknown) and c (2h).
func dailyPlan() -> DailyPlan {
    DailyPlan(
        date: "2026-10-14", tag: "momentum-plan-2026-10-14", capacityMinutes: 480, bufferMinutes: 60,
        obligations: [planItem("overdue", minutes: 30), planItem("due")],
        rollover: [planItem("r", minutes: 45, selected: true, plannedDate: "2026-10-13")],
        candidates: [planItem("a", minutes: 300), planItem("b"), planItem("c", minutes: 120)])
}

struct PlanSheetTests {
    @Test func tickingSumsTheChosenEstimatesAndWarnsOverCapacity() {
        var sheet = PlanSheet(plan: dailyPlan())
        #expect(sheet.fixedMinutes == 30 && sheet.unestimatedFixed == 1 && sheet.availableMinutes == 390)
        #expect(sheet.chosenMinutes == 45 && sheet.remainingMinutes == 345 && !sheet.isOverCapacity)

        let (a, b, c) = (sheet.plan.candidates[0], sheet.plan.candidates[1], sheet.plan.candidates[2])
        sheet.toggle(a)
        sheet.toggle(b)
        #expect(sheet.chosenMinutes == 345 && sheet.unestimatedChosen == 1 && !sheet.isOverCapacity)
        sheet.toggle(c)
        #expect(sheet.chosenMinutes == 465 && sheet.remainingMinutes == -75 && sheet.isOverCapacity)
        sheet.toggle(sheet.plan.rollover[0])
        #expect(sheet.chosenMinutes == 420 && sheet.remainingMinutes == -30)
        #expect(sheet.selectedUUIDs == ["a", "b", "c"])
    }

    @Test func obligationsCannotBeUnticked() {
        var plan = dailyPlan()
        plan.obligations[0].selected = true
        var sheet = PlanSheet(plan: plan)
        sheet.toggle(plan.obligations[0])
        sheet.toggle(plan.obligations[1])
        #expect(sheet.selectedUUIDs == ["overdue", "r"])
    }
}

@MainActor
struct PlanStoreTests {
    private func openedStore(results: [PlanSaveResult]) async -> (AppStore, FakeBridge) {
        let a = TaskItem.fixture("a")
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [a])])
        await bridge.setPlan(dailyPlan(), results: results)
        let store = signedInStore(bridge)
        await store.start()
        await store.openPlan()
        return (store, bridge)
    }

    @Test func confirmingClosesTheSheetAndOffersNoUndo() async {
        let (store, bridge) = await openedStore(results: [PlanSaveResult(applied: 2, total: 2)])
        store.cancelPlan()
        await store.toggleStart(.fixture("a"))
        #expect(store.undoWindow.isAvailable(at: .now))

        await store.openPlan()
        #expect(store.hasOverlay)
        store.planSheet?.toggle(dailyPlan().candidates[0])
        await store.confirmPlan()
        #expect(store.planSheet == nil)
        #expect(store.toast?.message == "Daily plan saved · 2 tasks" && store.toast?.offersUndo == false)
        #expect(!store.undoWindow.isAvailable(at: .now))
        let calls = await bridge.calls
        #expect(calls.suffix(2) == ["applyPlan 2026-10-14 r,a", "load"])
    }

    @Test func aPartialSaveKeepsTheSheetAndItsTicks() async {
        let (store, bridge) = await openedStore(results: [
            PlanSaveResult(applied: 1, total: 3, error: "the server refused the change"), PlanSaveResult(applied: 2, total: 2),
        ])
        store.planSheet?.toggle(dailyPlan().candidates[0])
        await store.confirmPlan()
        #expect(store.planSheet?.selectedUUIDs == ["r", "a"] && store.planSheet?.saving == false)
        #expect(store.planSheet?.notice == PlanSheet.Notice(
            message: "Saved 1 of 3 changes. Confirm again to finish — nothing is applied twice.",
            reason: "the server refused the change"))
        #expect(await bridge.calls.suffix(2) == ["applyPlan 2026-10-14 r,a", "load"])

        await store.confirmPlan()
        #expect(store.planSheet == nil && store.toast?.message == "Daily plan saved · 2 tasks")
    }

    @Test func anUnchangedPlanSaysSo() async {
        let (store, bridge) = await openedStore(results: [PlanSaveResult(applied: 0, total: 0)])
        await store.confirmPlan()
        #expect(store.planSheet == nil && store.toast?.message == "Daily plan unchanged")
        #expect(await bridge.calls.last == "applyPlan 2026-10-14 r")
    }

    @Test func aFailedSaveKeepsTheSheetOpen() async {
        let (store, bridge) = await openedStore(results: [])
        await bridge.setFailure(BridgeError(message: "Momentum can’t reach the server", code: "unreachable"))
        await store.confirmPlan()
        #expect(store.planSheet?.notice == PlanSheet.Notice(message: "Momentum can’t reach the server"))
        #expect(store.server == .unreachable)
    }

    @Test func planTodayWaitsForASignInAndRespectsPlanningOff() async {
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [.fixture("a")])])
        let signedOut = AppStore(bridge: bridge, signIns: FakeSignInStore())
        await signedOut.start()
        await signedOut.openPlan()
        #expect(signedOut.planSheet == nil && signedOut.planningEnabled)

        var off = snapshot(inbox: [.fixture("a")])
        off.plan = PlanSummary(
            enabled: false, chosenCount: 0, chosenMinutes: 0, unestimatedChosen: 0, fixedMinutes: 0, unestimatedFixed: 0,
            capacityMinutes: 0, bufferMinutes: 0)
        let store = signedInStore(FakeBridge(snapshots: [off]))
        await store.start()
        await store.openPlan()
        #expect(!store.planningEnabled && store.planSheet == nil)
    }
}

@MainActor
struct PlanKeyCommandTests {
    private func openedStore() async -> (AppStore, FakeBridge) {
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [.fixture("a")])])
        await bridge.setPlan(dailyPlan(), results: [PlanSaveResult(applied: 1, total: 1)])
        let store = signedInStore(bridge)
        await store.start()
        await store.openPlan()
        return (store, bridge)
    }

    @discardableResult
    private func press(
        _ characters: String, _ key: KeyEquivalent? = nil, modifiers: EventModifiers = [], in store: AppStore,
        focusFilter: () -> Void = {}
    ) -> Bool {
        let result = PlanKeyCommands.handle(
            key: key ?? KeyEquivalent(Character(characters)), characters: characters, modifiers: modifiers, store: store,
            focusFilter: focusFilter)
        if case .handled = result { return true }
        return false
    }

    @Test func movementSkipsObligationsAndStopsAtTheEnds() async {
        let (store, _) = await openedStore()
        #expect(store.planSheet?.cursor == "r")
        press("k", in: store)
        #expect(store.planSheet?.cursor == "r")
        press("j", in: store)
        press("\u{F701}", .downArrow, modifiers: [.numericPad, .function], in: store)
        #expect(store.planSheet?.cursor == "b")
        press("G", modifiers: .shift, in: store)
        press("j", in: store)
        #expect(store.planSheet?.cursor == "c")
        press("\u{F700}", .upArrow, modifiers: [.numericPad, .function], in: store)
        #expect(store.planSheet?.cursor == "b")
        press("g", in: store)
        #expect(store.planSheet?.cursor == "r")
    }

    @Test func spaceTicksTheRowUnderTheCursor() async {
        let (store, _) = await openedStore()
        press(" ", .space, in: store)
        #expect(store.planSheet?.selectedUUIDs == [])
        press("j", in: store)
        press(" ", .space, in: store)
        #expect(store.planSheet?.selectedUUIDs == ["a"])
    }

    @Test func slashAndCommandFFocusTheFilter() async {
        let (store, _) = await openedStore()
        var focused = 0
        press("/", in: store, focusFilter: { focused += 1 })
        press("f", modifiers: .command, in: store, focusFilter: { focused += 1 })
        #expect(focused == 2)
        #expect(!press("n", modifiers: .command, in: store))
    }

    @Test func theFilterNarrowsTheCandidatesAndKeepsTheirTicks() async {
        let (store, _) = await openedStore()
        store.planSheet?.toggle(dailyPlan().candidates[2])
        store.planSheet?.filter = "b"
        #expect(store.planSheet?.visibleCandidates.map(\.id) == ["b"])
        #expect(store.planSheet?.choosable.map(\.id) == ["r", "b"])
        press("G", modifiers: .shift, in: store)
        #expect(store.planSheet?.cursor == "b")
        store.planSheet?.filter = "zzz"
        #expect(store.planSheet?.cursor == "r")
        #expect(store.planSheet?.selectedUUIDs == ["r", "c"])
    }

    @Test func escapeCancelsAndReturnConfirms() async throws {
        let (store, bridge) = await openedStore()
        press("\u{1b}", .escape, in: store)
        #expect(store.planSheet == nil)
        #expect(!press("j", in: store))

        await store.openPlan()
        press("\r", .return, in: store)
        for _ in 0..<100 where store.planSheet != nil {
            try await Task.sleep(for: .milliseconds(10))
        }
        #expect(store.planSheet == nil)
        #expect(await bridge.calls.contains("applyPlan 2026-10-14 r"))
    }

    @Test func listKeysWaitWhileThePlanIsOpen() async {
        let (store, _) = await openedStore()
        let result = TaskKeyCommands.handle(key: "c", characters: "c", modifiers: [], store: store, focusSearch: {})
        guard case .ignored = result else {
            Issue.record("c opened capture over the plan")
            return
        }
        #expect(!store.showCapture)
    }
}

struct PlanFormatTests {
    private let now = Date(timeIntervalSince1970: 1_792_000_000) // Wed 14 Oct 2026, 17:46 UTC

    private func item(todayGroup: String? = nil, due: Date? = nil, plannedDate: String? = nil) -> PlanItem {
        var task = TaskItem.fixture("t", due: due)
        task.todayGroup = todayGroup
        return PlanItem(task: task, selected: false, plannedDate: plannedDate)
    }

    @Test func reasonsSayWhyATaskIsInItsGroup() throws {
        let calendar = Calendar.current
        let today = calendar.startOfDay(for: now)
        let twoDaysAgo = try #require(calendar.date(byAdding: .day, value: -2, to: today))
        let threePM = try #require(calendar.date(bySettingHour: 15, minute: 0, second: 0, of: today))
        let yesterday = try #require(calendar.date(byAdding: .day, value: -1, to: today))
        let day = DateFormatter()
        day.dateFormat = "yyyy-MM-dd"

        #expect(Format.planReason(item(todayGroup: "Overdue", due: twoDaysAgo.addingTimeInterval(9 * 3600)), now: now)?.text == "Overdue 2d")
        #expect(Format.planReason(item(todayGroup: "Overdue", due: twoDaysAgo), now: now)?.tone == .overdue)
        #expect(Format.planReason(item(todayGroup: "Due Today", due: threePM), now: now)?.text == "Due today \(Format.time(threePM))")
        #expect(Format.planReason(item(todayGroup: "Due Today", due: today), now: now)?.text == "Due today")
        #expect(Format.planReason(item(plannedDate: day.string(from: yesterday)), now: now)?.text == "Planned yesterday")
        #expect(Format.planReason(item(plannedDate: day.string(from: twoDaysAgo)), now: now)?.text == "Planned 2 days ago")
        #expect(Format.planReason(item(), now: now) == nil)
    }

    @Test func minutesReadAsTheTUIShowsThem() {
        #expect(Format.minutes(45) == "45m")
        #expect(Format.minutes(120) == "2h")
        #expect(Format.minutes(160) == "2h 40m")
        #expect(Format.minutes(-75) == "1h 15m")
    }
}

@MainActor
struct PlanEntryTests {
    private func pressP(in store: AppStore) -> Bool {
        let result = TaskKeyCommands.handle(key: "P", characters: "P", modifiers: .shift, store: store, focusSearch: {})
        if case .handled = result { return true }
        return false
    }

    @Test func shiftPOpensPlanTodayFromTheList() async throws {
        let bridge = FakeBridge(snapshots: [snapshot(inbox: [.fixture("a")])])
        await bridge.setPlan(dailyPlan(), results: [])
        let store = signedInStore(bridge)
        await store.start()
        #expect(pressP(in: store))
        for _ in 0..<100 where store.planSheet == nil {
            try await Task.sleep(for: .milliseconds(10))
        }
        #expect(store.planSheet?.plan == dailyPlan())
    }

    @Test func shiftPIsNoShortcutWithPlanningOff() async {
        var off = snapshot(inbox: [.fixture("a")])
        off.plan = PlanSummary(
            enabled: false, chosenCount: 0, chosenMinutes: 0, unestimatedChosen: 0, fixedMinutes: 0, unestimatedFixed: 0,
            capacityMinutes: 0, bufferMinutes: 0)
        let bridge = FakeBridge(snapshots: [off])
        let store = signedInStore(bridge)
        await store.start()
        #expect(!pressP(in: store))
        #expect(await !bridge.calls.contains("plan"))
    }
}

struct PlanCalendarTests {
    @Test func calendarTimeComesOffWhatIsAvailable() {
        var plan = dailyPlan()
        plan.calendar = PlanCalendar(
            enabled: true, available: true, stale: false, error: nil, busyMinutes: 120, eventCount: 3,
            workingStart: "09:00", workingEnd: "17:00", blocks: [])
        let sheet = PlanSheet(plan: plan)
        #expect(sheet.calendarMinutes == 120 && sheet.availableMinutes == 270 && sheet.remainingMinutes == 225)
    }

    @Test func decodesThePlanItsCalendarAndTheSummary() throws {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let plan = try decoder.decode(DailyPlan.self, from: Data("""
        {"date":"2026-10-14","tag":"momentum-plan-2026-10-14","capacityMinutes":480,"bufferMinutes":60,"obligations":[],
        "rollover":[{"task":{"uuid":"r","id":0,"description":"Write ADR","status":"pending","estimateMinutes":45,"tags":["momentum-plan-2026-10-13"],
        "annotations":[],"depends":[],"urgency":6,"edit":{"description":"Write ADR","project":"","priority":"","due":"","scheduled":"",
        "tags":"momentum-plan-2026-10-13","estimate":"45m","recurrence":""}},"selected":true,"plannedDate":"2026-10-13"}],"candidates":[],
        "calendar":{"enabled":true,"available":true,"stale":true,"busyMinutes":120,"eventCount":5,"workingStart":"09:00","workingEnd":"17:00",
        "blocks":[{"start":"2026-10-14T08:30:00Z","end":"2026-10-14T08:45:00Z","titles":["Team stand-up"]}]}}
        """.utf8))
        #expect(plan.rollover.first?.plannedDate == "2026-10-13" && plan.rollover.first?.task.estimateMinutes == 45)
        #expect(plan.calendar?.stale == true && plan.calendar?.blocks.first?.titles == ["Team stand-up"])
        #expect(plan.calendar?.blocks.first?.start == Date(timeIntervalSince1970: 1_791_966_600))

        let summary = try decoder.decode(PlanSummary.self, from: Data("""
        {"enabled":true,"chosenCount":2,"chosenMinutes":60,"unestimatedChosen":1,"fixedMinutes":50,"unestimatedFixed":1,
        "capacityMinutes":360,"bufferMinutes":60,"calendarMinutes":0,"calendarAvailable":false,"calendarError":"read calendar gone.ics: no such file"}
        """.utf8))
        #expect(summary.calendarError == "read calendar gone.ics: no such file" && !summary.calendarAvailable)
    }
}
