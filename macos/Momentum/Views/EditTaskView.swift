import SwiftUI

/// Structured edit. Values use the TUI editor's text formats and go to the
/// bridge, which validates them and sends only changed fields.
struct EditTaskView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.dismiss) private var dismiss
    let request: EditRequest
    @State private var fields: EditFields
    @State private var error: BridgeError?
    @State private var saving = false
    @FocusState private var focus: EditField?

    init(request: EditRequest) {
        self.request = request
        _fields = State(initialValue: request.task.edit)
    }

    private var task: TaskItem { request.task }
    private var changed: Bool { fields != task.edit }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            VStack(alignment: .leading, spacing: 4) {
                Text("Edit Task").font(.title3.bold())
                Text("Only changed fields are sent to Taskwarrior. Other properties are kept.")
                    .foregroundStyle(.secondary)
            }
            Form {
                labeled(.description) {
                    TextField("Description", text: $fields.description)
                }
                labeled(.project) {
                    TextField("Project", text: $fields.project, prompt: Text("none"))
                        .textInputSuggestions {
                            ForEach(store.projects.filter { $0.value != fields.project && (fields.project.isEmpty || $0.value.localizedCaseInsensitiveContains(fields.project)) }.prefix(12), id: \.value) { project in
                                Text(project.label ?? project.value).textInputCompletion(project.value)
                            }
                        }
                }
                labeled(.priority) {
                    Picker("Priority", selection: $fields.priority) {
                        Text("None").tag("")
                        Text("Low").tag("L")
                        Text("Medium").tag("M")
                        Text("High").tag("H")
                    }
                    .pickerStyle(.segmented)
                }
                labeled(.due) {
                    DateExpressionField(title: "Due", text: $fields.due)
                }
                labeled(.scheduled) {
                    DateExpressionField(title: "Scheduled", text: $fields.scheduled)
                }
                labeled(.estimate, hint: store.estimatesWritable ? "1 minute to 24 hours, e.g. 45m, 1.5h, 1h30m. Empty clears it." : "Estimate writes are blocked until ~/.taskrc defines uda.estimate.type=duration.") {
                    TextField("Estimate", text: $fields.estimate, prompt: Text("none"))
                        .disabled(!store.estimatesWritable)
                }
                labeled(.recurrence, hint: task.parent == nil ? nil : store.isBackend
                    ? "Changes the rule of the whole series. Use Stop Repeating to end it."
                    : "Changes this instance. Use Stop Repeating to end the whole series.") {
                    TextField("Repeats", text: $fields.recurrence, prompt: Text("does not repeat"))
                        .textInputSuggestions {
                            ForEach(matching(["daily", "weekdays", "weekly", "2wks", "monthly"], fields.recurrence), id: \.self) { Text($0).textInputCompletion($0) }
                        }
                }
                labeled(.tags) {
                    TextField("Tags", text: $fields.tags, prompt: Text("space separated"))
                }
            }
            .formStyle(.grouped)
            .scrollDisabled(true)
            if let error, error.field == nil {
                Label(error.message, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.red).textSelection(.enabled)
            }
            HStack {
                Text("↩ or ⌘S to save · esc to cancel").font(.caption).foregroundStyle(.secondary)
                Spacer()
                Button("Cancel", role: .cancel) { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button("Save") { save() }
                    .keyboardShortcut(.defaultAction)
                    .disabled(!changed || saving)
                Button("Save", action: save)
                    .keyboardShortcut("s")
                    .frame(width: 0, height: 0)
                    .opacity(0)
                    .accessibilityHidden(true)
            }
        }
        .padding(20)
        .frame(width: 520)
        .onAppear { focus = request.focus }
    }

    private func labeled(_ field: EditField, hint: String? = nil, @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            content().focused($focus, equals: field)
            if let error, error.field == field.bridgeName {
                Text(error.message).font(.caption).foregroundStyle(.red)
            } else if let hint {
                Text(hint).font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    private func save() {
        guard changed, !saving else { return }
        saving = true
        Task {
            defer { saving = false }
            do {
                try await store.save(task, edits: fields)
                dismiss()
            } catch let failure as BridgeError {
                error = failure
            } catch {
                self.error = BridgeError(message: error.localizedDescription)
            }
        }
    }
}

/// A Taskwarrior date expression ("tomorrow", "2026-10-15 09:30", …) with a
/// calendar picker that fills in the local "YYYY-MM-DD HH:MM" form.
struct DateExpressionField: View {
    let title: String
    @Binding var text: String
    @State private var picking = false
    @State private var date = Date()

    private static let format: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd HH:mm"
        return formatter
    }()

    var body: some View {
        HStack {
            TextField(title, text: $text, prompt: Text("none — e.g. tomorrow, friday, 2026-10-15 09:30"))
                .textInputSuggestions {
                    ForEach(matching(["today", "tomorrow", "monday", "friday", "next-week"], text), id: \.self) { Text($0).textInputCompletion($0) }
                }
            Button {
                date = Self.format.date(from: text) ?? Calendar.current.date(bySettingHour: 9, minute: 0, second: 0, of: .now) ?? .now
                picking = true
            } label: {
                Image(systemName: "calendar")
            }
            .buttonStyle(.borderless)
            .help("Pick a date")
            .popover(isPresented: $picking) {
                VStack(alignment: .trailing) {
                    DatePicker(title, selection: $date)
                        .datePickerStyle(.graphical)
                        .labelsHidden()
                    Button("Use Date") {
                        text = Self.format.string(from: date)
                        picking = false
                    }
                    .keyboardShortcut(.defaultAction)
                }
                .padding()
            }
            if !text.isEmpty {
                Button {
                    text = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                }
                .buttonStyle(.borderless)
                .foregroundStyle(.secondary)
                .help("Clear \(title.lowercased())")
            }
        }
    }
}

extension EditField {
    /// The field name the bridge reports in errors.
    var bridgeName: String {
        switch self {
        case .description: "description"
        case .project: "project"
        case .priority: "priority"
        case .due: "due"
        case .scheduled: "scheduled"
        case .tags: "tags"
        case .estimate: "estimate"
        case .recurrence: "recurrence"
        }
    }
}

/// Completions that extend what is typed, so Return never lands on a
/// suggestion the user did not ask for.
func matching(_ options: [String], _ text: String) -> [String] {
    let typed = text.trimmingCharacters(in: .whitespaces).lowercased()
    return options.filter { typed.isEmpty || ($0.hasPrefix(typed) && $0 != typed) }
}
