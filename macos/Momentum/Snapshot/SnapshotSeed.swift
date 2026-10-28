#if DEBUG
import Foundation

/// The tasks every snapshot page shows. Dates are relative to the moment of
/// the run, so "Due Today" and "Yesterday" stay true, and every value is one
/// a person can enter in the app.
struct SnapshotSeed {
    var tasks: [TaskItem]
    var today: [TaskSection]
    var inbox: [TaskSection]
    var completed: [TaskSection]
    var projects: [ProjectEntry]
    var plan: PlanSummary
    var dailyPlan: DailyPlan

    static let projects = [
        ProjectEntry(value: "work", label: "Work"),
        ProjectEntry(value: "work.client", label: "Clients"),
        ProjectEntry(value: "work.client.acme", label: "Acme Corp"),
        ProjectEntry(value: "ops.infra", label: "Infrastructure"),
        ProjectEntry(value: "personal.home", label: "Home"),
        ProjectEntry(value: "personal.health", label: "Health"),
    ]

    static var empty: SnapshotSeed {
        let seed = SnapshotSeed.make(now: .now)
        return SnapshotSeed(
            tasks: [], today: [], inbox: [], completed: [], projects: projects,
            plan: PlanSummary(
                enabled: true, chosenCount: 0, chosenMinutes: 0, unestimatedChosen: 0, fixedMinutes: 0, unestimatedFixed: 0,
                capacityMinutes: 480, bufferMinutes: 60),
            dailyPlan: DailyPlan(
                date: seed.dailyPlan.date, tag: seed.dailyPlan.tag, capacityMinutes: 480, bufferMinutes: 60,
                obligations: [], rollover: [], candidates: []))
    }

    private static let calendar = Calendar.current

    /// A moment `days` from today at `hour:minute`.
    private static func at(_ days: Int, _ hour: Int, _ minute: Int = 0, from now: Date) -> Date {
        let day = calendar.date(byAdding: .day, value: days, to: calendar.startOfDay(for: now))!
        return calendar.date(bySettingHour: hour, minute: minute, second: 0, of: day)!
    }

    private static func text(_ date: Date?) -> String {
        date.map { format("yyyy-MM-dd HH:mm").string(from: $0) } ?? ""
    }

    private static func format(_ pattern: String) -> DateFormatter {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.calendar = calendar
        formatter.dateFormat = pattern
        return formatter
    }

    private static func minutes(_ value: Int?) -> String? {
        guard let value else { return nil }
        return value % 60 == 0 ? "\(value / 60)h" : value > 60 ? "\(value / 60)h \(value % 60)m" : "\(value)m"
    }

