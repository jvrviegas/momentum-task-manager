import SwiftUI

struct TaskListView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.appearsActive) private var appearsActive
    @State private var compact = false
    let focus: FocusState<FocusedPane?>.Binding

    var body: some View {
        content
            .navigationTitle(store.view.title)
            .navigationSubtitle(subtitle)
            .background(Theme.content)
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
        } else if let status = store.status, store.backendURL == nil {
            ContentUnavailableView {
                Label("Add a Momentum server", systemImage: "server.rack")
            } description: {
                Text("Momentum keeps your tasks on a Momentum server. Add its address to \(status.configPath):")
                Text(verbatim: "[backend]\nurl = \"https://momentum.example.com\"")
                    .font(.system(size: 12, design: .monospaced))
                    .textSelection(.enabled)
                Text("Then quit and reopen Momentum Task Manager, and sign in.")
            }
        } else if let url = store.backendURL, store.server == .signedOut, store.snapshot == nil {
            ContentUnavailableView {
                Label("Sign in to see your tasks", systemImage: "person.crop.circle")
            } description: {
                Text("Momentum keeps your tasks on \(url). Sign in with Google or GitHub in your browser.")
            } actions: {
                Button("Sign In…") { store.beginSignIn() }
                    .keyboardShortcut(.defaultAction)
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
            ProgressView("Loading tasks from the server…")
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            List(selection: $store.selection) {
                if store.view == .completed, let error = store.snapshot?.completedError {
                    Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(Theme.warn)
                }
                ForEach(store.visibleSections) { section in
                    Section {
                        ForEach(section.tasks) { task in
                            TaskRow(task: task, compact: compact, emphasized: focus.wrappedValue == .list && appearsActive && store.selection == task.uuid)
                                .tag(task.uuid)
                        }
                    } header: {
                        SectionHeader(section: section, view: store.view)
                    }
                }
                .listRowSeparatorTint(Theme.separator)
            }
            .scrollContentBackground(.hidden)
            .onGeometryChange(for: Bool.self) { $0.size.width < 600 } action: { compact = $0 }
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
            .focused(focus, equals: .list)
            .onAppear { focus.wrappedValue = .list }
            .taskKeyCommands(focusSearch: { focus.wrappedValue = .search })
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
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            if view == .inbox, section.project != nil {
                Image(systemName: "folder").foregroundStyle(Theme.text3)
            }
            Text(section.title)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(section.title == "Overdue" && view == .today ? Theme.danger : Theme.text2)
            Text("\(section.tasks.count)")
                .font(.system(size: 11.5))
                .foregroundStyle(Theme.text3)
                .monospacedDigit()
            if view == .inbox, let project = section.project, project != section.title {
                Text(project)
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundStyle(Theme.text3)
            }
        }
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
            Button("Duplicate") { Task { await store.duplicate(task) } }
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
