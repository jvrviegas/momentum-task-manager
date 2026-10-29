import SwiftUI

/// The pane with keyboard focus. The single-key shortcuts live on the sidebar
/// and the task list, so leaving search hands focus back to the list.
enum FocusedPane: Hashable {
    case sidebar, list, search
}

struct ContentView: View {
    @Environment(AppStore.self) private var store
    @FocusState private var focus: FocusedPane?
    @State private var windowSize = CGSize(width: 1120, height: 720)

    var body: some View {
        @Bindable var store = store
        NavigationSplitView {
            SidebarView(focus: $focus)
                .navigationSplitViewColumnWidth(min: 190, ideal: 220, max: 280)
        } detail: {
            TaskListView(focus: $focus)
                .inspector(isPresented: $store.showInspector) {
                    InspectorView()
                        .inspectorColumnWidth(min: 260, ideal: 340, max: 440)
                }
        }
        .onGeometryChange(for: CGSize.self) { $0.size } action: { windowSize = $0 }
        .searchable(text: $store.query, placement: .toolbar, prompt: "Search \(store.view.title)")
        .searchFocused($focus, equals: .search)
        // Return keeps the filter and Esc clears it, as in the TUI; either way
        // the list takes focus. Esc alone would leave nothing focused.
        .onSubmit(of: .search) { focus = .list }
        .onChange(of: focus) { old, new in
            if old == .search, new == nil { focus = .list }
        }
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                ServerStatusView()
            }
            if store.planningEnabled {
                ToolbarItem(placement: .primaryAction) {
                    Button {
                        Task { await store.openPlan() }
                    } label: {
                        Label("Plan", systemImage: "text.badge.plus")
                            .labelStyle(.titleAndIcon)
                    }
                    .help("Plan Today (⇧P)")
                    .disabled(store.account == nil)
                }
            }
            ToolbarItem(placement: .primaryAction) {
                Button {
                    store.showCapture = true
                } label: {
                    Label("New Task", systemImage: "plus")
                        .labelStyle(.titleAndIcon)
                }
                .buttonStyle(.borderedProminent)
                .tint(Theme.tint)
                .help("New Task (⌘N)")
                .disabled(store.account == nil)
            }
            ToolbarItem(placement: .primaryAction) {
                Button {
                    store.showInspector.toggle()
                } label: {
                    Label(store.showInspector ? "Hide Inspector" : "Show Inspector", systemImage: "sidebar.right")
                }
                .help("Toggle inspector (⌃⌘I)")
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
        .sheet(item: Binding(get: { store.signInModel.flow }, set: { if $0 == nil { store.signInModel.cancel() } })) { _ in
            SignInView()
        }
        .sheet(item: Binding(get: { store.planSheet }, set: { if $0 == nil { store.cancelPlan() } })) { _ in
            // The design's 1040×720, kept inside the window.
            PlanTodayView()
                .frame(width: min(1040, windowSize.width - 24), height: min(720, windowSize.height - 64))
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
            Text("The task is marked deleted on the server. You can undo this for \(Duration.seconds(store.undoWindow.delay).formatted(.units(width: .wide))) afterwards\(task.recur != nil ? "; other instances of the series are not affected" : "").")
        }
        .confirmationDialog(
            "Stop repeating “\(store.pendingStopRecurrence?.description ?? "")”?",
            isPresented: Binding(get: { store.pendingStopRecurrence != nil }, set: { if !$0 { store.pendingStopRecurrence = nil } }),
            presenting: store.pendingStopRecurrence
        ) { task in
            Button("Stop Repeating") { Task { await store.stopRecurrence(task) } }
                .keyboardShortcut("y", modifiers: [])
            Button("Cancel", role: .cancel) {}
                .keyboardShortcut("n", modifiers: [])
        } message: { _ in
            Text("Momentum ends the series, so no new instances are created. Existing instances and completed history stay.")
        }
        .overlay(alignment: .bottom) {
            ToastView()
                .animation(.snappy, value: store.toast)
        }
    }
}

/// What the app last learned about its server; a click signs in again or
/// retries.
struct ServerStatusView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        if let url = store.backendURL {
            Button {
                if store.server == .signedOut {
                    store.beginSignIn()
                } else {
                    Task { await store.refresh(announce: true) }
                }
            } label: {
                TimelineView(.periodic(from: .now, by: 1)) { context in
                    Label(title(at: context.date), systemImage: symbol)
                        .labelStyle(.titleAndIcon)
                        .font(.system(size: 12))
                        .foregroundStyle([.unreachable, .signedOut].contains(store.server) ? Theme.warn : Theme.text3)
                        .monospacedDigit()
                }
            }
            .buttonStyle(.borderless)
            .help(help(url))
        }
    }

    private func help(_ url: String) -> String {
        switch store.server {
        case .signedOut: return "Sign in to \(url)"
        case .unreachable: return "Momentum can’t reach \(url). Click to try again."
        default: return "Tasks are kept on \(url). Click to reload."
        }
    }

    private var symbol: String {
        switch store.server {
        case .signedOut: return "person.crop.circle.badge.exclamationmark"
        case .unreachable: return "exclamationmark.triangle"
        case .unknown where store.snapshot == nil: return "arrow.triangle.2.circlepath"
        default: return store.undoClosesAt == nil ? "checkmark.icloud" : "clock.arrow.circlepath"
        }
    }

    private func title(at date: Date) -> String {
        switch store.server {
        case .signedOut: return "Signed out"
        case .unreachable: return "Server unreachable"
        case .unknown where store.snapshot == nil: return "Connecting…"
        default:
            guard let until = store.undoClosesAt, until > date else { return "Online" }
            return "Saved · undo \(max(0, Int(until.timeIntervalSince(date).rounded(.up))))s"
        }
    }
}
