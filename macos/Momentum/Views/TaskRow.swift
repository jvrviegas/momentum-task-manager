import SwiftUI

struct TaskRow: View {
    @Environment(AppStore.self) private var store
    let task: TaskItem

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            CompletionButton(task: task)
            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(task.description)
                        .foregroundStyle(task.isCompleted ? .secondary : .primary)
                        .lineLimit(2)
                    if task.isPending, task.start != nil {
                        ActiveBadge()
                    }
                    if task.recur != nil {
                        Image(systemName: "repeat").foregroundStyle(.secondary).help("Repeats \(task.recurDisplay ?? "")")
                    }
                    if !task.annotations.isEmpty {
                        Image(systemName: "note.text").foregroundStyle(.secondary).help("\(task.annotations.count) note(s)")
                    }
                }
                metadata
            }
            Spacer(minLength: 8)
            trailing
        }
        .padding(.vertical, 3)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var metadata: some View {
        let date = task.isPending ? Format.relevantDate(for: task, now: store.now) : nil
        let tags = task.tags.filter { !$0.hasPrefix("momentum-plan-") }
        if task.project != nil || date != nil || task.estimate != nil || !tags.isEmpty {
            HStack(spacing: 10) {
                if let project = task.project {
                    Label(project, systemImage: "folder").foregroundStyle(.cyan)
                }
                if let date {
                    Label(date.text, systemImage: date.scheduled ? "calendar" : date.tone == .overdue ? "exclamationmark.circle" : "clock")
                        .foregroundStyle(date.tone.color)
                        .help(date.help)
                }
                if task.isPending, let estimate = task.estimate {
                    Label(estimate, systemImage: "hourglass").help("Estimate")
                } else if task.isPending, task.estimateWarning != nil {
                    Label("Estimate", systemImage: "exclamationmark.triangle").help(task.estimateWarning ?? "")
                }
                if !tags.isEmpty {
                    Text(tags.map { "+\($0)" }.joined(separator: " ")).foregroundStyle(.green)
                }
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            .labelStyle(CompactLabelStyle())
            .lineLimit(1)
        }
    }

    @ViewBuilder
    private var trailing: some View {
        if task.isCompleted, let end = task.end {
            Text(Format.completed(end, now: store.now))
                .font(.caption)
                .foregroundStyle(.secondary)
                .monospacedDigit()
        } else if let label = Format.priority(task.priority) {
            PriorityMark(priority: task.priority ?? "")
                .help("Priority \(label.lowercased())")
                .accessibilityLabel("Priority \(label)")
        }
    }
}

struct CompletionButton: View {
    @Environment(AppStore.self) private var store
    let task: TaskItem

    var body: some View {
        Button {
            Task { await store.complete(task) }
        } label: {
            Image(systemName: task.isCompleted ? "checkmark.circle.fill" : "circle")
                .font(.body)
                .foregroundStyle(task.isCompleted ? AnyShapeStyle(.secondary) : task.priority == "H" ? AnyShapeStyle(.red) : AnyShapeStyle(.secondary))
        }
        .buttonStyle(.plain)
        .disabled(!task.isPending)
        .accessibilityLabel(task.isCompleted ? "Completed" : "Complete \(task.description)")
    }
}

struct PriorityMark: View {
    let priority: String

    var body: some View {
        Text(priority == "H" ? "!!!" : priority == "M" ? "!!" : "!")
            .font(.callout.weight(.bold))
            .foregroundStyle(priority == "H" ? Color.red : priority == "M" ? .orange : .secondary)
    }
}

struct ActiveBadge: View {
    var body: some View {
        Label("Active", systemImage: "play.fill")
            .font(.caption2.weight(.semibold))
            .labelStyle(CompactLabelStyle())
            .foregroundStyle(.green)
            .padding(.horizontal, 6)
            .padding(.vertical, 1)
            .background(.green.opacity(0.15), in: Capsule())
            .help("Started — active in Taskwarrior")
    }
}

struct CompactLabelStyle: LabelStyle {
    func makeBody(configuration: Configuration) -> some View {
        HStack(spacing: 3) {
            configuration.icon
            configuration.title
        }
    }
}
