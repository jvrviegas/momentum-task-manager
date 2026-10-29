import Foundation
import Testing
@testable import Momentum

@MainActor
struct ProjectsModelTests {
    private static let rows = [
        ProjectRow(value: "home", name: "Home", tag: "catalog", tasks: 2),
        ProjectRow(value: "work", name: "Work", tag: "catalog", tasks: 4),
        ProjectRow(value: "work.client", name: "Client", tag: "catalog", tasks: 3),
        ProjectRow(value: "work.legacy", name: "legacy", tag: "tasks", tasks: 2),
    ]

    private func move(_ uuid: String, _ old: String, _ new: String) -> ProjectTaskMove {
        ProjectTaskMove(uuid: uuid, oldValue: old, newValue: new)
    }

    private func makeStore() async -> (AppStore, FakeBridge) {
        let bridge = FakeBridge(snapshots: [snapshot(today: [.fixture("a")])])
        await bridge.setProjects(ProjectsSnapshot(revision: "p1", entries: Self.rows, tasksError: nil))
        let store = signedInStore(bridge)
        await store.start()
        await store.catalog.reload()
        return (store, bridge)
    }

    private func row(_ value: String) -> ProjectRow { Self.rows.first { $0.value == value }! }

    /// A rename of work to job that the preview offers tasks for.
    private func renamePreview(destinationTaskOnly: Bool = false) -> ProjectPreview {
        ProjectPreview(
            value: "job", valueChanged: true, catalogDescendants: 1,
            exactTasks: [move("1", "work", "job")],
            subprojectTasks: [move("1", "work", "job"), move("2", "work.client", "job.client")],
            destinationTaskOnly: destinationTaskOnly)
    }

    private func beginRename(_ store: AppStore, _ bridge: FakeBridge, destinationTaskOnly: Bool = false) async {
        await bridge.setPreview(renamePreview(destinationTaskOnly: destinationTaskOnly))
        store.catalog.beginEdit(row("work"))
        store.catalog.sheet?.name = "Job"
        store.catalog.sheet?.segment = "job"
        await store.catalog.submit()
    }

    @Test func theListMirrorsWhatTheBridgeReturns() async {
        let (store, _) = await makeStore()
        #expect(store.catalog.rows == Self.rows)
        #expect(store.catalog.rows.map(\.depth) == [0, 0, 1, 1])
        #expect(store.catalog.rows.map(\.inCatalog) == [true, true, true, false])
    }

    @Test func addingSendsTheDraftOnTheRevisionItReadAndLoadsAgain() async {
        let (store, bridge) = await makeStore()
        store.catalog.beginAdd()
        store.catalog.sheet?.name = "Acme"
        store.catalog.sheet?.parent = "work"
        store.catalog.sheet?.segment = "acme"
        #expect(store.catalog.sheet?.value == "work.acme", "the full value is the parent and the segment")
        await store.catalog.submit()

        #expect(await bridge.projectSaves == [ProjectEdit(revision: "p1", action: "add", name: "Acme", parent: "work", segment: "acme")])
        #expect(store.catalog.sheet == nil)
        #expect(await bridge.calls.filter { $0 == "projects" }.count == 2, "the tab reads the catalog again")
        #expect(await bridge.calls.filter { $0 == "load" }.count == 2, "the main window loads again, for its suggestions")
    }

    @Test func aRefusedDraftStaysInTheSheetOnItsField() async {
        let (store, bridge) = await makeStore()
        await bridge.setProjectFailure(BridgeError(message: "“work.client” is already in the catalog.", field: "segment"))
        store.catalog.beginAdd()
        store.catalog.sheet?.name = "Again"
        store.catalog.sheet?.parent = "work"
        store.catalog.sheet?.segment = "client"
        await store.catalog.submit()

        #expect(store.catalog.sheet?.errors["segment"] == "“work.client” is already in the catalog.")
        #expect(store.catalog.sheet?.name == "Again", "the draft stays")
        #expect(await bridge.calls.filter { $0 == "projects" }.count == 1, "nothing was saved, so nothing is read again")

        store.catalog.sheet?.segment = "other"
        await store.catalog.submit()
        #expect(store.catalog.sheet == nil)
    }

    @Test func changingOnlyTheNameSavesWithoutAPreview() async {
        let (store, bridge) = await makeStore()
        store.catalog.beginEdit(row("home"))
        store.catalog.sheet?.name = "House"
        await store.catalog.submit()
        #expect(await bridge.previews.isEmpty, "no value change, so no tasks are offered")
        #expect(await bridge.projectSaves == [ProjectEdit(revision: "p1", action: "edit", source: "home", name: "House", segment: "home")])
        #expect(store.catalog.sheet == nil)
    }

