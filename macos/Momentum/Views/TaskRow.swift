import SwiftUI

struct TaskRow: View {
    @Environment(AppStore.self) private var store
    let task: TaskItem
    /// Narrow lists move the project from the right into the line below.
    var compact = false
    /// Selected in the focused list, on the accent-colored selection.
    var emphasized = false
    @State private var hovering = false

    var body: some View {
        HStack(spacing: 10) {
            CompletionButton(task: task, emphasized: emphasized)
            VStack(alignment: .leading, spacing: 1) {
                HStack(spacing: 7) {
                    Text(task.description)
                        .foregroundStyle(tone(task.isCompleted ? Theme.text2 : Theme.text))
                        .lineLimit(1)
                    if task.isPending, task.start != nil {
                        ActiveBadge()
                    }
                    if task.recur != nil {
                        Image(systemName: "repeat").foregroundStyle(tone(Theme.text3)).help("Repeats \(task.recurDisplay ?? "")")
                    }
                    if !task.annotations.isEmpty {
                        Image(systemName: "note.text").foregroundStyle(tone(Theme.text3)).help("\(task.annotations.count) note(s)")
                    }
                    let blockers = task.isPending ? store.dependencies.blockers(of: task) : []
                    if !blockers.isEmpty {
                        Image(systemName: "hourglass")
                            .foregroundStyle(tone(Theme.text3))
                            .help("Blocked by \(blockers.map { "“\($0.description)”" }.formatted(.list(type: .and)))")
                            .accessibilityLabel("Blocked")
                    }
                }
                subline
            }
            Spacer(minLength: 8)
            meta
        }
        .frame(minHeight: 28)
        .padding(.vertical, 2)
        .onHover { hovering = $0 }
        .accessibilityElement(children: .combine)
    }

    private var tags: [String] {
        task.tags.filter { !$0.hasPrefix("momentum-plan-") }
    }

    @ViewBuilder
    private var subline: some View {
        let project = compact ? task.project : nil
        if project != nil || !tags.isEmpty {
            HStack(spacing: 10) {
                if let project {
                    Label(project, systemImage: "folder")
                }
                ForEach(tags, id: \.self) { tag in
                    Text("+\(tag)")
                }
            }
            .font(.system(size: 11.5))
            .foregroundStyle(tone(Theme.text3))
            .labelStyle(CompactLabelStyle())
            .lineLimit(1)
        }
    }

    @ViewBuilder
    private var meta: some View {
        HStack(spacing: 12) {
            if task.isPending, hovering || store.selection == task.uuid {
                RowActions(task: task)
            } else if !compact, let project = task.project {
                Text(project)
                    .truncationMode(.tail)
                    .frame(maxWidth: 160, alignment: .trailing)
                    .help("Project \(project)")
            }
            if task.isCompleted, let end = task.end {
                Text(Format.completed(end, now: store.now))
            }
            if task.isPending, let date = Format.relevantDate(for: task, now: store.now) {
                Group {
                    if date.scheduled {
                        Label(date.text, systemImage: "calendar")
                    } else {
                        Text(date.text)
                    }
                }
                .fontWeight(date.tone == .normal ? nil : .medium)
                .foregroundStyle(tone(date.tone.color))
                .help(date.help)
            }
            if task.isPending, let estimate = task.estimate {
                Label(estimate, systemImage: "clock").help("Estimate")
            } else if task.isPending, let warning = task.estimateWarning {
                Label("Estimate", systemImage: "exclamationmark.triangle").foregroundStyle(tone(Theme.warn)).help(warning)
            }
            if task.isPending, let label = Format.priority(task.priority) {
                PriorityMark(priority: task.priority ?? "", emphasized: emphasized)
                    .help("Priority \(label.lowercased())")
                    .accessibilityLabel("Priority \(label)")
            }
        }
        .font(.system(size: 12))
        .monospacedDigit()
        .foregroundStyle(tone(Theme.text3))
        .labelStyle(CompactLabelStyle())
        .lineLimit(1)
        .fixedSize(horizontal: true, vertical: false)
    }

    /// The design's color, or white on the emphasized selection.
    private func tone(_ color: Color) -> Color {
        emphasized ? .white : color
    }
}

/// Start, edit and delete, shown on the hovered or selected row.
private struct RowActions: View {
    @Environment(AppStore.self) private var store
    let task: TaskItem

    var body: some View {
        HStack(spacing: 2) {
            Button(task.start == nil ? "Start" : "Stop", systemImage: task.start == nil ? "play.fill" : "stop") {
                Task { await store.toggleStart(task) }
            }
            .help(task.start == nil ? "Start (S)" : "Stop (S)")
            Button("Edit", systemImage: "pencil") {
                store.editing = EditRequest(task: task, focus: .description)
            }
            .help("Edit (E)")
            Button("Delete", systemImage: "trash") {
                store.pendingDelete = task
            }
            .help("Delete (D)")
        }
        .labelStyle(.iconOnly)
        .buttonStyle(.borderless)
        .controlSize(.small)
    }
}

struct CompletionButton: View {
    @Environment(AppStore.self) private var store
    let task: TaskItem
    var emphasized = false
    @State private var hovering = false

    var body: some View {
        if task.isPending {
            Button {
                Task { await store.complete(task) }
            } label: {
                mark
            }
            .buttonStyle(.plain)
            .onHover { hovering = $0 }
            .accessibilityLabel("Complete \(task.description)")
        } else {
            // Not a disabled button, which would dim the completed check.
            mark.accessibilityLabel(task.isCompleted ? "Completed" : task.status.capitalized)
        }
    }

    private var mark: some View {
        ZStack {
            if task.isCompleted {
                Circle().fill(Theme.accent)
            } else {
                Circle().strokeBorder(hovering ? Theme.accent : ring, lineWidth: 1.3)
            }
            if task.isCompleted || hovering {
                Image(systemName: "checkmark")
                    .font(.system(size: 8, weight: .bold))
                    .foregroundStyle(task.isCompleted ? Theme.accentInk : Theme.accent)
            }
        }
        .frame(width: 18, height: 18)
        .contentShape(Circle())
    }

    private var ring: Color {
        if emphasized { return .white }
        return task.priority == "H" || task.priority == "M" ? Theme.priority(task.priority) : Theme.text3
    }
}

struct PriorityMark: View {
    let priority: String
    var emphasized = false

    var body: some View {
        Text(priority == "H" ? "!!!" : priority == "M" ? "!!" : "!")
            .font(.system(size: 12, weight: .bold))
            .tracking(-0.5)
            .foregroundStyle(emphasized ? .white : Theme.priority(priority))
    }
}

struct ActiveBadge: View {
    var title = "Active"
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var dimmed = false

    var body: some View {
        HStack(spacing: 4) {
            Circle()
                .fill(Theme.accent)
                .frame(width: 6, height: 6)
                .opacity(dimmed ? 0.35 : 1)
                .animation(reduceMotion ? nil : .easeInOut(duration: 1.2).repeatForever(), value: dimmed)
            Text(title)
        }
        .fixedSize()
        .font(.system(size: 11, weight: .semibold))
        .foregroundStyle(Theme.accentText)
        .padding(.leading, 5)
        .padding(.trailing, 6)
        .padding(.vertical, 1)
        .background(Theme.accentSoft, in: Capsule())
        .help("Started — in progress")
        .onAppear { dimmed = true }
    }
}

struct CompactLabelStyle: LabelStyle {
    func makeBody(configuration: Configuration) -> some View {
        HStack(spacing: 4) {
            configuration.icon
            configuration.title
        }
    }
}
