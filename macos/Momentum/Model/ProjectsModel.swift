import Foundation
import Observation

/// What a rename does with the tasks that have the old value.
enum ProjectRenameMode: Sendable {
    case catalogOnly
    case catalogAndTasks
}

/// The sheet that adds or edits a catalog entry. A rename that changes the
/// value goes on, in the same sheet, to a second step where tasks are offered,
/// so the sheet never opens another.
@MainActor
@Observable
final class ProjectSheet: Identifiable {
    enum Kind: Equatable {
        case add
        case edit(source: ProjectRow)
    }

    enum Step: Equatable {
        case form
        /// The value changes: what happens to the catalog and to tasks.
        case rename(ProjectPreview)
        /// The catalog is saved but no task moved, and why.
        case partial(String)
    }

    let id = UUID()
    let kind: Kind
    var name: String
    var parent: String
    var segment: String
    var step = Step.form
    var mode = ProjectRenameMode.catalogOnly
    var includeSubprojects = false
    /// Agreed to merge the moved tasks into the ones already on the new value.
    var mergeConfirmed = false
    /// Messages for refused fields: name, parent and segment.
    var errors: [String: String] = [:]
    /// A problem that belongs to no field.
    var notice: String?
    var working = false

    init(kind: Kind, name: String = "", parent: String = "", segment: String = "") {
        self.kind = kind
        self.name = name
        self.parent = parent
        self.segment = segment
    }

    /// The full value, as the catalog keeps it; empty until there is a segment.
    var value: String {
        segment.isEmpty ? "" : parent.isEmpty ? segment : "\(parent).\(segment)"
    }

    var title: String {
        switch kind {
        case .add: "Add Project"
        case .edit: "Edit Project"
        }
    }

    /// Whether the second step can be confirmed: moving tasks onto a value
    /// that tasks already use needs an agreement to merge.
    var canConfirmRename: Bool {
        guard case .rename(let preview) = step else { return false }
        return mode == .catalogOnly || !preview.destinationTaskOnly || mergeConfirmed
    }

    /// The tasks the preview lists for the Include subprojects choice.
    func tasksToMove(_ preview: ProjectPreview) -> [ProjectTaskMove] {
        includeSubprojects ? preview.subprojectTasks : preview.exactTasks
    }
}

/// A removal waiting for its answer: what happens to the entries below.
struct ProjectRemoval: Identifiable {
    var row: ProjectRow
    /// Catalog entries below it.
    var children: Int
    var id: String { row.value }
}

/// What the Projects tab shows and does. Every add, edit and removal is saved
/// from its own sheet or prompt, never from a draft on the tab; a rename that
/// moves tasks goes through a preview first.
@MainActor
@Observable
final class ProjectsModel {
    private(set) var snapshot: ProjectsSnapshot?
    private(set) var loadError: String?
    /// A problem that belongs to the tab: a refused removal, or a file that
    /// changed under it.
    private(set) var notice: String?
    var sheet: ProjectSheet?
    var removal: ProjectRemoval?
    @ObservationIgnored weak var store: AppStore?

    var rows: [ProjectRow] { snapshot?.entries ?? [] }