    @Test func aValueChangeAsksForAPreviewAndStartsOnCatalogOnly() async {
        let (store, bridge) = await makeStore()
        await beginRename(store, bridge)
        let previews = await bridge.previews, saves = await bridge.projectSaves
        #expect(previews.count == 1 && saves.isEmpty, "a preview writes nothing")
        guard case .rename(let preview)? = store.catalog.sheet?.step else {
            Issue.record("the sheet should show the rename step")
            return
        }
        #expect(preview.value == "job")
        #expect(store.catalog.sheet?.mode == .catalogOnly && store.catalog.sheet?.includeSubprojects == false)

        await store.catalog.confirmRename()
        let save = await bridge.projectSaves.first
        #expect(save?.action == "edit" && save?.source == "work" && save?.segment == "job")
        #expect(save?.migrate == false && save?.tasks == [] && save?.includeSubprojects == false)
    }

    @Test func catalogAndTasksSendsTheTasksTheChosenScopeShows() async {
        let (store, bridge) = await makeStore()
        await beginRename(store, bridge)
        store.catalog.sheet?.mode = .catalogAndTasks
        await store.catalog.confirmRename()
        var save = await bridge.projectSaves.last
        #expect(save?.migrate == true && save?.tasks == [move("1", "work", "job")], "with subprojects off, the exact tasks")

        await beginRename(store, bridge)
        store.catalog.sheet?.mode = .catalogAndTasks
        store.catalog.sheet?.includeSubprojects = true
        await store.catalog.confirmRename()
        save = await bridge.projectSaves.last
        #expect(save?.includeSubprojects == true && save?.tasks.count == 2, "with subprojects on, the descendants' tasks too")
    }

    @Test func movingTasksOntoAValueTasksUseNeedsTheMergeAgreement() async {
        let (store, bridge) = await makeStore()
        await beginRename(store, bridge, destinationTaskOnly: true)
        #expect(store.catalog.sheet?.canConfirmRename == true, "catalog only has nothing to merge")

        store.catalog.sheet?.mode = .catalogAndTasks
        #expect(store.catalog.sheet?.canConfirmRename == false)
        await store.catalog.confirmRename()
        #expect(await bridge.projectSaves.isEmpty, "no save without the agreement")

        store.catalog.sheet?.mergeConfirmed = true
        #expect(store.catalog.sheet?.canConfirmRename == true)
        await store.catalog.confirmRename()
        #expect(await bridge.projectSaves.count == 1)
    }

    @Test func movedTasksCloseTheUndoWindowAndACatalogEditDoesNot() async {
        let (store, bridge) = await makeStore()
        await store.complete(.fixture("a"))
        #expect(store.undoClosesAt != nil, "a completed task can be undone")

        store.catalog.beginEdit(row("home"))
        store.catalog.sheet?.name = "House"
        await store.catalog.submit()
        #expect(store.undoClosesAt != nil, "a catalog edit leaves the undo window alone")

        await bridge.setProjectSave(ProjectSaveResult(revision: "p3", changed: 2))
        await beginRename(store, bridge)
        store.catalog.sheet?.mode = .catalogAndTasks
        await store.catalog.confirmRename()
        #expect(store.undoClosesAt == nil, "no undo reverses a migration")
        #expect(store.toast?.message == "Moved 2 tasks to “job”")
        #expect(store.catalog.sheet == nil)
    }

    @Test func aStalePreviewGoesBackToTheFormAndMovesNothing() async {
        let (store, bridge) = await makeStore()
        await beginRename(store, bridge)
        store.catalog.sheet?.mode = .catalogAndTasks
        await bridge.setProjectFailure(BridgeError(message: "Your tasks changed since the preview. Preview the rename again.", code: "stalePreview"))
        await store.catalog.confirmRename()

        #expect(store.catalog.sheet?.step == .form)
        #expect(store.catalog.sheet?.notice == "Your tasks changed since the preview. Preview the rename again.")
        #expect(store.undoClosesAt == nil && store.toast?.message != "Moved 0 tasks to “job”")
    }

    @Test func aRefusedMigrationKeepsTheSheetOpenWithTheReason() async {
        let (store, bridge) = await makeStore()
        await beginRename(store, bridge)
        store.catalog.sheet?.mode = .catalogAndTasks
        await bridge.setProjectSave(ProjectSaveResult(revision: "p2", changed: 0, taskError: "the server did not answer"))
        await store.catalog.confirmRename()

        #expect(store.catalog.sheet?.step == .partial("the server did not answer"))
        #expect(await bridge.calls.filter { $0 == "projects" }.count == 2, "the saved catalog is read back")
    }

