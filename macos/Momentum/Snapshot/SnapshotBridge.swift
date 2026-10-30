#if DEBUG
import Foundation

/// An in-memory stand-in for the Momentum server and the bridge. A snapshot
/// run uses it, so it reaches no server, reads no config file, and starts no
/// child process.
actor SnapshotBridge: TaskBridge {
    struct Config {
        /// nil shows the "Add a Momentum server" page.
        var backendURL: String? = "https://momentum.example.com"
        var seed = SnapshotSeed.make(now: .now)
        var statusFailure: BridgeError?
        /// Every load fails.
        var loadFailure: BridgeError?
        /// Loads after the first fail, as when the server goes away.
        var failLoadsAfterFirst: BridgeError?
        /// The first load never answers.
        var hangLoad = false
        var signInFailure: BridgeError?
        var applyPlanResult = PlanSaveResult(applied: 3, total: 3)
        var captureReview: Interpretation?
        var plan: DailyPlan?
        /// Why the newest config.toml was not applied.
        var configError: String?
        var restartRequired = false
        var planningEnabled = true
        var dailyCapacity = "8h"
        var buffer = "1h"
        var weekdayCapacity = ["friday": "4h"]
        /// Which source config.toml names, "files" or "google".
        var calendarSource = "files"
        /// What the server says of this account's Google Calendar.
        var googleStatus = GoogleCalendarStatus(offered: true, connected: false, email: nil, needsReconnect: false)
        /// How long the link a connect gets works, in seconds.
        var googleLinkLifetime: TimeInterval = 600
        /// Whether Google can be asked to revoke the access on a disconnect.
        var googleRevoked = true
        var calendarEnabled = true
        var calendarFiles = SnapshotBridge.defaultCalendarFiles
        var calendarHours = (start: "09:00", end: "17:00")
        var includeAllDay = false
        var includeTransparent = false
        var staleAfter = "24h"
        var doctor = SnapshotBridge.defaultDoctor
        var projectRows = SnapshotBridge.defaultProjectRows
        /// Why the task counts are missing.
        var projectsTasksError: String?
    }

    /// Every check passing, as on a Mac that works.
    static let defaultDoctor = DoctorReport(
        configPath: "~/.config/momentum/config.toml", bridgeVersion: "v0.1.0-12-g4bafc30",
        checks: [
            DoctorCheck(name: "Backend server", status: "ok", detail: "https://momentum.example.com answers; version 1.4.0"),
            DoctorCheck(name: "Sign-in", status: "ok", detail: "joao@example.com until 2026-12-01"),
            DoctorCheck(name: "Time zone", status: "ok", detail: "Europe/Lisbon"),
            DoctorCheck(name: "Momentum config", status: "ok", detail: "~/.config/momentum/config.toml"),
            DoctorCheck(name: "Calendar source", status: "ok", detail: "1 local ICS source(s); read-only"),
        ],
        ready: true)

    /// A catalog with children, and two values only tasks have, in path order.
    static let defaultProjectRows = [
        ProjectRow(value: "home", name: "Home", tag: "catalog", tasks: 2),
        ProjectRow(value: "side.gig", name: "gig", tag: "tasks", tasks: 1),
        ProjectRow(value: "work", name: "Work", tag: "catalog", tasks: 4),
        ProjectRow(value: "work.client", name: "Client", tag: "catalog", tasks: 3),
        ProjectRow(value: "work.client.acme", name: "Acme", tag: "catalog", tasks: 1),
        ProjectRow(value: "work.legacy", name: "legacy", tag: "tasks", tasks: 2),
    ]

    /// The four states a calendar file can be in.
    static let defaultCalendarFiles = [
        CalendarFile(path: "~/Calendars/work.ics", status: "available", detail: nil, modified: .now.addingTimeInterval(-3600)),
        CalendarFile(path: "~/Calendars/team.ics", status: "stale", detail: nil, modified: .now.addingTimeInterval(-3 * 86400)),
        CalendarFile(path: "~/Downloads/old-export.ics", status: "missing", detail: nil, modified: nil),
        CalendarFile(path: "~/Calendars/shared", status: "error", detail: "is a directory", modified: .now.addingTimeInterval(-86400)),
    ]

    private var config: Config
    /// Bumped by each save, as a new version of config.toml is.
    private var revision = 1
    private var loads = 0
    private var completed: [String: Date] = [:]
    private var deleted: Set<String> = []
    private var started: Set<String> = []
    private var stopped: Set<String> = []

    init(_ config: Config) {
        self.config = config
    }

    func status() async throws -> BridgeStatus {
        if let failure = config.statusFailure { throw failure }
        return BridgeStatus(
            configPath: "~/.config/momentum/config.toml", mutationDelaySeconds: 15, refreshIntervalSeconds: 0, backendUrl: config.backendURL,
            configError: config.configError, restartRequired: config.restartRequired)
    }

    func load(calendar: CalendarInput?) async throws -> Snapshot {
        loads += 1
        if config.hangLoad { try await Task.sleep(for: .seconds(3600)) }
        if let failure = config.loadFailure { throw failure }
        if loads > 1, let failure = config.failLoadsAfterFirst { throw failure }
        var snapshot = snapshot()
        if let error = calendar?.error {
            snapshot.plan?.calendarMinutes = 0
            snapshot.plan?.calendarAvailable = false
            snapshot.plan?.calendarError = error
        }
        return snapshot
    }

    /// The seed with the changes made since: completed and deleted tasks move
    /// or leave, started tasks turn active.
    private func snapshot() -> Snapshot {
        let seed = config.seed
        func keep(_ sections: [TaskSection]) -> [TaskSection] {
            sections.compactMap { section in
                let uuids = section.uuids.filter { completed[$0] == nil && !deleted.contains($0) }
                return uuids.isEmpty ? nil : TaskSection(title: section.title, project: section.project, uuids: uuids)
            }
        }
        let tasks = seed.tasks.filter { !deleted.contains($0.uuid) }.map { task -> TaskItem in
            var task = task
            if let end = completed[task.uuid] {
                task.status = "completed"
                task.end = end
            }
            if started.contains(task.uuid) { task.start = .now }
            if stopped.contains(task.uuid) { task.start = nil }
            return task
        }
        var done = seed.completed
        if !completed.isEmpty {
            let newest = completed.sorted { $0.value > $1.value }.map(\.key)
            if let index = done.firstIndex(where: { $0.title == "Today" }) {
                done[index].uuids = newest + done[index].uuids
            } else {
                done.insert(TaskSection(title: "Today", project: nil, uuids: newest), at: 0)
            }
        }
        return Snapshot(
            tasks: tasks, today: keep(seed.today), inbox: keep(seed.inbox), completed: done, completedError: nil,
            projects: seed.projects, plan: planSummary(seed.plan))
    }

    /// The sidebar's meter, with the capacity and buffer the settings give,
    /// or nothing while Plan Today is off.
    private func planSummary(_ seeded: PlanSummary) -> PlanSummary {
        guard config.planningEnabled else {
            return PlanSummary(
                enabled: false, chosenCount: 0, chosenMinutes: 0, unestimatedChosen: 0, fixedMinutes: 0, unestimatedFixed: 0,
                capacityMinutes: 0, bufferMinutes: 0)
        }
        var summary = seeded
        let today = Calendar.current.weekdaySymbols[Calendar.current.component(.weekday, from: .now) - 1].lowercased()
        summary.capacityMinutes = Self.minutes(config.weekdayCapacity[today] ?? config.dailyCapacity) ?? seeded.capacityMinutes
        summary.bufferMinutes = Self.minutes(config.buffer) ?? seeded.bufferMinutes
        return summary
    }

    /// 8h, 7h30m and 45m as minutes; nil for anything else.
    static func minutes(_ text: String) -> Int? {
        var total = 0, number = ""
        for character in text.trimmingCharacters(in: .whitespaces) {
            if character.isNumber {
                number.append(character)
            } else if let value = Int(number), character == "h" || character == "m" {
                total += character == "h" ? value * 60 : value
                number = ""
            } else {
                return nil
            }
        }
        return number.isEmpty && total > 0 ? total : nil
    }

    // MARK: Capture

    /// Phrases the fake interpreter recognizes, as the Go interpreter would
    /// for the sentences the snapshot pages type.
    private static let phrases: [(text: String, field: String, value: String, provenance: String)] = [
        ("tomorrow at 3pm", "due", "tomorrow 15:00", "inferred"),
        ("every Friday", "recurrence", "weekly", "inferred"),
        ("about 1h", "estimate", "60", "inferred"),
        ("#work.client", "project", "work.client", "explicit"),
    ]

    func interpret(_ input: String, literals: [TextSpan]) async throws -> Interpretation {
        let scalars = Array(input.unicodeScalars)
        var candidates: [Candidate] = []
        for phrase in Self.phrases {
            let needle = Array(phrase.text.unicodeScalars)
            guard let start = Self.index(of: needle, in: scalars) else { continue }
            candidates.append(Candidate(
                field: phrase.field, text: phrase.text, value: phrase.value, start: start, end: start + needle.count,
                provenance: phrase.provenance, accepted: true, conflict: false, blocking: false))
        }
        var rest = scalars
        for candidate in candidates.sorted(by: { $0.start > $1.start }) {
            rest.removeSubrange(candidate.start..<candidate.end)
        }
        var description = ""
        description.unicodeScalars.append(contentsOf: rest)
        description = description.replacingOccurrences(of: ",", with: "").split(separator: " ").joined(separator: " ")
        return Interpretation(
            token: Int64(input.count), source: input, valid: true, requiresReview: false, description: description,
            candidates: candidates, diagnostics: [], reviewValues: [])
    }

    /// Suggests projects for a `#` token that ends at the cursor.
    func suggest(_ input: String, cursor: Int) async throws -> SuggestResult {
        let scalars = Array(input.unicodeScalars)
        let end = min(cursor, scalars.count)
        guard let hash = scalars[..<end].lastIndex(of: "#"), !scalars[hash..<end].contains(" ") else { return .none }
        let typed = String(String.UnicodeScalarView(scalars[(hash + 1)..<end])).lowercased()
        let matches = config.seed.projects.filter { $0.value.lowercased().contains(typed) }
        guard !matches.isEmpty else { return .none }
        let suggestions = matches.map { project -> Suggestion in
            var replaced = Array(scalars[..<(hash + 1)])
            replaced += Array(project.value.unicodeScalars) + Array(scalars[end...])
            var text = ""
            text.unicodeScalars.append(contentsOf: replaced)
            return Suggestion(
                value: project.value, text: "#\(project.value)", label: project.label, input: text,
                cursor: hash + 1 + project.value.unicodeScalars.count)
        }
        return SuggestResult(active: true, kind: "project", start: hash, end: end, suggestions: suggestions)
    }

    func capture(_ input: String, literals: [TextSpan]) async throws -> CaptureResult {
        if let review = config.captureReview, input == review.source { return CaptureResult(added: false, review: review) }
        return CaptureResult(added: true, review: nil)
    }

    func captureReview(_ submission: ReviewSubmission) async throws {}
    func captureExplicit(_ input: String) async throws {}

    func modify(uuid: String, before: EditFields, after: EditFields) async throws -> Bool { before != after }

    // No page shows what a note, a dependency or a copy changes after it is made.
    func addNote(uuid: String, text: String) async throws {}
    func removeNote(uuid: String, note: Annotation) async throws {}
    func addDependency(uuid: String, on: String) async throws {}
    func removeDependency(uuid: String, on: String) async throws {}
    func duplicate(uuid: String) async throws -> String { uuid }

    func perform(_ action: TaskAction, uuid: String) async throws {
        switch action {
        case .complete: completed[uuid] = .now
        case .delete: deleted.insert(uuid)
        case .start:
            started.insert(uuid)
            stopped.remove(uuid)
        case .stop:
            stopped.insert(uuid)
            started.remove(uuid)
        case .stopRecurrence: break
        }
    }

    func undo() async throws {}

    // MARK: Plan Today

    func plan(calendar: CalendarInput?) async throws -> DailyPlan {
        var plan = config.plan ?? config.seed.dailyPlan
        if let error = calendar?.error {
            plan.calendar = PlanCalendar(
                enabled: true, available: false, stale: false, error: error, busyMinutes: 0, eventCount: 0,
                workingStart: config.calendarHours.start, workingEnd: config.calendarHours.end, blocks: [])
        }
        return plan
    }

    func applyPlan(date: String, uuids: [String]) async throws -> PlanSaveResult { config.applyPlanResult }

    // MARK: Settings

    private func settingsSnapshot() -> SettingsSnapshot {
        SettingsSnapshot(
            configPath: "~/.config/momentum/config.toml", revision: "r\(revision)", configError: config.configError,
            restartRequired: config.restartRequired,
            planning: PlanningSettings(
                enabled: config.planningEnabled, dailyCapacity: config.dailyCapacity, buffer: config.buffer,
                weekdayCapacity: config.weekdayCapacity),
            calendar: CalendarSettings(
                source: config.calendarSource, enabled: config.calendarEnabled, files: config.calendarFiles, workingStart: config.calendarHours.start,
                workingEnd: config.calendarHours.end, includeAllDay: config.includeAllDay, includeTransparent: config.includeTransparent,
                staleAfter: config.staleAfter))
    }

    func settings() async throws -> SettingsSnapshot { settingsSnapshot() }

    /// Changes the daily capacity as an editor would, so the next save from a
    /// tab that read the file before is refused.
    func editedElsewhere(dailyCapacity: String) {
        config.dailyCapacity = dailyCapacity
        revision += 1
    }

    /// Refuses what the real bridge refuses, with its words.
    func savePlanning(_ change: PlanningChange) async throws -> SettingsSnapshot {
        guard change.revision == "r\(revision)" else { throw Self.conflict }
        let lengthHint = "Enter a length of time, such as 8h or 7h30m."
        if let text = change.dailyCapacity, Self.minutes(text) == nil {
            throw BridgeError(message: lengthHint, field: "dailyCapacity")
        }
        if let text = change.buffer, Self.minutes(text) == nil, text != "0m" {
            throw BridgeError(message: lengthHint, field: "buffer")
        }
        for (day, text) in change.weekdayCapacity ?? [:] where !text.isEmpty && Self.minutes(text) == nil {
            throw BridgeError(message: lengthHint, field: "weekdayCapacity.\(day)")
        }
        if let enabled = change.enabled { config.planningEnabled = enabled }
        if let text = change.dailyCapacity { config.dailyCapacity = text }
        if let text = change.buffer { config.buffer = text }
        for (day, text) in change.weekdayCapacity ?? [:] { config.weekdayCapacity[day] = text.isEmpty ? nil : text }
        revision += 1
        return settingsSnapshot()
    }

    func saveCalendar(_ change: CalendarChange) async throws -> SettingsSnapshot {
        guard change.revision == "r\(revision)" else { throw Self.conflict }
        let paths = change.paths ?? config.calendarFiles.map(\.path)
        let source = change.source ?? config.calendarSource
        if source != "files", source != "google" {
            throw BridgeError(message: "Choose Calendar files or Google Calendar.", field: "source")
        }
        // Files are only checked when they are the source.
        if source == "files", (change.enabled ?? config.calendarEnabled) && paths.isEmpty {
            if change.source != nil {
                throw BridgeError(message: "Add a calendar file first, or turn off Use calendar files.", field: "source")
            }
            throw BridgeError(
                message: change.enabled == nil ? "Turn off Use calendar files before removing the last one." : "Add a calendar file first.",
                field: change.enabled == nil ? "paths" : "enabled")
        }
        let start = change.workingStart ?? config.calendarHours.start, end = change.workingEnd ?? config.calendarHours.end
        if (change.workingStart != nil || change.workingEnd != nil) && start >= end {
            throw BridgeError(message: "Working hours must end after they start.", field: "workingHours")
        }
        if let text = change.staleAfter, Self.minutes(text) == nil, text != "0m" {
            throw BridgeError(message: "Enter a length of time, such as 8h or 7h30m.", field: "staleAfter")
        }
        config.calendarSource = source
        config.calendarHours = (start, end)
        if let include = change.includeAllDay { config.includeAllDay = include }
        if let include = change.includeTransparent { config.includeTransparent = include }
        if let text = change.staleAfter { config.staleAfter = text }
        if let enabled = change.enabled { config.calendarEnabled = enabled }
        if change.paths != nil {
            config.calendarFiles = paths.map { path in
                config.calendarFiles.first { $0.path == path }
                    ?? CalendarFile(path: path, status: "available", detail: nil, modified: .now)
            }
        }
        revision += 1
        return settingsSnapshot()
    }

    // MARK: Google Calendar

    func calendarStatus() async throws -> GoogleCalendarStatus { config.googleStatus }

    /// A link that works, and a status that does not change: the page that
    /// waits for the browser keeps waiting.
    func connectCalendar() async throws -> CalendarConnectLink {
        CalendarConnectLink(
            url: "https://momentum.example.com/calendar/connect/snapshot", expiresAt: .now.addingTimeInterval(config.googleLinkLifetime))
    }

    /// The account is no longer connected, and the server says whether Google
    /// could be asked to revoke the access.
    func disconnectCalendar() async throws -> GoogleCalendarDisconnect {
        config.googleStatus = GoogleCalendarStatus(offered: config.googleStatus.offered, connected: false, email: nil, needsReconnect: false)
        return GoogleCalendarDisconnect(revoked: config.googleRevoked)
    }

    // MARK: Projects

    func doctor() async throws -> DoctorReport { config.doctor }

    func projects() async throws -> ProjectsSnapshot {
        ProjectsSnapshot(revision: "r\(revision)", entries: config.projectRows, tasksError: config.projectsTasksError)
    }

    /// Checks a draft as the real bridge does and counts the tasks a rename
    /// would move, from the counts the rows carry.
    func previewMigration(_ edit: ProjectEdit) async throws -> ProjectPreview {
        guard edit.revision == "r\(revision)" else { throw Self.conflict }
        let value = try checkedValue(edit)
        func moves(_ includeSubprojects: Bool) -> [ProjectTaskMove] {
            config.projectRows.flatMap { row -> [ProjectTaskMove] in
                guard row.value == edit.source || (includeSubprojects && row.value.hasPrefix(edit.source + ".")) else { return [] }
                let moved = value + row.value.dropFirst(edit.source.count)
                return (0..<row.tasks).map { ProjectTaskMove(uuid: "\(row.value)-\($0)", oldValue: row.value, newValue: moved) }
            }
        }
        return ProjectPreview(
            value: value, valueChanged: value != edit.source,
            catalogDescendants: config.projectRows.filter { $0.inCatalog && $0.value.hasPrefix(edit.source + ".") }.count,
            exactTasks: moves(false), subprojectTasks: moves(true),
            destinationTaskOnly: config.projectRows.contains { !$0.inCatalog && ($0.value == value || $0.value.hasPrefix(value + ".")) })
    }

    func saveProject(_ edit: ProjectEdit) async throws -> ProjectSaveResult {
        guard edit.revision == "r\(revision)" else { throw Self.conflict }
        if edit.action == "remove" {
            config.projectRows = config.projectRows.compactMap { row in
                guard row.value == edit.source || (edit.removeChildren && row.inCatalog && row.value.hasPrefix(edit.source + ".")) else { return row }
                // A value tasks still use stays, as one found only on tasks.
                return row.tasks > 0 ? ProjectRow(value: row.value, name: ProjectsModel.split(row.value).segment, tag: "tasks", tasks: row.tasks) : nil
            }
        } else {
            let value = try checkedValue(edit)
            let tasks = config.projectRows.first { $0.value == value }?.tasks ?? 0
            config.projectRows.removeAll { $0.value == value || $0.value == edit.source }
            config.projectRows.append(ProjectRow(value: value, name: edit.name, tag: "catalog", tasks: tasks))
            config.projectRows.sort { $0.value.split(separator: ".").map(String.init).lexicographicallyPrecedes($1.value.split(separator: ".").map(String.init)) }
        }
        revision += 1
        return ProjectSaveResult(revision: "r\(revision)", changed: 0)
    }

    private func checkedValue(_ edit: ProjectEdit) throws -> String {
        if edit.name.trimmingCharacters(in: .whitespaces).isEmpty { throw BridgeError(message: "Enter a name.", field: "name") }
        if edit.segment.range(of: "^[a-z0-9]+(?:-[a-z0-9]+)*$", options: .regularExpression) == nil {
            throw BridgeError(message: "Use lowercase letters, numbers and hyphens, such as client-name.", field: "segment")
        }
        let value = edit.parent.isEmpty ? edit.segment : "\(edit.parent).\(edit.segment)"
        if config.projectRows.contains(where: { $0.inCatalog && $0.value == value && $0.value != edit.source }) {
            throw BridgeError(message: "“\(value)” is already in the catalog.", field: "segment")
        }
        return value
    }

    private static let conflict = BridgeError(
        message: "config.toml changed outside Momentum. The tab now shows what it says; make the change again.", code: "configConflict")

    // MARK: Sign-in

    func setToken(_ token: String) async throws {}

    func startSignIn() async throws -> DeviceSignIn {
        if let failure = config.signInFailure { throw failure }
        return DeviceSignIn(
            deviceCode: "snapshot-device-code", userCode: "BCDF-GHJK", verificationUrl: "https://momentum.example.com/device",
            verificationUrlComplete: "https://momentum.example.com/device?code=BCDF-GHJK", expiresAt: .now.addingTimeInterval(600),
            intervalSeconds: 3600)
    }

    func pollSignIn(deviceCode: String) async throws -> SignInPoll { SignInPoll(state: "pending") }
    func revoke(token: String) async throws {}

    private static func index(of needle: [Unicode.Scalar], in haystack: [Unicode.Scalar]) -> Int? {
        guard !needle.isEmpty, haystack.count >= needle.count else { return nil }
        return (0...(haystack.count - needle.count)).first { haystack[$0..<($0 + needle.count)].elementsEqual(needle) }
    }
}

/// A sign-in store that lives in memory, so a snapshot run never touches the
/// Keychain.
final class SnapshotSignInStore: SignInStore, @unchecked Sendable {
    private let lock = NSLock()
    private var saved: [String: SavedSignIn]

    init(_ saved: [SavedSignIn] = []) {
        self.saved = Dictionary(uniqueKeysWithValues: saved.map { ($0.server, $0) })
    }

    func read(server: String) throws -> SavedSignIn? { lock.withLock { saved[server] } }
    func save(_ signIn: SavedSignIn) throws { lock.withLock { saved[signIn.server] = signIn } }
    func remove(server: String) throws { _ = lock.withLock { saved.removeValue(forKey: server) } }
}
#endif
