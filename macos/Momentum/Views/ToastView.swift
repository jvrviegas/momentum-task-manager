import SwiftUI

struct ToastView: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        if let toast = store.toast {
            HStack(spacing: 12) {
                switch toast.kind {
                case .error: Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Theme.danger)
                case .success: Image(systemName: "checkmark.circle.fill").foregroundStyle(Theme.accent)
                case .info: EmptyView()
                }
                Text(toast.message)
                    .foregroundStyle(toast.kind == .error ? Theme.danger : Theme.text)
                    .lineLimit(3)
                    .frame(maxWidth: 380, alignment: .leading)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
                if toast.offersUndo, let until = store.undoClosesAt {
                    TimelineView(.periodic(from: .now, by: 1)) { context in
                        let seconds = max(0, Int(until.timeIntervalSince(context.date).rounded(.up)))
                        HStack(spacing: 6) {
                            CountdownRing(remaining: Double(seconds) / store.undoWindow.delay)
                            Text("\(seconds)s left")
                        }
                        .monospacedDigit()
                        .foregroundStyle(Theme.text3)
                        .help("Undo closes when the countdown ends")
                    }
                    Button {
                        Task { await store.undo() }
                    } label: {
                        ShortcutLabel(title: "Undo", systemImage: "arrow.uturn.backward", key: "⌘Z")
                    }
                    .controlSize(.small)
                }
                Button {
                    store.dismissToast()
                } label: {
                    Image(systemName: "xmark").font(.system(size: 10, weight: .semibold))
                }
                .buttonStyle(.borderless)
                .foregroundStyle(Theme.text2)
                .accessibilityLabel("Dismiss")
            }
            .font(.system(size: 12.5))
            .padding(.leading, 14)
            .padding(.trailing, 8)
            .padding(.vertical, 7)
            .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 11))
            .background(Theme.elevated.opacity(0.92), in: RoundedRectangle(cornerRadius: 11))
            .overlay(RoundedRectangle(cornerRadius: 11).strokeBorder(Theme.separatorStrong, lineWidth: 0.5))
            .shadow(color: .black.opacity(0.14), radius: 12, y: 8)
            .shadow(color: .black.opacity(0.06), radius: 3, y: 2)
            .padding(.bottom, 20)
            .padding(.horizontal, 24)
            .transition(.move(edge: .bottom).combined(with: .opacity))
            .id(toast.id)
            .accessibilityElement(children: .contain)
            .accessibilityAddTraits(toast.kind == .error ? [] : .updatesFrequently)
        }
    }
}

/// How much of the undo window is left, drawn as a shrinking arc.
private struct CountdownRing: View {
    let remaining: Double

    var body: some View {
        ZStack {
            Circle().stroke(Theme.separatorStrong, lineWidth: 1.6)
            Circle()
                .trim(from: 0, to: min(1, max(0, remaining)))
                .stroke(Theme.accent, style: StrokeStyle(lineWidth: 1.6, lineCap: .round))
                .rotationEffect(.degrees(-90))
                .animation(.linear(duration: 1), value: remaining)
        }
        .frame(width: 14, height: 14)
        .accessibilityHidden(true)
    }
}

struct ShortcutsView: View {
    @Environment(\.dismiss) private var dismiss

    private static let groups: [(String, [(String, String)])] = [
        ("Navigate", [("j / ↓", "Next task"), ("k / ↑", "Previous task"), ("g / G", "First / last task"),
                      ("1 2 3", "Inbox / Today / Completed"), ("↩", "Show details"), ("/", "Search"), ("esc", "Clear search")]),
        ("Act on selection", [("Space", "Complete"), ("s", "Start / stop"), ("d", "Delete…"), ("x", "Stop repeating…"),
                              ("u", "Undo"), ("r", "Refresh"), ("c", "Quick capture")]),
        ("Edit a field", [("e", "Description"), ("E", "Estimate"), ("p", "Project"), ("!", "Priority"),
                          ("D", "Due"), ("S", "Scheduled"), ("t", "Tags"), ("R", "Repeats")]),
        ("Quick capture", [("↩", "Add (or review)"), ("↑ ↓", "Choose suggestion"), ("⇥", "Accept suggestion"),
                           ("esc", "Keep phrase as text / close"), ("\\#", "Literal trigger")]),
        ("App", [("⌘N", "New task"), ("⌘F", "Search"), ("⌘Z", "Undo last change"),
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