    func reload() async {
        guard let store else { return }
        do {
            snapshot = try await store.bridge.projects()
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }

    // MARK: Opening

    func beginAdd() {
        notice = nil
        sheet = ProjectSheet(kind: .add)
    }

    /// Opens the add sheet filled in from a value only tasks have. Nothing is
    /// added until the sheet is saved.
    func beginAdd(from row: ProjectRow) {
        notice = nil
        let (parent, segment) = Self.split(row.value)
        sheet = ProjectSheet(kind: .add, name: Self.suggestedName(segment), parent: parent, segment: segment)
    }

    func beginEdit(_ row: ProjectRow) {
        notice = nil
        let (parent, segment) = Self.split(row.value)
        sheet = ProjectSheet(kind: .edit(source: row), name: row.name, parent: parent, segment: segment)
    }

    func beginRemove(_ row: ProjectRow) {
        notice = nil
        removal = ProjectRemoval(row: row, children: childCount(of: row))
    }

    /// The parents a sheet offers: every value, except, when editing, the
    /// entry itself and what is below it, which it cannot move under.
    func parentOptions(for sheet: ProjectSheet) -> [String] {
        var values = rows.map(\.value)
        if case .edit(let source) = sheet.kind {
            values.removeAll { $0 == source.value || $0.hasPrefix(source.value + ".") }
        }
        if !sheet.parent.isEmpty, !values.contains(sheet.parent) { values.append(sheet.parent) }
        return values
    }

    // MARK: Saving

    /// Saves the sheet's draft. An edit that changes the value first asks for
    /// a preview, which also checks the draft; the rest save at once.
    func submit() async {
        guard let sheet, !sheet.working, let revision = snapshot?.revision else { return }
        sheet.notice = nil
        sheet.errors = [:]
        var edit = draft(sheet, revision: revision)
        guard case .edit(let source) = sheet.kind, sheet.value != source.value else {
            await save(sheet, edit)
            return
        }
        sheet.working = true
        defer { sheet.working = false }
        do {
            let preview = try await store?.bridge.previewMigration(edit)
            guard let preview else { return }
            if preview.valueChanged {
                sheet.mode = .catalogOnly
                sheet.includeSubprojects = false
                sheet.mergeConfirmed = false
                sheet.step = .rename(preview)
            } else {
                edit.migrate = false
                sheet.working = false
                await save(sheet, edit)
            }
        } catch {
            await refuse(error, in: sheet)
        }
    }

    /// Saves the rename the second step shows, with the choices made there.
    func confirmRename() async {
        guard let sheet, !sheet.working, sheet.canConfirmRename, case .rename(let preview) = sheet.step,
            let revision = snapshot?.revision
        else { return }
        var edit = draft(sheet, revision: revision)
        edit.includeSubprojects = sheet.includeSubprojects
        edit.migrate = sheet.mode == .catalogAndTasks
        edit.tasks = edit.migrate ? sheet.tasksToMove(preview) : []
        await save(sheet, edit)
    }

    func confirmRemoval(children: Bool) async {
        guard let removal, let revision = snapshot?.revision else { return }
        self.removal = nil
        notice = nil
        var edit = ProjectEdit(revision: revision, action: "remove", source: removal.row.value)
        edit.removeChildren = children
        do {
            _ = try await store?.bridge.saveProject(edit)
            await saved()
        } catch let failure as BridgeError {
            notice = failure.message
            if failure.code == "configConflict" { await reload() }
        } catch {
            notice = error.localizedDescription
        }
    }

    private func save(_ sheet: ProjectSheet, _ edit: ProjectEdit) async {
        guard let store else { return }
        sheet.working = true
        defer { sheet.working = false }
        do {
            let result = try await store.bridge.saveProject(edit)
            await saved()
            if result.changed > 0 {
                // The server moved the tasks in one step, which no undo reverses.
                store.closeUndoWindow()
                store.show("Moved \(result.changed) \(result.changed == 1 ? "task" : "tasks") to “\(sheet.value)”", .success)
            }
            if let reason = result.taskError {
                sheet.step = .partial(reason)
            } else {
                self.sheet = nil
            }
        } catch {
            await refuse(error, in: sheet)
        }
    }

    /// What follows a saved change: the tab and the main window, whose
    /// suggestions and labels come from the catalog, read it again.
    private func saved() async {
        await reload()
        if let store, store.account != nil { await store.refresh() }
    }

    private func refuse(_ error: any Error, in sheet: ProjectSheet) async {
        guard let failure = error as? BridgeError else {
            sheet.notice = error.localizedDescription
            return
        }
        if let field = failure.field {
            sheet.errors[field] = failure.message
            sheet.step = .form
        } else if failure.code == "configConflict" {
            self.sheet = nil
            notice = failure.message
            await reload()
        } else if failure.code == "stalePreview" {
            sheet.step = .form
            sheet.notice = failure.message
        } else {
            sheet.notice = failure.message
        }
    }

    private func draft(_ sheet: ProjectSheet, revision: String) -> ProjectEdit {
        var edit = ProjectEdit(revision: revision, action: "add", name: sheet.name, parent: sheet.parent, segment: sheet.segment)
        if case .edit(let source) = sheet.kind {
            edit.action = "edit"
            edit.source = source.value
        }
        return edit
    }

    private func childCount(of row: ProjectRow) -> Int {
        rows.filter { $0.inCatalog && $0.value.hasPrefix(row.value + ".") }.count
    }

    // MARK: Values

    /// work.client as ("work", "client"); a value with no dot has no parent.
    nonisolated static func split(_ value: String) -> (parent: String, segment: String) {
        guard let dot = value.lastIndex(of: ".") else { return ("", value) }
        return (String(value[..<dot]), String(value[value.index(after: dot)...]))
    }

    /// A name to start from for a value found on tasks: client-name is Client Name.
    nonisolated static func suggestedName(_ segment: String) -> String {
        segment.replacingOccurrences(of: "-", with: " ").capitalized
    }
}
