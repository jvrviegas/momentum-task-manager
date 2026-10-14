import SwiftUI

struct InspectorView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        if let task = store.selectedTask {
            ScrollView {
                TaskDetails(task: task)
                    .padding(16)
            }
        } else {
            ContentUnavailableView("No Selection", systemImage: "sidebar.right", description: Text("Select a task to see its details."))
        }
    }
}

private struct TaskDetails: View {
    @Environment(AppStore.self) private var store
    let task: TaskItem
    @State private var showTechnical = false

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            header
            if task.isPending {
                HStack {
                    Button(task.start == nil ? "Start" : "Stop", systemImage: task.start == nil ? "play" : "stop") {
                        Task { await store.toggleStart(task) }
                    }
                    Button("Edit", systemImage: "pencil") {
                        store.editing = EditRequest(task: task, focus: .description)
                    }
                    Button("Delete", systemImage: "trash", role: .destructive) {
                        store.pendingDelete = task
                    }
                }
                .controlSize(.small)
            } else if task.isCompleted {
                Label("Completed tasks are read-only. Undo the completion while undo is available to change it.", systemImage: "lock")
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }
            properties
            if let recur = task.recur {
                section("Repeats", systemImage: "repeat") {
                    Text(task.recurDisplay ?? recur).bold() + Text("  recur:\(recur)").font(.caption.monospaced()).foregroundStyle(.secondary)
                    if task.isPending, task.recurrenceTarget != nil {
                        Button("Stop Repeating…") { store.pendingStopRecurrence = task }
                            .controlSize(.small)
                    }
                }
            }
            if !task.depends.isEmpty {
                section("Depends on", systemImage: "link") {
                    ForEach(task.depends, id: \.self) { uuid in
                        if let dependency = store.tasksByUUID[uuid] {
                            Button(dependency.description) { store.selection = uuid }
                                .buttonStyle(.link)
                        } else {
                            Text(uuid).font(.caption.monospaced()).foregroundStyle(.secondary)
                        }
                    }
                }
            }
            section("Notes", systemImage: "note.text") {
                if task.annotations.isEmpty {
                    Text("No annotations.").foregroundStyle(.secondary)
                }
                ForEach(Array(task.annotations.enumerated()), id: \.offset) { _, note in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(note.description).textSelection(.enabled)
                        if let entry = note.entry {
                            Text(Format.full(entry)).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                }
            }
            DisclosureGroup("Taskwarrior details", isExpanded: $showTechnical) {
                Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 12, verticalSpacing: 4) {
                    technical("UUID", task.uuid)
                    technical("ID", task.number == 0 ? "—" : "\(task.number)")
                    technical("Status", task.status)
                    technical("Urgency", task.urgency.formatted(.number.precision(.fractionLength(3))))
                    if let entry = task.entry { technical("Entry", entry.formatted(.iso8601)) }
                    if let modified = task.modified { technical("Modified", modified.formatted(.iso8601)) }
                    if let start = task.start { technical("Start", start.formatted(.iso8601)) }
                    if let end = task.end { technical("End", end.formatted(.iso8601)) }
                    if let raw = task.estimateRaw { technical("estimate", raw) }
                    if let parent = task.parent { technical("parent", parent) }
                    ForEach((task.extra ?? [:]).sorted(by: { $0.key < $1.key }), id: \.key) { key, value in
                        technical(key, value)
                    }
                }
                .font(.caption.monospaced())
                .textSelection(.enabled)
                .padding(.top, 6)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            CompletionButton(task: task)
            VStack(alignment: .leading, spacing: 6) {
                Text(task.description)
                    .font(.title3.weight(.semibold))
                    .textSelection(.enabled)
                HStack(spacing: 6) {
                    if task.isCompleted, let end = task.end {
                        Text("Completed \(Format.full(end))").foregroundStyle(.secondary)
                    }
                    if task.isPending, let start = task.start {
                        ActiveBadge().help("Active since \(Format.full(start))")
                    }
                    if task.isPending {
                        Chip(text: task.todayGroup ?? "Inbox")
                    }
                }
                .font(.caption)
            }
        }
    }

    private var properties: some View {
        Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 14, verticalSpacing: 8) {
            row("Project") {
                if let project = task.project {
                    let label = store.projects.first { $0.value == project }?.label
                    Text(label ?? project) + Text(label != nil ? "  \(project)" : "").font(.caption.monospaced()).foregroundStyle(.secondary)
                } else {
                    Text("None").foregroundStyle(.secondary)
                }
            }
            row("Priority") {
                if let label = Format.priority(task.priority) {
                    Text(label).foregroundStyle(task.priority == "H" ? .red : task.priority == "M" ? .orange : .primary)
                } else {
                    Text("None").foregroundStyle(.secondary)
                }
            }
            row("Due") {
                if let due = task.due {
                    Text(Format.full(due)).foregroundStyle(task.todayGroup == "Overdue" ? .red : .primary)
                } else {
                    Text("No due date").foregroundStyle(.secondary)
                }
            }
            row("Scheduled") {
                Text(task.scheduled.map(Format.full) ?? "Not scheduled")
                    .foregroundStyle(task.scheduled == nil ? .secondary : .primary)
            }
            row("Estimate") {
                VStack(alignment: .leading, spacing: 2) {
                    if let estimate = task.estimate {
                        Text(estimate)
                    } else if let warning = task.estimateWarning {
                        Text("Unsupported value \(task.estimateRaw ?? "")").foregroundStyle(.orange)
                        Text(warning).font(.caption).foregroundStyle(.secondary)
                    } else {
                        Text("Unknown").foregroundStyle(.secondary)
                    }
                    if !store.estimatesWritable {
                        Text("Estimate UDA not configured — read-only.").font(.caption).foregroundStyle(.secondary)
                    }
                }
            }
            row("Tags") {
                if task.tags.isEmpty {
                    Text("None").foregroundStyle(.secondary)
                } else {
                    Text(task.tags.map { "+\($0)" }.joined(separator: " ")).foregroundStyle(.green)
                }
            }
        }
    }

    private func row(_ label: String, @ViewBuilder value: () -> some View) -> some View {
        GridRow {
            Text(label).foregroundStyle(.secondary).gridColumnAlignment(.trailing)
            value()
        }
    }

    private func technical(_ key: String, _ value: String) -> some View {
        GridRow {
            Text(key).foregroundStyle(.secondary).gridColumnAlignment(.trailing)
            Text(value)
        }
    }

    private func section(_ title: String, systemImage: String, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(title, systemImage: systemImage)
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
                .textCase(.uppercase)
            content()
        }
    }
}

struct Chip: View {
    let text: String

    var body: some View {
        Text(text)
            .padding(.horizontal, 6)
            .padding(.vertical, 1)
            .background(.quaternary, in: Capsule())
    }
}
