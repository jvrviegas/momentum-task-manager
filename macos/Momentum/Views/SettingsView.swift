import AppKit
import SwiftUI

enum SettingsTab: String {
    case projects, planning, calendar, appearance, diagnostics
}

/// The Settings window (settings plan S1): five tabs, each change applied at
/// once. Appearance is built; the others say how to change the same thing in
/// config.toml until their own tabs are.
struct SettingsView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.scenePhase) private var scenePhase
    @AppStorage("settings.tab") private var tab = SettingsTab.projects

    var body: some View {
        TabView(selection: $tab) {
            Tab("Projects", systemImage: "folder", value: SettingsTab.projects) {
                SettingsPage { ProjectsTab() }
            }
            Tab("Planning", systemImage: "text.badge.checkmark", value: SettingsTab.planning) {
                SettingsPage { PlanningTab() }
            }
            Tab("Calendar", systemImage: "calendar", value: SettingsTab.calendar) {
                SettingsPage { CalendarTab() }
            }
            Tab("Appearance", systemImage: "circle.lefthalf.filled", value: SettingsTab.appearance) {
                SettingsPage { AppearanceSettings() }
            }
            Tab("Diagnostics", systemImage: "stethoscope", value: SettingsTab.diagnostics) {
                SettingsPage {
                    SettingsPlaceholder(title: "Diagnostics", symbol: "stethoscope", hint: "For now, run `momentum doctor` in a terminal.", showsConfig: false)
                }
            }
        }
        .frame(width: 760, height: 600)
        .modifier(AppearancePreference())
        // The bridge re-reads config.toml when it changed, so the tabs open, and
        // show again, on what the file says now.
        .task { await refresh() }
        .onChange(of: scenePhase) { _, phase in
            if phase == .active { Task { await refresh() } }
        }
    }

    private func refresh() async {
        await store.refreshStatus()
        await store.settings.reload()
        await store.macCalendar.refresh()
        await store.catalog.reload()
    }
}

/// A problem that belongs to no one field: a write that failed, or a file
/// that changed under the tab.
struct SettingsNotice: View {
    let text: String

    var body: some View {
        Label {
            Text(text).textSelection(.enabled)
        } icon: {
            Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Theme.warn)
        }
        .font(.system(size: 12.5))
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, 4)
    }
}

/// What every tab sits in: the config notice above its content.
private struct SettingsPage<Content: View>: View {
    @ViewBuilder var content: Content

    var body: some View {
        VStack(spacing: 0) {
            ConfigNotice()
            content.frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
}

/// Says when config.toml could not be applied or a changed [backend] url
/// waits for a restart. It is inline, not an alert, because nothing needs
/// doing right now.
private struct ConfigNotice: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        if store.configProblem != nil || store.restartRequired {
            VStack(alignment: .leading, spacing: 10) {
                if let problem = store.configProblem {
                    Label {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Unable to read config.toml").fontWeight(.semibold)
                            Text("The app is still using the last settings that worked.")
                            Text(problem).foregroundStyle(Theme.text2).textSelection(.enabled)
                        }
                    } icon: {
                        Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Theme.warn)
                    }
                }
                if store.restartRequired {
                    Label {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("`[backend] url` changed").fontWeight(.semibold)
                            Text("Quit and reopen Momentum Task Manager to use it.")
                        }
                    } icon: {
                        Image(systemName: "arrow.clockwise.circle.fill").foregroundStyle(Theme.warn)
                    }
                }
                Button("Show config.toml in Finder") { ConfigFile.reveal(store.status?.configPath) }
                    .controlSize(.small)
            }
            .font(.system(size: 12.5))
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 20)
            .padding(.vertical, 12)
            .background(Theme.warnSoft)
            .overlay(alignment: .bottom) { Rectangle().fill(Theme.separator).frame(height: 0.5) }
        }
    }
}

