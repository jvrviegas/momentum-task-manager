import SwiftUI

struct ContentView: View {
    @Environment(AppStore.self) private var store
    @FocusState private var searchFocused: Bool

    var body: some View {
        @Bindable var store = store
        NavigationSplitView {
            SidebarView(focusSearch: { searchFocused = true })
                .navigationSplitViewColumnWidth(min: 190, ideal: 220, max: 280)
        } detail: {
            TaskListView(focusSearch: { searchFocused = true })
                .inspector(isPresented: $store.showInspector) {
                    InspectorView()
                        .inspectorColumnWidth(min: 260, ideal: 320, max: 440)
                }
        }
        .searchable(text: $store.query, placement: .toolbar, prompt: "Search \(store.view.title)")
        .searchFocused($searchFocused)
        .toolbar {
            ToolbarItem(placement: .navigation) {
                SyncStatusView()
            }
            ToolbarItem(placement: .primaryAction) {
                Button {
                    store.showCapture = true
                } label: {
                    Label("New Task", systemImage: "plus")
                }
                .help("New Task (⌘N)")
                .disabled(store.status == nil)
            }
        }
        .sheet(isPresented: $store.showCapture) {
            CaptureView(store: store)
        }
        .sheet(item: $store.editing) { request in
            EditTaskView(request: request)
        }
        .sheet(isPresented: $store.showShortcuts) {
            ShortcutsView()
        }
        .confirmationDialog(
            "Delete “\(store.pendingDelete?.description ?? "")”?",
            isPresented: Binding(get: { store.pendingDelete != nil }, set: { if !$0 { store.pendingDelete = nil } }),
            presenting: store.pendingDelete
        ) { task in
            Button("Delete", role: .destructive) { Task { await store.delete(task) } }
                .keyboardShortcut("y", modifiers: [])
            Button("Cancel", role: .cancel) {}
                .keyboardShortcut("n", modifiers: [])
        } message: { task in
            Text("The task is marked deleted in Taskwarrior. You can undo this \(store.isLocalOnly ? "until your next change" : "until it syncs")\(task.recur != nil ? "; other instances of the series are not affected" : "").")
        }
        .confirmationDialog(
            "Stop repeating “\(store.pendingStopRecurrence?.description ?? "")”?",
            isPresented: Binding(get: { store.pendingStopRecurrence != nil }, set: { if !$0 { store.pendingStopRecurrence = nil } }),
            presenting: store.pendingStopRecurrence
        ) { task in
            Button("Stop Repeating") { Task { await store.stopRecurrence(task) } }
            Button("Cancel", role: .cancel) {}
        } message: { _ in
            Text("Momentum expires the recurrence template so Taskwarrior creates no new instances. Existing instances and completed history stay.")
        }
        .overlay(alignment: .bottom) {
            ToastView()
                .animation(.snappy, value: store.toast)
        }
    }
}

struct SyncStatusView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        Button {
            store.syncNow()
        } label: {
            TimelineView(.periodic(from: .now, by: 1)) { context in
                Label(title(at: context.date), systemImage: symbol)
                    .labelStyle(.titleAndIcon)
                    .foregroundStyle(store.sync.phase == .retrying ? AnyShapeStyle(.orange) : AnyShapeStyle(.secondary))
                    .monospacedDigit()
            }
        }
        .buttonStyle(.borderless)
        .help(store.syncError ?? (store.isLocalOnly ? "Taskwarrior sync is not configured" : "Sync now (⌘R)"))
    }

    private var symbol: String {
        if store.isLocalOnly { return "internaldrive" }
        switch store.sync.phase {
        case .inFlight: return "arrow.triangle.2.circlepath"
        case .retrying: return "exclamationmark.triangle"
        case .grace: return "clock.arrow.circlepath"
        default: return "checkmark.icloud"
        }
    }

    private func title(at date: Date) -> String {
        if store.isLocalOnly { return "Local only" }
        switch store.sync.phase {
        case .grace:
            guard let until = store.sync.undoUntil else { return "Changes pending sync" }
            return "Syncing in \(max(0, Int(until.timeIntervalSince(date).rounded(.up))))s"
        case .inFlight:
            return "Syncing…"
        case .retrying:
            return "Sync failed · retrying"
        default:
            guard let last = store.lastSyncAt else { return "Sync ready" }
            return "Synced \(last.formatted(.relative(presentation: .named)))"
        }
    }
}
