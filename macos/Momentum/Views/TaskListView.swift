import SwiftUI

struct TaskListView: View {
    @Environment(AppStore.self) private var store
    @FocusState private var listFocused: Bool
    let focusSearch: () -> Void

    var body: some View {
        content
            .navigationTitle(store.view.title)
            .navigationSubtitle(subtitle)
    }

    @ViewBuilder
    private var content: some View {
        @Bindable var store = store
        if let error = store.startupError {
            ContentUnavailableView {
                Label("Couldn’t start Momentum", systemImage: "exclamationmark.triangle")
            } description: {
                Text(error)
            } actions: {
                Button("Try Again") { Task { await store.start() } }
            }
        } else if let error = store.loadError {
            ContentUnavailableView {
                Label("Couldn’t load tasks", systemImage: "exclamationmark.triangle")
            } description: {
                Text("No data changed.\n\(error)")
            } actions: {
                Button("Try Again") { Task { await store.refresh() } }
            }
        } else if store.snapshot == nil {
            ProgressView("Loading tasks…")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            List(selection: $store.selection) {
                if store.view == .today, !store.estimatesWritable {
                    EstimateBanner()
                }
                if store.view == .completed, let error = store.snapshot?.completedError {
                    Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(.orange)
                }
                ForEach(store.visibleSections) { section in
                    Section {
                        ForEach(section.tasks) { task in
                            TaskRow(task: task).tag(task.uuid)
                        }
                    } header: {
                        SectionHeader(section: section, view: store.view)
                    }
                }
            }
            .overlay {
                if store.visibleSections.isEmpty {
                    emptyState
                }
            }
            .contextMenu(forSelectionType: String.self) { uuids in
                if let task = uuids.first.flatMap({ store.tasksByUUID[$0] }) {
                    TaskMenu(task: task)
                }
            } primaryAction: { _ in
                store.showInspector = true
            }
            .focused($listFocused)
            .onAppear { listFocused = true }
            .taskKeyCommands(focusSearch: focusSearch)
        }
    }

    private var subtitle: String {
        if !store.query.trimmingCharacters(in: .whitespaces).isEmpty {
            return "\(store.visibleTasks.count) of \(store.count(store.view)) match"
        }
        switch store.view {
        case .today: return store.now.formatted(.dateTime.weekday(.wide).month(.wide).day())
        case .inbox: return "By project · oldest first"
        case .completed: return "Last 30 days · read-only"
        }
    }

    @ViewBuilder
    private var emptyState: some View {
        if !store.query.trimmingCharacters(in: .whitespaces).isEmpty {
            ContentUnavailableView.search(text: store.query)
        } else {
            switch store.view {
            case .today:
                ContentUnavailableView {
                    Label("Nothing due today", systemImage: "sun.max")
                } description: {
                    Text("Overdue, due, scheduled and planned tasks land here.")
                } actions: {
                    Button("New Task") { store.showCapture = true }
                    Button("Open Inbox") { store.view = .inbox }
                }
            case .inbox:
                ContentUnavailableView {
                    Label("No pending tasks", systemImage: "tray")
                } description: {
                    Text("Tasks that are not due or scheduled today land here.")
                } actions: {
                    Button("New Task") { store.showCapture = true }
                }
            case .completed:
                ContentUnavailableView {
                    Label("No recently completed tasks", systemImage: "checkmark.circle")
                } description: {
                    Text("Tasks completed in the last 30 days appear here.")
                }
            }
        }
    }
}

struct SectionHeader: View {
    let section: ResolvedSection
    let view: ViewName

    var body: some View {
        HStack(spacing: 6) {
            if view == .inbox, section.project != nil {
                Image(systemName: "folder")
            }
            Text(section.title)
                .foregroundStyle(section.title == "Overdue" && view == .today ? AnyShapeStyle(.red) : AnyShapeStyle(.primary))
            Text("\(section.tasks.count)")
                .foregroundStyle(.secondary)
                .monospacedDigit()
            if view == .inbox, let project = section.project, project != section.title {
                Text(project)
                    .font(.caption.monospaced())
                    .foregroundStyle(.tertiary)
            }
        }
    }
}

struct EstimateBanner: View {
    var body: some View {
        Label {
            VStack(alignment: .leading, spacing: 2) {
                Text("Estimates are read-only on this Mac.").bold()
                Text("Add `uda.estimate.type=duration` and `uda.estimate.label=Estimate` to ~/.taskrc. Other actions keep working.")
                    .foregroundStyle(.secondary)
            }
        } icon: {
            Image(systemName: "exclamationmark.triangle").foregroundStyle(.orange)
        }
        .font(.callout)
        .padding(.vertical, 4)
    }
}

struct TaskMenu: View {
    @Environment(AppStore.self) private var store
    let task: TaskItem

    var body: some View {
        if task.isPending {
            Button("Complete") { Task { await store.complete(task) } }
            Button(task.start == nil ? "Start" : "Stop") { Task { await store.toggleStart(task) } }
            Button("Edit…") { store.editing = EditRequest(task: task, focus: .description) }
            if task.recurrenceTarget != nil {
                Button("Stop Repeating…") { store.pendingStopRecurrence = task }
            }
            Divider()
            Button("Delete…", role: .destructive) { store.pendingDelete = task }
        } else {
            Button("Show Details") {
                store.selection = task.uuid
                store.showInspector = true
            }
        }
    }
}
