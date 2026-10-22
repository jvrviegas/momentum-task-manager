import Foundation

/// The Plan Today sheet while it is open: the plan the bridge built and the
/// ticks made since. Only the ticked estimates are summed here, as the TUI
/// does; the planning rules stay in Go.
struct PlanSheet: Identifiable, Equatable {
    /// Why the last save stopped. After a partial save, reason is the
    /// server's answer.
    struct Notice: Equatable {
        var message: String
        var reason: String?
    }

    let plan: DailyPlan
    private(set) var selected: Set<String>
    var saving = false
    var notice: Notice?

    var id: String { plan.date }

    init(plan: DailyPlan) {
        self.plan = plan
        selected = Set((plan.obligations + plan.rollover + plan.candidates).filter(\.selected).map(\.id))
    }

    func isSelected(_ item: PlanItem) -> Bool {
        selected.contains(item.id)
    }

    func isFixed(_ item: PlanItem) -> Bool {
        plan.obligations.contains { $0.id == item.id }
    }

    /// Ticks or unticks a rollover task or a candidate. Obligations are fixed.
    mutating func toggle(_ item: PlanItem) {
        guard !isFixed(item) else { return }
        if selected.remove(item.id) == nil {
            selected.insert(item.id)
        }
    }

    /// Every selected task, obligations included, as applyPlan takes them.
    var selectedUUIDs: [String] {
        (plan.obligations + plan.rollover + plan.candidates).filter(isSelected).map(\.id)
    }

    /// The ticked rollover tasks and candidates.
    var chosen: [PlanItem] {
        (plan.rollover + plan.candidates).filter(isSelected)
    }

    var chosenMinutes: Int { minutes(chosen) }
    var unestimatedChosen: Int { chosen.count { $0.task.estimateMinutes == nil } }
    var fixedMinutes: Int { minutes(plan.obligations) }
    var unestimatedFixed: Int { plan.obligations.count { $0.task.estimateMinutes == nil } }

    /// What is left for chosen work after obligations and the buffer.
    var availableMinutes: Int {
        plan.capacityMinutes - fixedMinutes - plan.bufferMinutes
    }

    /// Below zero when the chosen work is over capacity, which warns but
    /// never blocks.
    var remainingMinutes: Int { availableMinutes - chosenMinutes }
    var isOverCapacity: Bool { remainingMinutes < 0 }

    private func minutes(_ items: [PlanItem]) -> Int {
        items.compactMap(\.task.estimateMinutes).reduce(0, +)
    }
}
