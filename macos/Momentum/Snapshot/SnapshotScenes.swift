#if DEBUG
import Foundation

/// One snapshot page: the world the fake bridge serves, and the steps that
/// bring the app to the state the page shows. Names match the `Screens:`
/// lines in docs/STORIES.md, and the images are `<name>-light.png` and
/// `<name>-dark.png`.
struct SnapshotScene {
    var name: String
    var config = SnapshotBridge.Config()
    var signedIn = true
    /// What the Mac answers when macOS Calendar asks for access.
    var macAccess = CalendarAccess.granted
    /// false takes the page while the first load is still running.
    var waitsForStart = true
    var run: @MainActor (SnapshotSession) async -> Void = { _ in }

    @MainActor
    static func all(now: Date) -> [SnapshotScene] {
        let seed = SnapshotSeed.make(now: now)
        let unreachable = BridgeError(message: "Momentum can’t reach https://momentum.example.com. Check your connection.", code: "unreachable")

        var empty = SnapshotBridge.Config()
        empty.seed = .empty

        var review = SnapshotBridge.Config()
        let source = "Pay the parking fine @friday ~90x"
        review.captureReview = Interpretation(
            token: 7, source: source, valid: false, requiresReview: true, description: "Pay the parking fine",
            candidates: [
                Candidate(field: "due", text: "@friday", value: "friday", start: 21, end: 28, provenance: "explicit", accepted: true, conflict: false, blocking: false),
                Candidate(
                    field: "estimate", text: "~90x", value: "90x", start: 29, end: 33, provenance: "explicit", accepted: false, conflict: false,
                    blocking: true, diagnostic: "“90x” isn’t an estimate. Use minutes or hours, for example 45m or 1h30m."),
            ],
            diagnostics: [Diagnostic(message: "“90x” isn’t an estimate. Use minutes or hours, for example 45m or 1h30m.", blocking: true)],
            reviewValues: ["Pay the parking fine", "20261030T090000", "", "90x", "", "", "", ""])

        var offline = SnapshotBridge.Config()
        offline.failLoadsAfterFirst = unreachable

        var partial = SnapshotBridge.Config()
        partial.applyPlanResult = PlanSaveResult(applied: 1, total: 3, error: "Momentum can’t reach https://momentum.example.com.")

        var noCalendar = SnapshotBridge.Config()
        var calendarLess = seed.dailyPlan
        calendarLess.calendar = PlanCalendar(
            enabled: true, available: false, stale: false, error: "~/Calendars/work.ics can’t be read.",
            busyMinutes: 0, eventCount: 0, workingStart: "09:00", workingEnd: "17:00", blocks: [])
        noCalendar.plan = calendarLess
        noCalendar.seed.plan.calendarMinutes = 0
        noCalendar.seed.plan.calendarAvailable = false
        noCalendar.seed.plan.calendarError = "~/Calendars/work.ics can’t be read."

        var noFiles = SnapshotBridge.Config()
        noFiles.calendarEnabled = false
        noFiles.calendarFiles = []

        var noProjects = SnapshotBridge.Config()
        noProjects.projectRows = []

        var badConfig = SnapshotBridge.Config()
        badConfig.configError = "config key \"calendar.paths\" requires at least one local ICS path"
        var movedServer = SnapshotBridge.Config()
        movedServer.restartRequired = true

        return [
            // First run, signing in, and the states of the first load.
            SnapshotScene(name: "setup", config: { var c = SnapshotBridge.Config(); c.backendURL = nil; return c }(), signedIn: false),
            SnapshotScene(name: "signed-out", signedIn: false),
            SnapshotScene(name: "sign-in", signedIn: false, run: { $0.store.beginSignIn(); await $0.settle(900) }),
            SnapshotScene(
                name: "sign-in-failed",
                config: { var c = SnapshotBridge.Config(); c.signInFailure = BridgeError(message: "The server didn’t answer the sign-in request."); return c }(),
                signedIn: false, run: { $0.store.beginSignIn(); await $0.settle(900) }),
            SnapshotScene(name: "loading", config: { var c = SnapshotBridge.Config(); c.hangLoad = true; return c }(), waitsForStart: false),
            SnapshotScene(name: "load-error", config: { var c = SnapshotBridge.Config(); c.loadFailure = unreachable; return c }()),
            SnapshotScene(name: "startup-error", config: { var c = SnapshotBridge.Config(); c.statusFailure = BridgeError(message: "momentum-bridge exited unexpectedly."); return c }()),

            // The views.
            SnapshotScene(name: "today"),
            SnapshotScene(name: "inbox", run: { $0.store.view = .inbox }),
            SnapshotScene(name: "completed", run: { $0.store.view = .completed }),
            SnapshotScene(name: "today-empty", config: empty),
            SnapshotScene(name: "inbox-empty", config: empty, run: { $0.store.view = .inbox }),
            SnapshotScene(name: "completed-empty", config: empty, run: { $0.store.view = .completed }),
            SnapshotScene(name: "inspector", run: { $0.store.selection = $0.task("Send Q3 invoice").uuid }),
            SnapshotScene(name: "search", run: { $0.store.view = .inbox; $0.store.query = "client" }),

            // Changes, and what a person is asked on the way.
            SnapshotScene(name: "undo", run: { await $0.store.complete($0.task("Review pull request")) }),
            SnapshotScene(name: "unreachable", config: offline, run: { await $0.store.refresh(announce: true) }),
            SnapshotScene(name: "edit", run: { $0.store.editing = EditRequest(task: $0.task("Prepare the Northwind"), focus: .description) }),
            SnapshotScene(name: "delete", run: { $0.store.pendingDelete = $0.task("Review pull request") }),
            SnapshotScene(
                name: "stop-repeating", run: { $0.store.view = .inbox; $0.store.pendingStopRecurrence = $0.task("Water the plants") }),
            SnapshotScene(name: "shortcuts", run: { $0.store.showShortcuts = true }),

            // Adding a task.
            SnapshotScene(name: "capture", run: { $0.store.showCapture = true }),
            SnapshotScene(
                name: "capture-typed",
                run: { $0.store.showCapture = true; await $0.settle(); await $0.type("Prepare proposal tomorrow at 3pm, every Friday, about 1h #work.client") }),
            SnapshotScene(
                name: "capture-suggestions",
                run: { $0.store.showCapture = true; await $0.settle(); await $0.type("Call Sam about the invoice #wo") }),
            SnapshotScene(
                name: "capture-review", config: review,
                run: { $0.store.showCapture = true; await $0.settle(); await $0.type(source); await $0.pressReturn() }),

            // Settings, and the appearance a person chooses.
            SnapshotScene(name: "settings-projects", run: { await $0.openProjects() }),
            SnapshotScene(name: "settings-projects-empty", config: noProjects, run: { await $0.openProjects() }),
            SnapshotScene(
                name: "settings-project-sheet",
                run: {
                    await $0.openProjects()
                    $0.store.catalog.beginAdd()
                    $0.store.catalog.sheet?.name = "Acme Corp"
                    $0.store.catalog.sheet?.parent = "work.client"
                    $0.store.catalog.sheet?.segment = "acme-corp"
                }),
            SnapshotScene(
                name: "settings-project-add-from-tasks",
                run: { await $0.openProjects(); $0.store.catalog.beginAdd(from: $0.projectRow("side.gig")) }),
            SnapshotScene(
                name: "settings-project-remove", run: { await $0.openProjects(); $0.store.catalog.beginRemove($0.projectRow("work")) }),
            SnapshotScene(
                name: "settings-project-rename",
                run: {
                    await $0.openProjects()
                    $0.store.catalog.beginEdit($0.projectRow("work"))
                    $0.store.catalog.sheet?.name = "Job"
                    $0.store.catalog.sheet?.segment = "job"
                    await $0.store.catalog.submit()
                    $0.store.catalog.sheet?.mode = .catalogAndTasks
                    $0.store.catalog.sheet?.includeSubprojects = true
                }),
            SnapshotScene(
                name: "settings-project-rename-merge",
                run: {
                    await $0.openProjects()
                    $0.store.catalog.beginEdit($0.projectRow("home"))
                    $0.store.catalog.sheet?.parent = "side"
                    $0.store.catalog.sheet?.segment = "gig"
                    await $0.store.catalog.submit()
                    $0.store.catalog.sheet?.mode = .catalogAndTasks
                }),
            SnapshotScene(name: "settings-appearance", run: { await $0.openSettings(.appearance) }),
            SnapshotScene(name: "settings-reopened", run: { await $0.reopenSettings(.planning) }),
            SnapshotScene(name: "settings-planning", run: { await $0.openSettings(.planning) }),
            SnapshotScene(
                name: "settings-planning-error",
                run: {
                    await $0.openSettings(.planning)
                    await $0.typeInField(at: 0, "a day")
                    await $0.typeInField(at: 2, "half")
                }),
            SnapshotScene(
                name: "settings-planning-off",
                run: { await $0.openSettings(.planning); await $0.store.settings.setPlanningEnabled(false); await $0.settle() }),
            SnapshotScene(name: "today-planning-off", run: { await $0.store.settings.reload(); await $0.store.settings.setPlanningEnabled(false) }),
            SnapshotScene(name: "settings-calendar", run: { await $0.openSettings(.calendar) }),
            SnapshotScene(name: "settings-calendar-empty", config: noFiles, run: { await $0.openSettings(.calendar) }),
            SnapshotScene(
                name: "settings-calendar-refused", config: noFiles,
                run: { await $0.openSettings(.calendar); await $0.store.settings.setCalendarEnabled(true); await $0.settle() }),
            SnapshotScene(
                name: "settings-calendar-mac", run: { await $0.store.macCalendar.choose(.mac); await $0.openSettings(.calendar) }),
            SnapshotScene(
                name: "settings-calendar-denied", macAccess: .denied,
                run: { await $0.store.macCalendar.choose(.mac); await $0.openSettings(.calendar) }),
            SnapshotScene(name: "settings-config-error", config: badConfig, run: { await $0.openSettings(.planning) }),
            SnapshotScene(name: "settings-restart", config: movedServer, run: { await $0.openSettings(.planning) }),
            SnapshotScene(name: "appearance-light", run: { await $0.chooseAppearance(.light) }),
            SnapshotScene(name: "appearance-dark", run: { await $0.chooseAppearance(.dark) }),
            SnapshotScene(
                name: "appearance-dark-delete",
                run: { await $0.chooseAppearance(.dark); $0.store.pendingDelete = $0.task("Review pull request") }),

            // Plan Today.
            SnapshotScene(name: "plan-today", run: { await $0.store.openPlan() }),
            SnapshotScene(name: "plan-today-filter", run: { await $0.store.openPlan(); $0.store.planSheet?.filter = "read" }),
            SnapshotScene(
                name: "plan-today-over",
                run: {
                    await $0.store.openPlan()
                    for item in $0.store.planSheet?.plan.candidates ?? [] where !($0.store.planSheet?.isSelected(item) ?? true) {
                        $0.store.planSheet?.toggle(item)
                    }
                }),
            SnapshotScene(
                name: "plan-today-partial", config: partial,
                run: {
                    await $0.store.openPlan()
                    $0.store.planSheet?.toggle(PlanItem(task: $0.task("Read the Northwind"), selected: false, plannedDate: nil))
                    await $0.store.confirmPlan()
                }),
            SnapshotScene(name: "plan-today-calendar-unavailable", config: noCalendar, run: { await $0.store.openPlan() }),
            SnapshotScene(
                name: "plan-today-calendar-denied", macAccess: .denied,
                run: { await $0.store.macCalendar.choose(.mac); await $0.store.openPlan() }),
        ]
    }
}
#endif
