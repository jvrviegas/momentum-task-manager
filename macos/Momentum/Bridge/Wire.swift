import Foundation

// Mirrors internal/bridge/wire.go. Text offsets count Unicode scalars, the
// same unit as Go runes.

struct BridgeStatus: Decodable, Sendable, Equatable {
    var configPath: String
    /// How long the latest change can be undone.
    var mutationDelaySeconds: Int
    var refreshIntervalSeconds: Int
    /// The [backend] server; nil until config.toml names one.
    var backendUrl: String?
}

/// A started device sign-in: show userCode, open verificationUrlComplete, and
/// poll with deviceCode every intervalSeconds until expiresAt.
struct DeviceSignIn: Decodable, Sendable, Equatable {
    var deviceCode: String
    var userCode: String
    var verificationUrl: String
    var verificationUrlComplete: String
    var expiresAt: Date
    var intervalSeconds: Int
}

/// One poll's answer: state is "pending", "slowDown" (wait five seconds
/// longer between polls), or "signedIn" with the sign-in.
struct SignInPoll: Decodable, Sendable, Equatable {
    var state: String
    var token: String?
    var email: String?
    var expiresAt: Date?
}

struct Snapshot: Decodable, Sendable, Equatable {
    var tasks: [TaskItem]
    var today: [TaskSection]
    var inbox: [TaskSection]
    var completed: [TaskSection]
    var completedError: String?
    var projects: [ProjectEntry]
    var plan: PlanSummary?
}

/// The sidebar's "Today's plan" box. Chosen tasks are the ones tagged for
/// today outside the obligations, the Planned Today section; fixed sums the
/// obligations. Everything is zero when planning is off.
struct PlanSummary: Decodable, Sendable, Equatable {
    var enabled: Bool
    var chosenCount: Int
    var chosenMinutes: Int
    var unestimatedChosen: Int
    var fixedMinutes: Int
    var unestimatedFixed: Int
    var capacityMinutes: Int
    var bufferMinutes: Int
}

/// Today's plan for the Plan Today sheet. date is the local day as
/// YYYY-MM-DD, and tag the task tag that marks it.
struct DailyPlan: Decodable, Sendable, Equatable {
    var date: String
    var tag: String
    var capacityMinutes: Int
    var bufferMinutes: Int
    var obligations: [PlanItem]
    var rollover: [PlanItem]
    var candidates: [PlanItem]
}

struct PlanItem: Decodable, Sendable, Hashable, Identifiable {
    var task: TaskItem
    var selected: Bool
    /// The earlier day, as YYYY-MM-DD, a rollover task was planned for.
    var plannedDate: String?

    var id: String { task.uuid }
}

/// How many of a saved plan's tag changes were applied. A total of 0 means
/// the plan was unchanged; error says why it stopped after applied of them.
struct PlanSaveResult: Decodable, Sendable, Equatable {
    var applied: Int
    var total: Int
    var error: String?
}

struct TaskSection: Decodable, Sendable, Hashable {
    var title: String
    var project: String?
    var uuids: [String]
}

struct ProjectEntry: Decodable, Sendable, Hashable {
    var value: String
    var label: String?
}

struct TaskItem: Decodable, Sendable, Hashable, Identifiable {
    var uuid: String
    var number: Int
    var description: String
    var status: String
    var project: String?
    var priority: String?
    var estimateMinutes: Int?
    var estimate: String?
    var estimateWarning: String?
    var estimateRaw: String?
    var entry: Date?
    var end: Date?
    var modified: Date?
    var due: Date?
    var scheduled: Date?
    var start: Date?
    var wait: Date?
    var until: Date?
    var tags: [String]
    var annotations: [Annotation]
    var depends: [String]
    var recur: String?
    var recurDisplay: String?
    var parent: String?
    var recurrenceTarget: String?
    var urgency: Double
    var todayGroup: String?
    var edit: EditFields
    var extra: [String: String]?

    var id: String { uuid }
    var isPending: Bool { status == "pending" }
    var isCompleted: Bool { status == "completed" }

    enum CodingKeys: String, CodingKey {
        case uuid, number = "id", description, status, project, priority
        case estimateMinutes, estimate, estimateWarning, estimateRaw
        case entry, end, modified, due, scheduled, start, wait, until
        case tags, annotations, depends, recur, recurDisplay, parent, recurrenceTarget
        case urgency, todayGroup, edit, extra
    }
}

struct Annotation: Decodable, Sendable, Hashable {
    var description: String
    var entry: Date?
}

/// The eight editable values as text, in the TUI editor's formats.
struct EditFields: Codable, Sendable, Hashable {
    var description = ""
    var project = ""
    var priority = ""
    var due = ""
    var scheduled = ""
    var tags = ""
    var estimate = ""
    var recurrence = ""
}

struct TextSpan: Codable, Sendable, Hashable {
    var start: Int
    var end: Int
}

struct Interpretation: Decodable, Sendable, Equatable {
    var token: Int64?
    var source: String
    var valid: Bool
    var requiresReview: Bool
    var description: String
    var candidates: [Candidate]
    var diagnostics: [Diagnostic]
    var reviewValues: [String]
}

struct Candidate: Decodable, Sendable, Hashable {
    var field: String
    var text: String
    var value: String
    var start: Int
    var end: Int
    var provenance: String
    var accepted: Bool
    var conflict: Bool
    var blocking: Bool
    var diagnostic: String?

    var isInferred: Bool { provenance == "inferred" }
    /// Matches the TUI: inferred phrases show when accepted or blocking;
    /// explicit trigger tokens always show.
    var isHighlighted: Bool { !isInferred || accepted || blocking }
}

struct Diagnostic: Decodable, Sendable, Hashable {
    var message: String
    var blocking: Bool
}

struct SuggestResult: Decodable, Sendable, Equatable {
    var active: Bool
    var kind: String?
    var start: Int
    var end: Int
    var suggestions: [Suggestion]

    static let none = SuggestResult(active: false, kind: nil, start: 0, end: 0, suggestions: [])
}

struct Suggestion: Decodable, Sendable, Hashable {
    var value: String
    var text: String
    var label: String?
    var input: String
    var cursor: Int
}

struct CaptureResult: Decodable, Sendable, Equatable {
    var added: Bool
    var review: Interpretation?
}

/// A correction review ready to submit, in quickadd.Review* field order.
struct ReviewSubmission: Encodable, Sendable, Equatable {
    var token: Int64
    var values: [String]
    var touched: [Bool]
    var decisions: [Bool]
}

enum TaskAction: String, Sendable {
    case complete, start, stop, delete, stopRecurrence
}
