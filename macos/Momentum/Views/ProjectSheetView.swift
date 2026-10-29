import SwiftUI

/// The sheet that adds or edits a catalog entry. A rename that changes the
/// value goes on, here, to a second step that offers to move tasks.
struct ProjectSheetView: View {
    @Environment(AppStore.self) private var store
    @Bindable var sheet: ProjectSheet

    var body: some View {
        let model = store.catalog
        VStack(alignment: .leading, spacing: 16) {
            Text(heading).font(.headline)
            switch sheet.step {
            case .form:
                form(model)
            case .rename(let preview):
                rename(preview)
            case .partial(let reason):
                partial(reason)
            }
            if let notice = sheet.notice {
                ErrorLine(text: notice)
            }
            buttons(model)
        }
        .padding(20)
        .frame(width: 460)
    }

    private var heading: String {
        switch sheet.step {
        case .form: sheet.title
        case .rename: "Rename Project"
        case .partial: "Rename Project"
        }
    }

    // MARK: Step one

    private func form(_ model: ProjectsModel) -> some View {
        Form {
            VStack(alignment: .leading, spacing: 4) {
                TextField("Name", text: $sheet.name, prompt: Text("Client Name"))
                    .onChange(of: sheet.name) { sheet.errors["name"] = nil }
                if let message = sheet.errors["name"] { ErrorLine(text: message) }
            }
            VStack(alignment: .leading, spacing: 4) {
                Picker("Parent", selection: $sheet.parent) {
                    Text("None").tag("")
                    ForEach(model.parentOptions(for: sheet), id: \.self) { value in
                        Text(value).font(.system(.body, design: .monospaced)).tag(value)
                    }
                }
                .onChange(of: sheet.parent) { sheet.errors["parent"] = nil }
                if let message = sheet.errors["parent"] { ErrorLine(text: message) }
            }
            VStack(alignment: .leading, spacing: 4) {
                TextField("Value", text: $sheet.segment, prompt: Text("client-name"))
                    .font(.system(.body, design: .monospaced))
                    .onChange(of: sheet.segment) { sheet.errors["segment"] = nil }
                if let message = sheet.errors["segment"] { ErrorLine(text: message) }
            }
            LabeledContent("Full value") {
                Text(sheet.value.isEmpty ? "—" : sheet.value)
                    .font(.system(.body, design: .monospaced))
                    .foregroundStyle(sheet.value.isEmpty ? Theme.text2 : Theme.text)
                    .textSelection(.enabled)
            }
        }
        .formStyle(.columns)
    }

    // MARK: Step two

    private func rename(_ preview: ProjectPreview) -> some View {
        let old: String = { if case .edit(let source) = sheet.kind { source.value } else { "" } }()
        let moved = sheet.tasksToMove(preview).count
        return VStack(alignment: .leading, spacing: 12) {
            Text("“\(old)” becomes “\(preview.value)”").fontWeight(.semibold)
            Picker("When you save", selection: $sheet.mode) {
                Text("Catalog only").tag(ProjectRenameMode.catalogOnly)
                Text("Catalog + pending tasks").tag(ProjectRenameMode.catalogAndTasks)
            }
            .pickerStyle(.radioGroup)
            if preview.catalogDescendants > 0 || preview.subprojectTasks.count > preview.exactTasks.count {
                Toggle("Include subprojects", isOn: $sheet.includeSubprojects)
            }
            VStack(alignment: .leading, spacing: 4) {
                Text(catalogLine(preview))
                if sheet.mode == .catalogAndTasks {
                    Text(moved == 0 ? "No pending tasks have this project, so only the catalog changes." : "\(moved) pending \(moved == 1 ? "task moves" : "tasks move") to “\(preview.value)”.")
                }
            }
            .foregroundStyle(Theme.text2)
            if sheet.mode == .catalogAndTasks && preview.destinationTaskOnly {
                VStack(alignment: .leading, spacing: 6) {
                    Label("Tasks already use “\(preview.value)”. The tasks that move will join them.", systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(Theme.warn)
                    Toggle("I understand that these tasks merge", isOn: $sheet.mergeConfirmed)
                }
            }
            Text("Tasks you completed or deleted keep their old project. Undo can’t reverse a rename that moves tasks.")
                .font(.callout)
                .foregroundStyle(Theme.text2)
        }
    }

    private func catalogLine(_ preview: ProjectPreview) -> String {
        guard sheet.includeSubprojects, preview.catalogDescendants > 0 else { return "One catalog entry changes." }
        let count = preview.catalogDescendants
        return "One catalog entry changes, and \(count) below it move with it."
    }

    private func partial(_ reason: String) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("The catalog is saved, but no task moved.", systemImage: "exclamationmark.triangle.fill").fontWeight(.semibold)
            Text(reason).foregroundStyle(Theme.text2).textSelection(.enabled)
            Text("Your tasks still use the old project, which shows under From tasks. To move them, rename the project again.")
                .foregroundStyle(Theme.text2)
        }
    }

    // MARK: Buttons

    private func buttons(_ model: ProjectsModel) -> some View {
        HStack {
            if case .rename = sheet.step {
                Button("Back") { sheet.step = .form }
                    .disabled(sheet.working)
            }
            Spacer()
            if sheet.working { ProgressView().controlSize(.small) }
            if case .partial = sheet.step {
                Button("Done") { model.sheet = nil }.keyboardShortcut(.defaultAction)
            } else {
                Button("Cancel") { model.sheet = nil }.keyboardShortcut(.cancelAction)
                Button(primaryTitle) { Task { await primaryAction(model) } }
                    .keyboardShortcut(.defaultAction)
                    .disabled(sheet.working || !canSubmit)
            }
        }
    }

    private var changesValue: Bool {
        if case .edit(let source) = sheet.kind { sheet.value != source.value } else { false }
    }

    private var primaryTitle: String {
        switch sheet.step {
        case .rename(let preview):
            sheet.mode == .catalogAndTasks && !sheet.tasksToMove(preview).isEmpty
                ? "Rename and Move \(sheet.tasksToMove(preview).count) \(sheet.tasksToMove(preview).count == 1 ? "Task" : "Tasks")" : "Save"
        default:
            if case .add = sheet.kind { "Add Project" } else { changesValue ? "Review Rename…" : "Save" }
        }
    }

    private var canSubmit: Bool {
        if case .rename = sheet.step { sheet.canConfirmRename } else { true }
    }

    private func primaryAction(_ model: ProjectsModel) async {
        if case .rename = sheet.step {
            await model.confirmRename()
        } else {
            await model.submit()
        }
    }
}
