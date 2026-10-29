import SwiftUI
import UniformTypeIdentifiers

/// Settings ▸ Calendar: where meetings come from (calendar files, macOS
/// Calendar or Google Calendar), and the hours and events that count against a
/// day's capacity.
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
                    Picker("Source", selection: Binding(get: { model.calendarChoice }, set: { value in Task { await model.chooseCalendarSource(value) } })) {
                        Text("Calendar files").tag(CalendarChoice.files)
                        Text("macOS Calendar").tag(CalendarChoice.mac)
                        Text("Google Calendar").tag(CalendarChoice.google)
                    }
                    if let message = model.error("calendar", "source") {
                        ErrorLine(text: message)
                    }
                } footer: {
                    Text("Calendar files and Google Calendar also apply to the terminal app on this Mac. macOS Calendar applies to this app only.")
                }
                switch model.calendarChoice {
                case .files:
                    filesSections(calendar, model)
                case .mac:
                    MacCalendarSection()
                case .google:
                    GoogleCalendarSection()
                }
                Section {
                    DatePicker("Start", selection: timeBinding(calendar.workingStart) { await model.setWorkingStart($0) }, displayedComponents: .hourAndMinute)
                    DatePicker("End", selection: timeBinding(calendar.workingEnd) { await model.setWorkingEnd($0) }, displayedComponents: .hourAndMinute)
                } header: {
                    Text("Working hours")
                } footer: {
                    VStack(alignment: .leading, spacing: 6) {
                        if let message = model.error("calendar", "workingHours") {
                            ErrorLine(text: message)
                        }
                        Text("Only meetings between these hours take time off a day’s capacity. This applies to every calendar source.")
                    }
                }
                Section("Events that count") {
                    Toggle(isOn: Binding(get: { calendar.includeAllDay }, set: { value in Task { await model.setIncludeAllDay(value) } })) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Count all-day events")
                            Text("An all-day event takes the whole working day off the capacity.")
                                .font(.callout)
                                .foregroundStyle(Theme.text2)
                        }
                    }
                    Toggle(isOn: Binding(get: { calendar.includeTransparent }, set: { value in Task { await model.setIncludeTransparent(value) } })) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Count events shown as free")
                            Text("Events your calendar marks as free, such as a reminder, count as busy too.")
                                .font(.callout)
                                .foregroundStyle(Theme.text2)
                        }
                    }
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

extension CalendarTab {
    /// The calendar files source: the switch, the files and how long a file
    /// may go unchanged.
    @ViewBuilder
    fileprivate func filesSections(_ calendar: CalendarSettings, _ model: SettingsModel) -> some View {
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
        Section {
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
            LabeledContent("Stale after") {
                LengthField(
                    tab: "calendar", field: "staleAfter", title: "Stale after", placeholder: "24h", value: calendar.staleAfter
                ) { await model.setStaleAfter($0) }
            }
        } header: {
            Text("Calendar files")
        } footer: {
            Text("A calendar file that hasn’t changed for this long shows as Stale. Calendar files only; 0m means never.")
        }
    }
}

/// A time of day kept as HH:MM, shown as a time field. A value the file
/// refuses leaves the field on what the file has.
private func timeBinding(_ text: String, save: @escaping @Sendable (String) async -> Void) -> Binding<Date> {
    let date = SettingsModel.date(fromTime: text)
    return Binding(get: { date }, set: { value in Task { await save(SettingsModel.time(from: value)) } })
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
