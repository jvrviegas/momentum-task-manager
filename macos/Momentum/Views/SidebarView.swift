import SwiftUI

struct SidebarView: View {
    @Environment(AppStore.self) private var store
    let focusSearch: () -> Void

    var body: some View {
        List(selection: Binding<ViewName?>(get: { store.view }, set: { if let view = $0 { store.view = view } })) {
            ForEach(ViewName.allCases) { view in
                HStack {
                    Label(view.title, systemImage: view.symbol)
                    Spacer()
                    if view == .today, store.overdueCount > 0 {
                        Text("\(store.overdueCount)")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.white)
                            .padding(.horizontal, 6)
                            .background(.red, in: Capsule())
                            .help("\(store.overdueCount) overdue")
                    }
                    if view != .completed, store.snapshot != nil {
                        Text("\(store.count(view))")
                            .foregroundStyle(.secondary)
                            .monospacedDigit()
                    }
                }
                .tag(view)
            }
        }
        .taskKeyCommands(focusSearch: focusSearch)
        .safeAreaInset(edge: .bottom) {
            VStack(alignment: .leading, spacing: 6) {
                if let url = store.backendURL {
                    // Backend mode has no contexts (D6).
                    Label(store.account?.email ?? "Signed out", systemImage: "person.crop.circle")
                        .help(store.account.map { "Signed in until \($0.expiresAt.formatted(date: .abbreviated, time: .omitted))" } ?? "")
                    Label(URL(string: url)?.host() ?? url, systemImage: "server.rack")
                        .help(url)
                } else {
                    Label {
                        Text("Context ") + Text(store.snapshot?.context ?? "none").bold()
                    } icon: {
                        Image(systemName: "square.3.layers.3d")
                    }
                    .help("Taskwarrior context. Change it with `task context` in a terminal.")
                    if let version = store.status?.taskwarriorVersion {
                        Label("Taskwarrior \(version)", systemImage: "terminal")
                    }
                }
            }
            .font(.callout)
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(12)
        }
    }
}
