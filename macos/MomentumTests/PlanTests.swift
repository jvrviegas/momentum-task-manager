import Foundation
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
