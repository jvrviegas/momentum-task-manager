import SwiftUI

struct ToastView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        if let toast = store.toast {
            HStack(spacing: 10) {
                switch toast.kind {
                case .error: Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                case .success: Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
                case .info: EmptyView()
                }
                Text(toast.message)
                    .lineLimit(3)
                    .textSelection(.enabled)
                if toast.offersUndo, store.sync.undoAvailable {
                    if let until = store.undoClosesAt {
                        TimelineView(.periodic(from: .now, by: 1)) { context in
                            let seconds = max(0, Int(until.timeIntervalSince(context.date).rounded(.up)))
                            Text(store.isBackend ? "\(seconds)s left" : "Syncs in \(seconds)s")
                                .monospacedDigit()
                                .foregroundStyle(.secondary)
                        }
                    }
                    Button("Undo") { Task { await store.undo() } }
                        .controlSize(.small)
                }
                Button {
                    store.dismissToast()
                } label: {
                    Image(systemName: "xmark")
                }
                .buttonStyle(.borderless)
                .accessibilityLabel("Dismiss")
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 9)
            .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 10))
            .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(.separator))
            .shadow(color: .black.opacity(0.15), radius: 10, y: 3)
            .padding(.bottom, 16)
            .padding(.horizontal, 24)
            .transition(.move(edge: .bottom).combined(with: .opacity))
            .id(toast.id)
            .accessibilityElement(children: .contain)
            .accessibilityAddTraits(toast.kind == .error ? [] : .updatesFrequently)
        }
    }
}

struct ShortcutsView: View {
    @Environment(\.dismiss) private var dismiss

    private static let groups: [(String, [(String, String)])] = [
        ("Navigate", [("j / ↓", "Next task"), ("k / ↑", "Previous task"), ("g / G", "First / last task"),
                      ("1 2 3", "Today / Inbox / Completed"), ("↩", "Show details"), ("/", "Search"), ("esc", "Clear search")]),
        ("Act on selection", [("Space", "Complete"), ("s", "Start / stop"), ("d", "Delete…"), ("x", "Stop repeating…"),
                              ("u", "Undo"), ("r", "Refresh"), ("c", "Quick capture")]),
        ("Edit a field", [("e", "Description"), ("E", "Estimate"), ("p", "Project"), ("!", "Priority"),
                          ("D", "Due"), ("S", "Scheduled"), ("t", "Tags"), ("R", "Repeats")]),
        ("Quick capture", [("↩", "Add (or review)"), ("↑ ↓", "Choose suggestion"), ("⇥", "Accept suggestion"),
                           ("esc", "Keep phrase as text / close"), ("\\#", "Literal trigger")]),
        ("App", [("⌘N", "New task"), ("⌘F", "Search"), ("⌘R", "Sync now"), ("⌘Z", "Undo last change"),
                 ("⌘E", "Edit"), ("⌘↩", "Complete"), ("⌃⌘I", "Inspector")]),
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Keyboard Shortcuts").font(.title2.bold())
            Text("Single-key shortcuts work while the task list has focus. Capture triggers: # project · ! priority · @ due · > scheduled · + tag · ~ estimate · ^ repeat")
                .foregroundStyle(.secondary)
            LazyVGrid(columns: [GridItem(.adaptive(minimum: 220), alignment: .top)], alignment: .leading, spacing: 18) {
                ForEach(Self.groups, id: \.0) { title, items in
                    VStack(alignment: .leading, spacing: 5) {
                        Text(title).font(.headline)
                        ForEach(items, id: \.1) { keys, label in
                            HStack {
                                Text(keys).font(.body.monospaced()).frame(width: 72, alignment: .leading)
                                Text(label).foregroundStyle(.secondary)
                            }
                        }
                    }
                }
            }
            HStack {
                Spacer()
                Button("Done") { dismiss() }.keyboardShortcut(.defaultAction)
            }
        }
        .padding(24)
        .frame(width: 720)
    }
}