    @Test func removingAsksAboutTheChildrenAndSendsTheAnswer() async {
        let (store, bridge) = await makeStore()
        store.catalog.beginRemove(row("work"))
        #expect(store.catalog.removal?.children == 1, "work.client is in the catalog; work.legacy is only on tasks")
        await store.catalog.confirmRemoval(children: false)
        #expect(await bridge.projectSaves.last == ProjectEdit(revision: "p1", action: "remove", source: "work", removeChildren: false))
        #expect(store.catalog.removal == nil)

        store.catalog.beginRemove(row("work"))
        await store.catalog.confirmRemoval(children: true)
        #expect(await bridge.projectSaves.last?.removeChildren == true)

        store.catalog.beginRemove(row("home"))
        #expect(store.catalog.removal?.children == 0)
    }

    @Test func addToCatalogFillsTheSheetAndAddsNothingUntilItIsSaved() async {
        let (store, bridge) = await makeStore()
        store.catalog.beginAdd(from: ProjectRow(value: "side.client-name", name: "client-name", tag: "tasks", tasks: 1))
        let sheet = store.catalog.sheet
        #expect(sheet?.parent == "side" && sheet?.segment == "client-name" && sheet?.name == "Client Name")
        #expect(sheet?.value == "side.client-name")
        #expect(await bridge.projectSaves.isEmpty)
    }

    @Test func aFileThatChangedElsewhereClosesTheSheetAndSaysSo() async {
        let (store, bridge) = await makeStore()
        await bridge.setProjectFailure(BridgeError(message: "config.toml changed outside Momentum. The tab now shows what it says; make the change again.", code: "configConflict"))
        store.catalog.beginAdd()
        store.catalog.sheet?.name = "Acme"
        store.catalog.sheet?.segment = "acme"
        await store.catalog.submit()

        #expect(store.catalog.sheet == nil)
        #expect(store.catalog.notice?.contains("changed outside Momentum") == true)
        #expect(await bridge.calls.filter { $0 == "projects" }.count == 2, "the tab shows what the file says now")
    }

    @Test func anEntryCannotMoveUnderItselfOrWhatIsBelowIt() async {
        let (store, _) = await makeStore()
        store.catalog.beginEdit(row("work"))
        let options = store.catalog.parentOptions(for: store.catalog.sheet!)
        #expect(options == ["home"], "not work, and not anything below it, whether in the catalog or only on tasks")
        store.catalog.beginAdd()
        #expect(store.catalog.parentOptions(for: store.catalog.sheet!) == ["home", "work", "work.client", "work.legacy"])
    }

    @Test func valuesSplitAtTheirLastDot() {
        #expect(ProjectsModel.split("work.client.acme") == ("work.client", "acme"))
        #expect(ProjectsModel.split("home") == ("", "home"))
        #expect(ProjectsModel.suggestedName("client-name") == "Client Name")
    }

    @Test func theBridgesAnswersDecodeAndAnEditEncodesWhatItReads() throws {
        let decoder = JSONDecoder()
        let list = try decoder.decode(ProjectsSnapshot.self, from: Data("""
        {"revision":"abc","entries":[{"value":"work","name":"Work","tag":"catalog","tasks":4},
          {"value":"work.legacy","name":"legacy","tag":"tasks","tasks":2}],"tasksError":"the server did not answer"}
        """.utf8))
        #expect(list.entries.map(\.tag) == ["catalog", "tasks"] && list.tasksError == "the server did not answer")
        let preview = try decoder.decode(ProjectPreview.self, from: Data("""
        {"value":"job","valueChanged":true,"catalogDescendants":1,
         "exactTasks":[{"uuid":"1","oldValue":"work","newValue":"job"}],"subprojectTasks":[],"destinationTaskOnly":true}
        """.utf8))
        #expect(preview.exactTasks == [move("1", "work", "job")] && preview.destinationTaskOnly)
        let result = try decoder.decode(ProjectSaveResult.self, from: Data(#"{"revision":"r2","changed":0,"taskError":"x","stale":true}"#.utf8))
        #expect(result.taskError == "x" && result.stale == true)

        var edit = ProjectEdit(revision: "r1", action: "edit", source: "work", name: "Job", parent: "", segment: "job")
        edit.tasks = [move("1", "work", "job")]
        let json = String(decoding: try JSONEncoder().encode(edit), as: UTF8.self)
        for key in ["revision", "action", "source", "name", "parent", "segment", "includeSubprojects", "removeChildren", "migrate", "tasks", "oldValue", "newValue"] {
            #expect(json.contains("\"\(key)\""), "\(key) is a key the bridge reads")
        }
    }
}
