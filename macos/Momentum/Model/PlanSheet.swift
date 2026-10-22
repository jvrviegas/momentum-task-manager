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
    /// The row the keyboard acts on: a rollover task or a visible candidate.
    private(set) var cursor: String?
    /// Narrows the candidates as the main search does. Hidden candidates
    /// keep their ticks.
    var filter = "" {
        didSet {
            if !choosable.contains(where: { $0.id == cursor }) {
                cursor = choosable.first?.id
            }
        }
    }

    var id: String { plan.date }

    init(plan: DailyPlan) {
        self.plan = plan
        selected = Set((plan.obligations + plan.rollover + plan.candidates).filter(\.selected).map(\.id))
        cursor = (plan.rollover + plan.candidates).first?.id
    }

    var visibleCandidates: [PlanItem] {
        let query = filter.trimmingCharacters(in: .whitespaces)
        return query.isEmpty ? plan.candidates : plan.candidates.filter { TaskSearch.matches($0.task, query: query) }
    }

    /// The rows the keyboard moves over; obligations are skipped.
    var choosable: [PlanItem] { plan.rollover + visibleCandidates }

    /// Moves the cursor, stopping at the first and last rows.
    mutating func move(by delta: Int) {
        let rows = choosable
        guard !rows.isEmpty else { return }
        let index = rows.firstIndex { $0.id == cursor } ?? -1
        cursor = rows[max(0, min(rows.count - 1, index + delta))].id
    }

    mutating func moveToFirst() { cursor = choosable.first?.id }
    mutating func moveToLast() { cursor = choosable.last?.id }

    /// Puts the cursor on a row the person clicked.
    mutating func point(at item: PlanItem) {
        if choosable.contains(where: { $0.id == item.id }) {
            cursor = item.id
        }
    }

    mutating func toggleCursor() {
        if let item = choosable.first(where: { $0.id == cursor }) {
            toggle(item)
        }
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

    /// The calendar's busy time; zero while it is off or unavailable.
    var calendarMinutes: Int { plan.calendar?.busyMinutes ?? 0 }

    /// What is left for chosen work after obligations, the calendar and the
    /// buffer.
    var availableMinutes: Int {
        plan.capacityMinutes - fixedMinutes - calendarMinutes - plan.bufferMinutes
    }

    /// Below zero when the chosen work is over capacity, which warns but
    /// never blocks.
    var remainingMinutes: Int { availableMinutes - chosenMinutes }
    var isOverCapacity: Bool { remainingMinutes < 0 }

    private func minutes(_ items: [PlanItem]) -> Int {
        items.compactMap(\.task.estimateMinutes).reduce(0, +)
    }
}
