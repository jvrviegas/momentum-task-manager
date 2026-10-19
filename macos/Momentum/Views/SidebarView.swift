import SwiftUI

struct SidebarView: View {
    @Environment(AppStore.self) private var store
    let focusSearch: () -> Void

    var body: some View {
        List(selection: Binding<ViewName?>(get: { store.view }, set: { if let view = $0 { store.view = view } })) {
            ForEach(ViewName.allCases) { view in
                HStack(spacing: 8) {
                    Label {
                        Text(view.title).foregroundStyle(Theme.text)
                    } icon: {
                        Image(systemName: view.symbol).foregroundStyle(Theme.accentText)
                    }
                    Spacer()
                    Group {
                        if view == .today, store.overdueCount > 0 {
                            Text("\(store.overdueCount)")
                                .fontWeight(.semibold)
                                .foregroundStyle(Theme.danger)
                                .help("\(store.overdueCount) overdue")
                        }
                        if view != .completed, store.snapshot != nil {
                            Text("\(store.count(view))")
                                .foregroundStyle(Theme.text3)
                        }
                    }
                    .font(.system(size: 12))
                    .monospacedDigit()
                }
                .tag(view)
            }
        }
        .scrollContentBackground(.hidden)
        .background(Theme.sidebar)
        .taskKeyCommands(focusSearch: focusSearch)
        .safeAreaInset(edge: .bottom) {
            VStack(alignment: .leading, spacing: 2) {
                if let url = store.backendURL {
                    footerRow(store.account?.email ?? "Signed out", systemImage: "person.crop.circle")
                        .help(store.account.map { "Signed in until \($0.expiresAt.formatted(date: .abbreviated, time: .omitted))" } ?? "")
                    footerRow(URL(string: url)?.host() ?? url, systemImage: "server.rack")
                        .help(url)
                }
                Button {
                    store.showShortcuts = true
                } label: {
                    HStack(spacing: 9) {
                        Image(systemName: "keyboard").foregroundStyle(Theme.text3).frame(width: 16)
                        Text("Keyboard Shortcuts")
                        Spacer()
                        KeyCap("?")
                    }
                    .foregroundStyle(Theme.text2)
                    .padding(.horizontal, 9)
                    .frame(height: 28)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 10)
            .padding(.top, 8)
            .padding(.bottom, 12)
            .overlay(alignment: .top) {
                Rectangle().fill(Theme.separator).frame(height: 0.5)
            }
            .background(Theme.sidebar)
        }
    }

    private func footerRow(_ title: String, systemImage: String) -> some View {
        HStack(spacing: 8) {
            Image(systemName: systemImage).foregroundStyle(Theme.text3)
            Text(title).foregroundStyle(Theme.text2)
        }
        .font(.system(size: 12))
        .padding(.horizontal, 9)
        .padding(.vertical, 6)
    }
}
