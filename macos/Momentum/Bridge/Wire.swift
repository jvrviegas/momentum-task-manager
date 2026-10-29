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
    /// Why a newer config.toml was not applied; the app keeps the last good
    /// settings meanwhile.
    var configError: String?
    /// A changed [backend] url waits for the app to restart.
    var restartRequired: Bool?
}

/// What the Settings tabs show: the values in use, the version of config.toml
/// they came from, and each calendar file's state.
struct SettingsSnapshot: Decodable, Sendable, Equatable {
    var configPath: String
    /// Sent back with a save, which is refused when the file has changed since.
    var revision: String
    var configError: String?
    var restartRequired: Bool?
    var planning: PlanningSettings
    var calendar: CalendarSettings
}

/// Lengths are as a person types them: 8h, 7h30m.
struct PlanningSettings: Decodable, Sendable, Equatable {
    var enabled: Bool
    var dailyCapacity: String
    var buffer: String
    /// Only the weekdays with a capacity of their own, by lowercase name.
    var weekdayCapacity: [String: String]
}

struct CalendarSettings: Decodable, Sendable, Equatable {
    /// Which source config.toml names: "files" or "google". Enabled and files
    /// are about the files only.
    var source = "files"
    var enabled: Bool
    var files: [CalendarFile]
    /// Times of day as HH:MM.
    var workingStart = "09:00"
    var workingEnd = "17:00"
    var includeAllDay = false
    var includeTransparent = false
    /// A length as a person types it; 0m means a file is never stale.
    var staleAfter = "24h"
}

/// One calendar file as stored, with what loading it on its own found:
/// available, stale, missing or error.
struct CalendarFile: Decodable, Sendable, Hashable, Identifiable {
    var path: String
    var status: String
    var detail: String?
    var modified: Date?

    var id: String { path }
}

/// The [planning] keys a save changes; a nil field stays as it is. An empty
/// weekday length gives that day the daily capacity again.
struct PlanningChange: Encodable, Sendable, Equatable {
    var revision: String
    var enabled: Bool?
    var dailyCapacity: String?
    var buffer: String?
    var weekdayCapacity: [String: String]?
}

/// What the server says of this account's Google Calendar: whether it offers
/// one at all, whether it is connected and as whom, and whether Google has
/// stopped accepting the connection.
struct GoogleCalendarStatus: Decodable, Sendable, Equatable {
    var offered: Bool
    var connected: Bool
    var email: String?
    var needsReconnect: Bool
}

/// The one-time link that connects Google Calendar to this account in a
/// browser. It works once, until expiresAt.
struct CalendarConnectLink: Decodable, Sendable, Equatable {
    var url: String
    var expiresAt: Date
}

/// The [calendar] keys a save changes; paths is the whole list of files.
struct CalendarChange: Encodable, Sendable, Equatable {
    var revision: String
    var source: String?
    var enabled: Bool?
    var paths: [String]?
    var workingStart: String?
    var workingEnd: String?
    var includeAllDay: Bool?
    var includeTransparent: Bool?
    var staleAfter: String?
}

/// One check of Diagnostics. status is "ok", "fail" or "skip", and detail has
/// had anything that looks like a secret removed.
struct DoctorCheck: Decodable, Sendable, Equatable, Identifiable {
    var name: String
    var status: String
    var detail: String

    var id: String { name }
}

struct DoctorReport: Decodable, Sendable, Equatable {
    var configPath: String
    var bridgeVersion: String
    var checks: [DoctorCheck]
    /// True when no check failed.
    var ready: Bool
}

/// One row of Settings ▸ Projects: an entry of the catalog, or a value found
/// only on tasks. tasks counts the pending tasks with exactly this value.
struct ProjectRow: Decodable, Sendable, Equatable, Identifiable {
    var value: String
    var name: String
    /// "catalog" or "tasks".
    var tag: String
    var tasks: Int

    var id: String { value }
    var inCatalog: Bool { tag == "catalog" }
    /// How deep the value sits: work.client is one below work.
    var depth: Int { value.split(separator: ".").count - 1 }
}

struct ProjectsSnapshot: Decodable, Sendable, Equatable {
    /// Sent back with a save, which is refused when config.toml has changed since.
    var revision: String
    var entries: [ProjectRow]
    /// Why the tasks could not be read, which leaves out the counts and the
    /// values found only on tasks.
    var tasksError: String?
}

/// One task's project before and after a rename. It holds no title.
struct ProjectTaskMove: Codable, Sendable, Equatable {
    var uuid: String
    var oldValue: String
    var newValue: String
}

/// One change to the catalog: add an entry, edit the one at source, or remove
/// it. The value of an added or edited entry is parent and segment joined by a
/// dot. With migrate, the tasks in tasks, the ones the preview showed, move
/// to the new value; without it only the catalog changes.
struct ProjectEdit: Encodable, Sendable, Equatable {
    var revision: String
    var action: String
    var source = ""
    var name = ""
    var parent = ""
    var segment = ""
    var includeSubprojects = false
    var removeChildren = false
    var migrate = false
    var tasks: [ProjectTaskMove] = []
}

/// What a rename would do: which pending tasks move with Include subprojects
/// off and on, and how many catalog entries below it move with it.
struct ProjectPreview: Decodable, Sendable, Equatable {
    var value: String
    var valueChanged: Bool
    var catalogDescendants: Int
    var exactTasks: [ProjectTaskMove]
    var subprojectTasks: [ProjectTaskMove]
    /// Tasks already use the new value, which the catalog lacks.
    var destinationTaskOnly: Bool
}

struct ProjectSaveResult: Decodable, Sendable, Equatable {
    var revision: String
    var changed: Int
    /// Why the server moved no task after the catalog was saved.
    var taskError: String?
    var stale: Bool?
}

/// What stands in for the calendar files when this Mac's source is macOS
/// Calendar, which only the app can read: today's events, or why there are
/// none ("no access"). Event titles go only with a plan.
struct CalendarInput: Encodable, Sendable, Equatable {
    var error: String?
    var events: [CalendarEvent]
}

/// One event as the Mac's Calendar has it. A free event is one the calendar
/// shows as free rather than busy.
struct CalendarEvent: Encodable, Sendable, Equatable {
    var start: Date
    var end: Date
    var allDay: Bool
    var free: Bool
    var title: String?
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
    /// Zero while the calendar is off or unavailable.
    var calendarMinutes = 0
    var calendarAvailable = false
    var calendarError: String?
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
    var calendar: PlanCalendar?
}

/// Today's busy time from the [calendar] files. error says why the calendar
/// is unavailable, which makes its time count as zero.
struct PlanCalendar: Decodable, Sendable, Equatable {
    var enabled: Bool
    var available: Bool
    var stale: Bool
    var error: String?
    var busyMinutes: Int
    var eventCount: Int
    /// Working hours as "HH:MM".
    var workingStart: String
    var workingEnd: String
    var blocks: [BusyBlock]
}

/// A merged stretch of busy time inside working hours, with the titles of
/// its events.
struct BusyBlock: Decodable, Sendable, Hashable {
    var start: Date
    var end: Date
    var titles: [String]
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
