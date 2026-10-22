import SwiftUI

struct DateLabel: Equatable {
    enum Tone { case overdue, today, normal }
    var text: String
    var tone: Tone
    var scheduled: Bool
    var help: String
}

enum Format {
    /// The single most relevant date for a pending row: due first, then scheduled.
    static func relevantDate(for task: TaskItem, now: Date) -> DateLabel? {
        if let due = task.due {
            let diff = days(from: now, to: due)
            if diff < 0 {
                return DateLabel(text: diff == -1 ? "Yesterday" : "\(-diff) days overdue", tone: .overdue, scheduled: false, help: "Due \(full(due))")
            }
            if diff == 0 {
                return DateLabel(text: hasTime(due) ? "Due \(time(due))" : "Due today", tone: .today, scheduled: false, help: "Due \(full(due))")
            }
            return DateLabel(text: "Due \(friendly(due, now: now))", tone: .normal, scheduled: false, help: "Due \(full(due))")
        }
        if let scheduled = task.scheduled {
            let diff = days(from: now, to: scheduled)
            let text = diff == 0 ? "Starts today" : diff < 0 ? "Started \(short(scheduled))" : "Starts \(friendly(scheduled, now: now))"
            return DateLabel(text: text, tone: .normal, scheduled: true, help: "Scheduled \(full(scheduled))")
        }
        return nil
    }

    /// Calendar days from `now` to `date` in the local time zone.
    static func days(from now: Date, to date: Date) -> Int {
        let calendar = Calendar.current
        return calendar.dateComponents([.day], from: calendar.startOfDay(for: now), to: calendar.startOfDay(for: date)).day ?? 0
    }

    static func hasTime(_ date: Date) -> Bool {
        let parts = Calendar.current.dateComponents([.hour, .minute], from: date)
        return parts.hour != 0 || parts.minute != 0
    }

    static func time(_ date: Date) -> String {
        date.formatted(date: .omitted, time: .shortened)
    }

    static func short(_ date: Date) -> String {
        date.formatted(.dateTime.month(.abbreviated).day())
    }

    static func friendly(_ date: Date, now: Date) -> String {
        let diff = days(from: now, to: date)
        if diff == 1 { return "tomorrow" }
        if diff > 1 && diff < 7 { return date.formatted(.dateTime.weekday(.wide)) }
        return short(date)
    }

    static func full(_ date: Date) -> String {
        let day = date.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day().year())
        return hasTime(date) ? "\(day), \(time(date))" : day
    }

    static func completed(_ date: Date, now: Date) -> String {
        days(from: now, to: date) >= -1 ? time(date) : short(date)
    }

    static func priority(_ value: String?) -> String? {
        switch value {
        case "H": "High"
        case "M": "Medium"
        case "L": "Low"
        default: nil
        }
    }

    /// Minutes as the TUI shows them: "45m", "2h", "2h 40m". The sign is
    /// left to the caller.
    static func minutes(_ value: Int) -> String {
        let (hours, rest) = (abs(value) / 60, abs(value) % 60)
        if hours == 0 { return "\(rest)m" }
        return rest == 0 ? "\(hours)h" : "\(hours)h \(rest)m"
    }

    /// A plan's YYYY-MM-DD day, at its start in the local time zone.
    static func planDay(_ value: String) -> Date? {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.date(from: value)
    }

    /// Why a task is in its Plan Today group: "Overdue 2d", "Due today
    /// 3:00 PM", "Scheduled today", "Planned yesterday", or a candidate's date.
    static func planReason(_ item: PlanItem, now: Date) -> DateLabel? {
        let task = item.task
        if let planned = item.plannedDate.flatMap(planDay) {
            let ago = -days(from: now, to: planned)
            return DateLabel(text: ago == 1 ? "Planned yesterday" : "Planned \(ago) days ago", tone: .normal, scheduled: false, help: "Planned for \(full(planned))")
        }
        switch (task.todayGroup, task.due, task.scheduled) {
        case ("Overdue", let due?, _):
            return DateLabel(text: "Overdue \(-days(from: now, to: due))d", tone: .overdue, scheduled: false, help: "Due \(full(due))")
        case ("Due Today", let due?, _):
            return DateLabel(text: hasTime(due) ? "Due today \(time(due))" : "Due today", tone: .today, scheduled: false, help: "Due \(full(due))")
        case ("Scheduled Today", _, let scheduled?):
            return DateLabel(text: "Scheduled today", tone: .today, scheduled: true, help: "Scheduled \(full(scheduled))")
        case (_, let due?, _):
            return DateLabel(text: "Due \(weekdayDay(due))", tone: .normal, scheduled: false, help: "Due \(full(due))")
        case (_, nil, let scheduled?):
            let text = days(from: now, to: scheduled) < 0 ? "Started \(short(scheduled))" : "Starts \(weekdayDay(scheduled))"
            return DateLabel(text: text, tone: .normal, scheduled: true, help: "Scheduled \(full(scheduled))")
        default:
            return nil
        }
    }

    /// "Fri, Oct 30".
    static func weekdayDay(_ date: Date) -> String {
        date.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day())
    }

    /// Review due values arrive as Taskwarrior basic ISO ("20261015T150000").
    static func reviewDate(_ value: String) -> String? {
        // DateFormatter reads an empty string as 1 Jan 2000.
        guard value.count == 15 else { return nil }
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyyMMdd'T'HHmmss"
        return formatter.date(from: value.trimmingCharacters(in: .whitespaces)).map(full)
    }
}

extension DateLabel.Tone {
    var color: Color {
        switch self {
        case .overdue: Theme.danger
        case .today: Theme.text2
        case .normal: Theme.text3
        }
    }
}

/// Capture highlight colors from the macOS design: project teal, dates blue,
/// other metadata amber.
enum FieldStyle {
    static func mark(for field: String) -> Color {
        switch field {
        case "project": Theme.markProject
        case "due", "time", "scheduled", "recurrence": Theme.markDate
        default: Theme.markMeta
        }
    }

    static func label(for field: String) -> String {
        switch field {
        case "project": "Project"
        case "priority": "Priority"
        case "due": "Due"
        case "time": "Time"
        case "scheduled": "Scheduled"
        case "tag": "Tag"
        case "estimate": "Estimate"
        case "recurrence": "Repeats"
        default: field.capitalized
        }
    }
}
