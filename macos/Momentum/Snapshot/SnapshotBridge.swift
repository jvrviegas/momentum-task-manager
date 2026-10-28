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
    }

    private var config: Config
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
            configPath: "~/.config/momentum/config.toml", mutationDelaySeconds: 15, refreshIntervalSeconds: 0, backendUrl: config.backendURL)
    }

    func load() async throws -> Snapshot {
        loads += 1
        if config.hangLoad { try await Task.sleep(for: .seconds(3600)) }
        if let failure = config.loadFailure { throw failure }
        if loads > 1, let failure = config.failLoadsAfterFirst { throw failure }
        return snapshot()
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
            projects: seed.projects, plan: seed.plan)
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

    func plan() async throws -> DailyPlan { config.plan ?? config.seed.dailyPlan }

    func applyPlan(date: String, uuids: [String]) async throws -> PlanSaveResult { config.applyPlanResult }

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