    static func make(now: Date) -> SnapshotSeed {
        var number = 0
        func task(
            _ description: String, project: String? = nil, priority: String? = nil, estimate: Int? = nil, due: Date? = nil,
            scheduled: Date? = nil, start: Date? = nil, tags: [String] = [], recur: String? = nil, recurDisplay: String? = nil,
            notes: [String] = [], depends: [String] = [], group: String? = nil, urgency: Double = 5, completed: Date? = nil
        ) -> TaskItem {
            number += 1
            let uuid = "00000000-0000-4000-8000-\(String(format: "%012d", number))"
            let recurring = recur != nil
            return TaskItem(
                uuid: uuid, number: number, description: description, status: completed == nil ? "pending" : "completed",
                project: project, priority: priority, estimateMinutes: estimate, estimate: minutes(estimate),
                entry: at(-9, 9, from: now), end: completed, modified: at(-1, 11, from: now), due: due, scheduled: scheduled, start: start,
                tags: tags, annotations: notes.map { Annotation(description: $0, entry: at(-2, 14, from: now)) }, depends: depends,
                recur: recur, recurDisplay: recurDisplay, parent: recurring ? "00000000-0000-4000-9000-000000000001" : nil,
                recurrenceTarget: recurring ? "00000000-0000-4000-9000-000000000001" : nil, urgency: urgency, todayGroup: group,
                edit: EditFields(
                    description: description, project: project ?? "", priority: priority ?? "", due: text(due),
                    scheduled: text(scheduled), tags: tags.joined(separator: " "), estimate: minutes(estimate) ?? "",
                    recurrence: recur ?? ""))
        }

        let tls = task("Renew the TLS certificate for the status page", project: "ops.infra", priority: "H", estimate: 45,
                       due: at(-3, 10, from: now), group: "Overdue", urgency: 14.2)
        let invoice = task("Send Q3 invoice to Acme Corp", project: "work.client.acme", priority: "H", estimate: 30,
                           due: at(-1, 17, from: now), notes: ["Ask finance for the PO number first."], group: "Overdue", urgency: 13.1)
        let proposal = task("Prepare the Northwind proposal", project: "work.client", priority: "M", estimate: 60,
                            due: at(0, 17, from: now), start: at(0, 9, 30, from: now), tags: ["pitch"], group: "Due Today", urgency: 11.4)
        let review = task("Review pull request #482", project: "work", priority: "L", estimate: 20, due: at(0, 15, from: now),
                          group: "Due Today", urgency: 8.9)
        let parking = task("Pay the parking fine", project: "personal.home", due: at(0, 18, from: now), group: "Due Today", urgency: 8.1)
        let dentist = task("Call the dentist to move the appointment", project: "personal.health", estimate: 10,
                           scheduled: at(0, 9, 30, from: now), group: "Scheduled Today", urgency: 6.3)
        let tag = "momentum-plan-\(planDay(now))"
        let notes = task("Write the Q4 planning notes", project: "work", estimate: 90, tags: [tag], group: "Planned Today", urgency: 5.2)

        let flights = task("Book flights to Lisbon", project: "personal.home", priority: "M", estimate: 25, urgency: 6.8)
        let tap = task("Fix the leaking kitchen tap", project: "personal.home", estimate: 60, tags: ["weekend"], urgency: 3.1)
        let migrate = task("Migrate the staging database to the new cluster", project: "ops.infra", priority: "H", estimate: 120,
                           due: at(6, 12, from: now), depends: [tls.uuid], urgency: 9.4)
        let contract = task("Read the Northwind contract draft", project: "work.client", estimate: 45, urgency: 4.7)
        let chapter = task("Read the Swift concurrency chapter", estimate: 50, tags: ["reading"], urgency: 2.2)
        let plants = task("Water the plants", project: "personal.home", estimate: 10, due: at(2, 18, from: now), recur: "weekly",
                          recurDisplay: "Weekly", urgency: 3.5)
        let long = task("Draft the onboarding checklist for new engineers, covering laptop setup, access requests and the first-week schedule",
                        project: "work", priority: "L", urgency: 1.9)
        let weird = task("Sketch the new logo", project: "work", urgency: 1.4)
        let rolled = task("Draft the retrospective agenda", project: "work", estimate: 45, tags: ["momentum-plan-\(planDay(at(-1, 9, from: now)))"],
                          urgency: 4.1)

        let done1 = task("Back up the photo library", project: "personal.home", estimate: 15, completed: at(0, 8, 40, from: now))
        let done2 = task("Reply to the Northwind kickoff email", project: "work.client", completed: at(0, 8, 5, from: now))
        let done3 = task("Submit the expense report", project: "work", estimate: 20, completed: at(-1, 16, 50, from: now))
        let done4 = task("Order printer paper", completed: at(-4, 11, 10, from: now))

        let pending = [tls, invoice, proposal, review, parking, dentist, notes, flights, tap, migrate, contract, chapter, plants, long, weird, rolled]
        let finished = [done1, done2, done3, done4]
        let all = pending + finished

        let today = [
            TaskSection(title: "Overdue", project: nil, uuids: [tls, invoice].map(\.uuid)),
            TaskSection(title: "Due Today", project: nil, uuids: [proposal, review, parking].map(\.uuid)),
            TaskSection(title: "Scheduled Today", project: nil, uuids: [dentist].map(\.uuid)),
            TaskSection(title: "Planned Today", project: nil, uuids: [notes].map(\.uuid)),
        ]
        let inbox = [
            TaskSection(title: "Infrastructure", project: "ops.infra", uuids: [migrate].map(\.uuid)),
            TaskSection(title: "Home", project: "personal.home", uuids: [flights, plants, tap].map(\.uuid)),
            TaskSection(title: "Clients", project: "work.client", uuids: [contract].map(\.uuid)),
            TaskSection(title: "Work", project: "work", uuids: [rolled, long, weird].map(\.uuid)),
            TaskSection(title: "No project", project: nil, uuids: [chapter].map(\.uuid)),
        ]
        let completed = [
            TaskSection(title: "Today", project: nil, uuids: [done1, done2].map(\.uuid)),
            TaskSection(title: "Yesterday", project: nil, uuids: [done3].map(\.uuid)),
            TaskSection(title: "Earlier", project: nil, uuids: [done4].map(\.uuid)),
        ]

        let obligations = [tls, invoice, proposal, review, parking, dentist]
        let obligationMinutes = obligations.compactMap(\.estimateMinutes).reduce(0, +)
        let calendar = PlanCalendar(
            enabled: true, available: true, stale: false, error: nil, busyMinutes: 150, eventCount: 3, workingStart: "09:00",
            workingEnd: "17:00",
            blocks: [
                BusyBlock(start: at(0, 10, from: now), end: at(0, 11, from: now), titles: ["Team standup", "Design review"]),
                BusyBlock(start: at(0, 14, from: now), end: at(0, 15, 30, from: now), titles: ["Client call: Acme"]),
            ])
        let dailyPlan = DailyPlan(
            date: planDay(now), tag: tag, capacityMinutes: 480, bufferMinutes: 60,
            obligations: obligations.map { PlanItem(task: $0, selected: true, plannedDate: nil) },
            rollover: [PlanItem(task: rolled, selected: true, plannedDate: planDay(at(-1, 9, from: now)))],
            candidates: [flights, contract, migrate, tap, chapter, long].map { PlanItem(task: $0, selected: $0.uuid == flights.uuid, plannedDate: nil) }
                + [PlanItem(task: notes, selected: true, plannedDate: nil)],
            calendar: calendar)
        let summary = PlanSummary(
            enabled: true, chosenCount: 1, chosenMinutes: 90, unestimatedChosen: 0, fixedMinutes: obligationMinutes,
            unestimatedFixed: obligations.count { $0.estimateMinutes == nil }, capacityMinutes: 480, bufferMinutes: 60,
            calendarMinutes: 150, calendarAvailable: true)
        return SnapshotSeed(
            tasks: all, today: today, inbox: inbox, completed: completed, projects: projects, plan: summary, dailyPlan: dailyPlan)
    }

    /// The local day as YYYY-MM-DD, as the plan's tag spells it.
    static func planDay(_ date: Date) -> String {
        format("yyyy-MM-dd").string(from: date)
    }
}
#endif
