import SwiftUI

/// Settings ▸ Projects: the catalog and the values found only on tasks, in
/// one list with their hierarchy. Each add, edit and removal is saved from
/// its own sheet or prompt.
struct ProjectsTab: View {
    @Environment(AppStore.self) private var store

    var body: some View {
        @Bindable var model = store.catalog
        content(model)
            .sheet(item: $model.sheet) { ProjectSheetView(sheet: $0) }
            .confirmationDialog(
                model.removal.map { "Remove “\($0.row.name)”?" } ?? "", isPresented: Binding(get: { model.removal != nil }, set: { if !$0 { model.removal = nil } }),
                titleVisibility: .visible, presenting: model.removal
            ) { removal in
                if removal.children > 0 {
                    Button("Remove Only “\(removal.row.name)”", role: .destructive) { Task { await model.confirmRemoval(children: false) } }
                    Button("Remove “\(removal.row.name)” and \(Self.projects(removal.children)) Under It", role: .destructive) {
                        Task { await model.confirmRemoval(children: true) }
                    }
                } else {
                    Button("Remove", role: .destructive) { Task { await model.confirmRemoval(children: false) } }
                }
                Button("Cancel", role: .cancel) { model.removal = nil }
            } message: { removal in
                Text(
                    (removal.children > 0 ? "It has \(Self.projects(removal.children).lowercased()) under it in the catalog. " : "")
                        + "Tasks keep their project; removing never changes a task.")
            }
    }

    @ViewBuilder
    private func content(_ model: ProjectsModel) -> some View {
        if let snapshot = model.snapshot {
            if snapshot.entries.isEmpty && snapshot.tasksError == nil {
                ContentUnavailableView {
                    Label("No projects yet", systemImage: "folder")
                } description: {
                    Text("Add a project to the catalog so New Task suggests it. Projects your tasks already use show here too.")
                } actions: {
                    Button("Add Project…") { model.beginAdd() }
                }
            } else {
                Form {
                    if let notice = model.notice {
                        SettingsNotice(text: notice)
                    }
                    if let reason = snapshot.tasksError {
                        SettingsNotice(text: "Task counts unavailable: \(reason)")
                    }
                    Section {
                        ForEach(snapshot.entries) { row in
                            ProjectRowView(row: row, showsCount: snapshot.tasksError == nil)
                        }
                        Button("Add Project…") { model.beginAdd() }
                    } header: {
                        Text("Projects")
                    } footer: {
                        Text("The catalog is what New Task suggests. A project from tasks is one your tasks use that the catalog doesn’t have.")
                    }
                }
                .formStyle(.grouped)
            }
        } else if let error = model.loadError {
            ContentUnavailableView("Unable to read projects", systemImage: "exclamationmark.triangle", description: Text(error))
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private static func projects(_ count: Int) -> String { "\(count) \(count == 1 ? "Project" : "Projects")" }
}

private struct ProjectRowView: View {
    @Environment(AppStore.self) private var store
    let row: ProjectRow
    let showsCount: Bool

    var body: some View {
        let model = store.catalog
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                Text(row.name).fontWeight(row.inCatalog ? .semibold : .regular)
                Text(row.value)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundStyle(Theme.text2)
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .help(row.value)
            }
            Spacer(minLength: 8)
            tag
            if showsCount {
                Text(row.tasks == 0 ? "No tasks" : "\(row.tasks) \(row.tasks == 1 ? "task" : "tasks")")
                    .font(.callout)
                    .foregroundStyle(Theme.text2)
                    .monospacedDigit()
                    .frame(width: 64, alignment: .trailing)
            }
            // One width for either control, so the tags and counts line up.
            Group {
                if row.inCatalog {
                    Menu {
                        Button("Edit…") { model.beginEdit(row) }
                        Button("Remove…", role: .destructive) { model.beginRemove(row) }
                    } label: {
                        Image(systemName: "ellipsis.circle")
                    }
                    .menuStyle(.borderlessButton)
                    .fixedSize()
                    .help("Edit or remove this project")
                    .accessibilityLabel("Actions for \(row.name)")
                } else {
                    Button("Add to Catalog…") { model.beginAdd(from: row) }
                        .buttonStyle(.borderless)
                        .help("Add this project to the catalog so New Task suggests it")
                }
            }
            .frame(width: 130, alignment: .trailing)
        }
        .padding(.leading, CGFloat(row.depth) * 18)
        .contextMenu {
            if row.inCatalog {
                Button("Edit…") { model.beginEdit(row) }
                Button("Remove…", role: .destructive) { model.beginRemove(row) }
            } else {
                Button("Add to Catalog…") { model.beginAdd(from: row) }
            }
        }
        .accessibilityElement(children: .contain)
    }

    /// A word, filled for the catalog and outlined for a project from tasks,
    /// so the difference is never color alone.
    private var tag: some View {
        Text(row.inCatalog ? "Catalog" : "From tasks")
            .font(.system(size: 11, weight: .medium))
            .foregroundStyle(row.inCatalog ? Theme.text : Theme.text2)
            .padding(.horizontal, 7)
            .padding(.vertical, 2)
            .background {
                if row.inCatalog {
                    Capsule().fill(.quaternary)
                } else {
                    Capsule().strokeBorder(Theme.text2.opacity(0.5), lineWidth: 1)
                }
            }
    }
}
