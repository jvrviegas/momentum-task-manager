import SwiftUI

struct InspectorView: View {
    @Environment(AppStore.self) private var store
    let focus: FocusState<FocusedPane?>.Binding

    var body: some View {
        Group {
            if let task = store.selectedTask {
                GeometryReader { proxy in
                    ScrollView {
                        TaskDetails(task: task, focus: focus)
                            .frame(minHeight: proxy.size.height, alignment: .top)
                    }
                }
            } else {
                ContentUnavailableView("No Selection", systemImage: "sidebar.right", description: Text("Select a task to see its details."))
            }
        }
        .background(Theme.panel)
    }
}

private struct TaskDetails: View {
    @Environment(AppStore.self) private var store
    let task: TaskItem
    let focus: FocusState<FocusedPane?>.Binding
    @State private var showTechnical = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            divider
            if task.isPending {
                HStack(spacing: 6) {
                    Button {
                        Task { await store.toggleStart(task) }
                    } label: {
                        ShortcutLabel(title: task.start == nil ? "Start" : "Stop", systemImage: task.start == nil ? "play" : "stop", key: "s")
                    }
                    Button {
                        store.editing = EditRequest(task: task, focus: .description)
                    } label: {
                        ShortcutLabel(title: "Edit", systemImage: "pencil", key: "e")
                    }
                    Button("Delete", systemImage: "trash") {
                        store.pendingDelete = task
                    }
                    .buttonStyle(.borderless)
                    .foregroundStyle(Theme.text2)
                    .accessibilityLabel("Delete task")
                }
                .controlSize(.small)
                .padding(.horizontal, 18)
                .padding(.vertical, 12)
                divider
            } else if task.isCompleted {
                Label("Completed tasks are read-only. Undo the completion while undo is available to change it.", systemImage: "lock")
                    .font(.system(size: 12))
                    .foregroundStyle(Theme.text3)
                    .padding(.horizontal, 18)
                    .padding(.vertical, 12)
                divider
            }
            properties
                .padding(.horizontal, 18)
                .padding(.vertical, 14)
            if let recur = task.recur {
                section("Repeats", systemImage: "repeat") {
                    VStack(alignment: .leading, spacing: 6) {
                        HStack(spacing: 7) {
                            Text(task.recurDisplay ?? recur).fontWeight(.semibold)
                            Text("recur:\(recur)").font(.system(size: 11, design: .monospaced)).foregroundStyle(Theme.text3)
                        }
                        if task.isPending, task.recurrenceTarget != nil {
                            Button {
                                store.pendingStopRecurrence = task
                            } label: {
                                ShortcutLabel(title: "Stop Repeating…", systemImage: "repeat", key: "x")
                                    .labelStyle(.titleOnly)
                            }
                            .controlSize(.small)
                        }
                    }
                    .padding(.horizontal, 12)
                    .padding(.vertical, 10)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .card(radius: 9)
                }
            }
            if task.isPending || !task.depends.isEmpty {
                section("Depends on", systemImage: "link") {
                    ForEach(task.depends, id: \.self) { uuid in
                        HStack(alignment: .firstTextBaseline, spacing: 6) {
                            if let dependency = store.tasksByUUID[uuid] {
                                link(to: dependency)
                            } else {
                                Text(uuid).font(.system(size: 11.5, design: .monospaced)).foregroundStyle(Theme.text3)
                            }
                            Spacer(minLength: 0)
                            // S-068: Undo takes a removal back, so nothing asks first.
                            if task.isPending {
                                Button("Remove dependency", systemImage: "xmark") { removeDependency(uuid) }
                                    .labelStyle(.iconOnly)
                                    .buttonStyle(.borderless)
                                    .font(.system(size: 10, weight: .semibold))
                                    .foregroundStyle(Theme.text3)
                                    .help("Remove this dependency")
                            }
                        }
                        .contextMenu {
                            if task.isPending {
                                Button("Remove Dependency") { removeDependency(uuid) }
                            }
                        }
                        .accessibilityAction(named: "Remove Dependency") { if task.isPending { removeDependency(uuid) } }
                    }
                    if task.isPending {
                        Button("Add Dependency…") { store.dependencyPicker = DependencyPicker(task: task) }
                            .controlSize(.small)
                    }
                }
            }
            let dependents = store.dependencies.dependents(of: task)
            if !dependents.isEmpty {
                section("Blocks", systemImage: "hourglass") {
                    ForEach(dependents) { dependent in
                        link(to: dependent)
                    }
                }
            }
            section("Notes", systemImage: "note.text") {
                if task.annotations.isEmpty && !task.isPending {
                    Text("No notes.").font(.system(size: 12)).foregroundStyle(Theme.text3)
                }
                ForEach(Array(task.annotations.enumerated()), id: \.offset) { _, note in
                    HStack(alignment: .top, spacing: 6) {
                        VStack(alignment: .leading, spacing: 3) {
                            Text(note.description).textSelection(.enabled)
                            if let entry = note.entry {
                                Text(Format.full(entry)).font(.system(size: 11)).foregroundStyle(Theme.text3)
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .accessibilityElement(children: .combine)
                        // S-066: Undo takes a removal back, so nothing asks first.
                        if task.isPending {
                            Button("Remove note", systemImage: "xmark") { remove(note) }
                                .labelStyle(.iconOnly)
                                .buttonStyle(.borderless)
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundStyle(Theme.text3)
                                .help("Remove this note")
                        }
                    }
                    .padding(.horizontal, 11)
                    .padding(.vertical, 9)
                    .card(radius: 8)
                    .contextMenu {
                        if task.isPending {
                            Button("Remove Note") { remove(note) }
                        }
                    }
                    .accessibilityAction(named: "Remove Note") { if task.isPending { remove(note) } }
                }
                if task.isPending {
                    NoteField(task: task, focus: focus)
                }
            }
            Spacer(minLength: 0)
            section {
                technical
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var header: some View {
        HStack(alignment: .top, spacing: 10) {
            CompletionButton(task: task)
                .padding(.top, 2)
            VStack(alignment: .leading, spacing: 6) {
                Text(task.description)
                    .font(.system(size: 16, weight: .semibold))
                    .textSelection(.enabled)
                HStack(spacing: 6) {
                    if task.isCompleted, let end = task.end {
                        Text("Completed \(Format.full(end))")
                    }
                    if task.isPending, let start = task.start {
                        ActiveBadge(title: "Active since \(Format.time(start))")
                            .help("Active since \(Format.full(start))")
                    }
                    if task.isPending {
                        Chip(text: task.todayGroup ?? "Inbox")
                    }
                }
                .font(.system(size: 12))
                .foregroundStyle(Theme.text3)
            }
        }
        .padding(.horizontal, 18)
        .padding(.top, 18)
        .padding(.bottom, 14)
    }

    private var properties: some View {
        Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 12, verticalSpacing: 9) {
            row("Project") {
                if let project = task.project {
                    let label = store.projects.first { $0.value == project }?.label
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Text(label ?? project)
                        if label != nil {
                            Text(project).font(.system(size: 12, design: .monospaced)).foregroundStyle(Theme.text3)
                        }
                    }
                } else {
                    Text("None").foregroundStyle(Theme.text3)
                }
            }
            row("Priority") {
                if let label = Format.priority(task.priority) {
                    Text(label).fontWeight(.semibold).foregroundStyle(Theme.priority(task.priority))
                } else {
                    Text("None").foregroundStyle(Theme.text3)
                }
            }
            row("Due") {
                if let due = task.due {
                    Text(Format.full(due)).foregroundStyle(task.todayGroup == "Overdue" ? Theme.danger : Theme.text)
                } else {
                    Text("No due date").foregroundStyle(Theme.text3)
                }
            }
            row("Scheduled") {
                Text(task.scheduled.map(Format.full) ?? "Not scheduled")
                    .foregroundStyle(task.scheduled == nil ? Theme.text3 : Theme.text)
            }
            row("Estimate") {
                VStack(alignment: .leading, spacing: 2) {
                    if let estimate = task.estimate {
                        Text(estimate)
                    } else if let warning = task.estimateWarning {
                        Text("Unsupported value \(task.estimateRaw ?? "")").foregroundStyle(Theme.warn)
                        Text(warning).font(.system(size: 11.5)).foregroundStyle(Theme.text3)
                    } else {
                        Text("Unknown").foregroundStyle(Theme.text3)
                    }
                }
            }
            row("Tags") {
                if task.tags.isEmpty {
                    Text("None").foregroundStyle(Theme.text3)
                } else {
                    FlowLayout(spacing: 4) {
                        ForEach(task.tags, id: \.self) { tag in
                            Chip(text: "+\(tag)")
                        }
                    }
                }
            }
        }
    }

    private var technical: some View {
        VStack(alignment: .leading, spacing: 8) {
            Button {
                withAnimation(.snappy(duration: 0.14)) { showTechnical.toggle() }
            } label: {
                HStack(spacing: 6) {
                    Image(systemName: "chevron.right")
                        .font(.system(size: 9, weight: .semibold))
                        .rotationEffect(.degrees(showTechnical ? 90 : 0))
                    Text("Details")
                }
                .font(.system(size: 11.5, weight: .semibold))
                .foregroundStyle(Theme.text3)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityValue(showTechnical ? "Expanded" : "Collapsed")
            if showTechnical {
                Grid(alignment: .leadingFirstTextBaseline, horizontalSpacing: 12, verticalSpacing: 6) {
                    technical("UUID", task.uuid)
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
                .textSelection(.enabled)
            }
        }
    }

    private var divider: some View {
        Rectangle().fill(Theme.separator).frame(height: 0.5)
    }

    private func remove(_ note: Annotation) {
        Task { await store.removeNote(note, from: task) }
    }

    /// Selects other when clicked. The link button style would centre a
    /// description that wraps.
    private func link(to other: TaskItem) -> some View {
        Button { store.selection = other.uuid } label: {
            Text(other.description).foregroundStyle(.link).multilineTextAlignment(.leading)
        }
        .buttonStyle(.plain)
        .pointerStyle(.link)
    }

    private func removeDependency(_ uuid: String) {
        Task { await store.removeDependency(task, on: uuid) }
    }

    private func row(_ label: String, @ViewBuilder value: () -> some View) -> some View {
        GridRow {
            Text(label)
                .font(.system(size: 12))
                .foregroundStyle(Theme.text3)
                .frame(width: 92, alignment: .trailing)
                .gridColumnAlignment(.trailing)
            value()
        }
    }

    private func technical(_ key: String, _ value: String) -> some View {
        GridRow {
            Text(key)
                .font(.system(size: 12))
                .foregroundStyle(Theme.text3)
                .frame(width: 92, alignment: .trailing)
                .gridColumnAlignment(.trailing)
            Text(value)
                .font(.system(size: 11.5, design: .monospaced))
                .foregroundStyle(Theme.text2)
        }
    }

    private func section(_ title: String? = nil, systemImage: String = "", @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            if let title {
                Label(title, systemImage: systemImage)
                    .font(.system(size: 11.5, weight: .semibold))
                    .foregroundStyle(Theme.text3)
                    .labelStyle(CompactLabelStyle())
            }
            content()
        }
        .padding(.horizontal, 18)
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(alignment: .top) { divider }
    }
}

/// The field at the end of a pending task's notes that adds one (S-065).
/// Return or Add sends it; Escape hands the focus back to the list.
private struct NoteField: View {
    @Environment(AppStore.self) private var store
    let task: TaskItem
    let focus: FocusState<FocusedPane?>.Binding

    var body: some View {
        let draft = store.noteDraft
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                TextField("Add a note", text: Binding(get: { draft.text(for: task) }, set: { draft.write($0, for: task) }), axis: .vertical)
                    .lineLimit(1...4)
                    .textFieldStyle(.roundedBorder)
                    .focused(focus, equals: .note)
                    .onSubmit(add)
                    .onExitCommand { focus.wrappedValue = .list }
                    .accessibilityLabel("New note")
                Button("Add", action: add)
                    .disabled(!draft.canAdd(to: task))
                    .accessibilityLabel("Add note")
            }
            if let error = draft.error(for: task) {
                Label(error, systemImage: "exclamationmark.triangle")
                    .font(.system(size: 11.5))
                    .foregroundStyle(Theme.danger)
                    .textSelection(.enabled)
            }
        }
        .onChange(of: draft.focusRequested, initial: true) { _, requested in
            guard requested else { return }
            focus.wrappedValue = .note
            draft.focusRequested = false
        }
    }

    private func add() {
        Task { await store.noteDraft.add(to: task, in: store) }
    }
}

struct Chip: View {
    let text: String

    var body: some View {
        Text(text)
            .fixedSize()
            .font(.system(size: 12))
            .foregroundStyle(Theme.text)
            .padding(.horizontal, 7)
            .padding(.vertical, 1)
            .background(Theme.hover, in: Capsule())
            .overlay(Capsule().strokeBorder(Theme.separator, lineWidth: 0.5))
    }
}

extension View {
    /// The design's raised card: an elevated surface with a hairline edge.
    func card(radius: CGFloat) -> some View {
        background(Theme.elevated, in: RoundedRectangle(cornerRadius: radius))
            .overlay(RoundedRectangle(cornerRadius: radius).strokeBorder(Theme.separator, lineWidth: 0.5))
    }
}