/// A tab whose own settings are not built yet.
private struct SettingsPlaceholder: View {
    @Environment(AppStore.self) private var store
    let title: String
    let symbol: String
    let hint: LocalizedStringKey
    var showsConfig = true

    var body: some View {
        ContentUnavailableView {
            Label("\(title) isn’t in Settings yet", systemImage: symbol)
        } description: {
            Text(hint)
        } actions: {
            if showsConfig {
                Button("Show config.toml in Finder") { ConfigFile.reveal(store.status?.configPath) }
            }
        }
    }
}

private struct AppearanceSettings: View {
    @AppStorage(AppearanceChoice.key) private var choice = AppearanceChoice.system

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Appearance").font(.headline)
            HStack(alignment: .top, spacing: 16) {
                ForEach(AppearanceChoice.allCases) { option in
                    AppearanceCard(option: option, selected: choice == option) { choice = option }
                }
            }
            Text("System follows your Mac’s setting. This applies to this Mac only; the terminal app keeps its own theme.")
                .foregroundStyle(Theme.text2)
            Spacer(minLength: 0)
        }
        .padding(24)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// One choice, drawn as a small window in the look it gives. The words say
/// which it is, so the picture is never the only cue.
private struct AppearanceCard: View {
    let option: AppearanceChoice
    let selected: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            VStack(spacing: 8) {
                preview
                    .frame(width: 200, height: 128)
                    .clipShape(RoundedRectangle(cornerRadius: 9))
                    .overlay(
                        RoundedRectangle(cornerRadius: 9)
                            .strokeBorder(selected ? Theme.accent : Theme.separatorStrong, lineWidth: selected ? 2.5 : 0.5))
                Text(option.title).fontWeight(selected ? .semibold : .regular)
            }
        }
        .buttonStyle(.plain)
        .accessibilityLabel(option.title)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    @ViewBuilder
    private var preview: some View {
        switch option {
        case .light: MiniWindow().environment(\.colorScheme, .light)
        case .dark: MiniWindow().environment(\.colorScheme, .dark)
        case .system:
            ZStack {
                MiniWindow().environment(\.colorScheme, .light)
                MiniWindow().environment(\.colorScheme, .dark)
                    .mask(HStack(spacing: 0) { Color.clear; Color.black })
            }
        }
    }
}

/// A sketch of the app's window: the sidebar, a few rows, and the accent.
private struct MiniWindow: View {
    var body: some View {
        HStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 7) {
                ForEach(0..<3, id: \.self) { index in
                    RoundedRectangle(cornerRadius: 3)
                        .fill(index == 1 ? Theme.accent : Theme.separatorStrong)
                        .frame(width: index == 1 ? 40 : 32, height: 7)
                }
                Spacer()
            }
            .padding(10)
            .frame(width: 62)
            .frame(maxHeight: .infinity)
            .background(Theme.sidebar)
            VStack(alignment: .leading, spacing: 9) {
                ForEach(0..<4, id: \.self) { index in
                    HStack(spacing: 7) {
                        Circle().strokeBorder(Theme.text3, lineWidth: 1).frame(width: 9, height: 9)
                        RoundedRectangle(cornerRadius: 3).fill(Theme.text3.opacity(0.55)).frame(width: 52 + CGFloat(index % 2) * 22, height: 6)
                    }
                }
                Spacer()
            }
            .padding(12)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
            .background(Theme.content)
        }
    }
}

enum ConfigFile {
    /// Shows config.toml in Finder, or the folder it belongs in while the file
    /// does not exist yet.
    static func reveal(_ path: String?) {
        guard let path else { return }
        let url = URL(fileURLWithPath: (path as NSString).expandingTildeInPath)
        if FileManager.default.fileExists(atPath: url.path) {
            NSWorkspace.shared.activateFileViewerSelecting([url])
        } else {
            NSWorkspace.shared.open(url.deletingLastPathComponent())
        }
    }
}
