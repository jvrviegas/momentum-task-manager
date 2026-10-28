import SwiftUI
import UniformTypeIdentifiers

/// Settings ▸ Calendar, for the calendar files source: whether the files count,
/// and which files there are, each with what loading it on its own found.
struct CalendarTab: View {
    @Environment(AppStore.self) private var store
    @State private var choosing = false

    var body: some View {
        let model = store.settings
        if let calendar = model.snapshot?.calendar {
            Form {
                if let notice = model.notice {
                    SettingsNotice(text: notice)
                }
                Section {
                    Toggle(isOn: Binding(get: { calendar.enabled }, set: { value in Task { await model.setCalendarEnabled(value) } })) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Use calendar files")
                            Text("Meetings in these files take time off each day’s capacity in Plan Today.")
                                .font(.callout)
                                .foregroundStyle(Theme.text2)
                        }
                    }
                    if let message = model.error("calendar", "enabled") {
                        ErrorLine(text: message)
                    }
                }
                Section("Calendar files") {
                    if calendar.files.isEmpty {
                        VStack(alignment: .leading, spacing: 4) {
                            Text("No calendar files yet").fontWeight(.semibold)
                            Text("Export a calendar to an .ics file, then add it here. Momentum reads the file; it never fetches a calendar for you.")
                                .foregroundStyle(Theme.text2)
                        }
                    }
                    ForEach(calendar.files) { file in
                        CalendarFileRow(file: file) { Task { await model.removeCalendarFile(file.path) } }
                    }
                    if let message = model.error("calendar", "paths") {
                        ErrorLine(text: message)
                    }
                    Button("Add…") { choosing = true }
                }
            }
            .formStyle(.grouped)
            .fileImporter(
                isPresented: $choosing, allowedContentTypes: [.calendarEvent, UTType(filenameExtension: "ics")].compactMap { $0 },
                allowsMultipleSelection: true
            ) { result in
                if case .success(let urls) = result {
                    Task { await model.addCalendarFiles(urls.map(\.path)) }
                }
            }
        } else if let notice = model.notice {
            ContentUnavailableView("Unable to read settings", systemImage: "exclamationmark.triangle", description: Text(notice))
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }
}

private struct CalendarFileRow: View {
    let file: CalendarFile
    let remove: () -> Void

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            VStack(alignment: .leading, spacing: 3) {
                Text(file.path)
                    .font(.system(size: 12.5, design: .monospaced))
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .help(file.path)
                status
            }
            Spacer()
            Button("Remove", action: remove)
                .buttonStyle(.borderless)
                .help("Remove this file from the list. The file itself stays.")
                .accessibilityLabel("Remove \(file.path)")
        }
        .accessibilityElement(children: .contain)
    }

    @ViewBuilder
    private var status: some View {
        switch file.status {
        case "available":
            label("Available", symbol: "checkmark.circle.fill", color: Theme.statusOK)
        case "stale":
            label(
                "Stale" + (file.modified.map { " · last changed \($0.formatted(.relative(presentation: .named)))" } ?? ""),
                symbol: "clock.badge.exclamationmark.fill", color: Theme.statusStale)
        case "missing":
            label("Missing", symbol: "questionmark.circle.fill", color: Theme.warn)
        default:
            label("Unreadable" + (file.detail.map { ": \($0)" } ?? ""), symbol: "exclamationmark.triangle.fill", color: Theme.danger)
        }
    }

    /// A symbol and a word, so the state is never a color alone.
    private func label(_ text: String, symbol: String, color: Color) -> some View {
        Label {
            Text(text).foregroundStyle(Theme.text2)
        } icon: {
            Image(systemName: symbol).foregroundStyle(color)
        }
        .font(.system(size: 12))
    }
}

struct ErrorLine: View {
    let text: String

    var body: some View {
        Label(text, systemImage: "exclamationmark.triangle.fill")
            .font(.caption)
            .foregroundStyle(Theme.danger)
            .fixedSize(horizontal: false, vertical: true)
    }
}
