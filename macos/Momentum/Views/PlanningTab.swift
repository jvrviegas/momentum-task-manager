import SwiftUI

/// Settings ▸ Planning: whether Plan Today is offered, and how much a day
/// holds. Each change is written to config.toml as it is made.
struct PlanningTab: View {
    @Environment(AppStore.self) private var store

    private static let days = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"]

    var body: some View {
        let model = store.settings
        if let planning = model.snapshot?.planning {
            Form {
                if let notice = model.notice {
                    SettingsNotice(text: notice)
                }
                Section {
                    Toggle(isOn: Binding(get: { planning.enabled }, set: { value in Task { await model.setPlanningEnabled(value) } })) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Use Plan Today")
                            Text("Adds the Plan button, the Today’s plan box in the sidebar, the Plan Today… menu item and the P key.")
                                .font(.callout)
                                .foregroundStyle(Theme.text2)
                        }
                    }
                }
                Section("Capacity") {
                    LabeledContent("Daily capacity") {
                        LengthField(field: "dailyCapacity", title: "Daily capacity", placeholder: "8h", value: planning.dailyCapacity) {
                            await model.setDailyCapacity($0)
                        }
                    }
                    LabeledContent("Buffer") {
                        LengthField(field: "buffer", title: "Buffer", placeholder: "1h", value: planning.buffer) {
                            await model.setBuffer($0)
                        }
                    }
                }
                .disabled(!planning.enabled)
                Section {
                    Grid(alignment: .topLeading, horizontalSpacing: 14, verticalSpacing: 12) {
                        GridRow {
                            ForEach(Self.days.prefix(4), id: \.self) { day in weekday(day, planning) }
                        }
                        GridRow {
                            ForEach(Self.days.suffix(3), id: \.self) { day in weekday(day, planning) }
                        }
                    }
                } header: {
                    Text("Capacity by weekday")
                } footer: {
                    // A weekday's message sits here, so the grid never changes height.
                    VStack(alignment: .leading, spacing: 6) {
                        if let message = weekdayProblem {
                            ErrorLine(text: message)
                        }
                        Text("Type a length of time, such as 8h or 7h30m. Leave a day empty to use the daily capacity.")
                    }
                }
                .disabled(!planning.enabled)
            }
            .formStyle(.grouped)
        } else if let notice = model.notice {
            ContentUnavailableView("Unable to read settings", systemImage: "exclamationmark.triangle", description: Text(notice))
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    /// The first weekday whose value was refused, with its name.
    private var weekdayProblem: String? {
        Self.days.lazy.compactMap { day in
            store.settings.error("planning", "weekdayCapacity.\(day)").map { "\(day.capitalized): \($0)" }
        }.first
    }

    private func weekday(_ day: String, _ planning: PlanningSettings) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(day.capitalized).font(.callout).foregroundStyle(Theme.text2)
            LengthField(
                field: "weekdayCapacity.\(day)", title: day.capitalized, placeholder: planning.dailyCapacity,
                value: planning.weekdayCapacity[day] ?? "", showsMessage: false
            ) { await store.settings.setWeekdayCapacity(day, $0) }
        }
    }
}

/// A length of time typed as 8h or 7h30m. It saves on Return and when it
/// loses focus, and a value the file would refuse stays in the field with the
/// message under it.
struct LengthField: View {
    @Environment(AppStore.self) private var store
    /// The tab whose messages the field shows.
    var tab = "planning"
    let field: String
    let title: String
    let placeholder: String
    /// What config.toml says now.
    let value: String
    /// false when the message is shown elsewhere; the field is outlined anyway.
    var showsMessage = true
    let save: (String) async -> Void
    @State private var draft = ""
    @FocusState private var focused: Bool

    var body: some View {
        VStack(alignment: .trailing, spacing: 4) {
            TextField(title, text: $draft, prompt: Text(placeholder))
                .labelsHidden()
                .textFieldStyle(.roundedBorder)
                .frame(width: 96)
                .overlay {
                    if store.settings.error(tab, field) != nil {
                        RoundedRectangle(cornerRadius: 6).strokeBorder(Theme.danger, lineWidth: 1.5)
                    }
                }
                .focused($focused)
                .onSubmit(commit)
                .onChange(of: focused) { _, isFocused in
                    if !isFocused { commit() }
                }
                .onChange(of: draft) { store.settings.clearError(tab, field) }
                .accessibilityLabel(title)
            if showsMessage, let message = store.settings.error(tab, field) {
                ErrorLine(text: message)
            }
        }
        .onAppear { draft = value }
        .onChange(of: value) { _, new in draft = new }
        .onChange(of: store.settings.reloads) { draft = value }
    }

    private func commit() {
        let text = draft.trimmingCharacters(in: .whitespaces)
        guard text != value else { return }
        Task { await save(text) }
    }
}
