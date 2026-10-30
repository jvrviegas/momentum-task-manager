import Foundation
import Testing
@testable import Momentum

/// The bridge's answers as Go writes them: `TestWire…` in tui/internal/bridge
/// keeps one file per answer in testdata/wire. Each is decoded here with the
/// app's own decoder, and every key in it must reach a property, so a key
/// renamed on one side fails here, not in the running app. After a change to
/// the wire on purpose, run `go test ./internal/bridge -run TestWire -update`
/// in tui/ and add the new file to `fixtures`.
struct WireContractTests {
    struct Fixture: Sendable, CustomTestStringConvertible {
        let name: String
        let decode: @Sendable (Data) throws -> Any
        var testDescription: String { name }

        init<T: Decodable>(_ name: String, _ type: T.Type) {
            self.name = name
            decode = { try ProcessBridge.decoder.decode(T.self, from: $0) }
        }
    }

    static let fixtures = [
        Fixture("status", BridgeStatus.self),
        Fixture("status-config-error", BridgeStatus.self),
        Fixture("status-restart-required", BridgeStatus.self),
        Fixture("load", Snapshot.self),
        Fixture("load-trouble", Snapshot.self),
        Fixture("interpret", Interpretation.self),
        Fixture("suggest", SuggestResult.self),
        Fixture("capture-added", CaptureResult.self),
        Fixture("capture-review", CaptureResult.self),
        Fixture("modify", Changed.self),
        Fixture("duplicate", Duplicated.self),
        Fixture("plan", DailyPlan.self),
        Fixture("apply-plan", PlanSaveResult.self),
        Fixture("apply-plan-stopped", PlanSaveResult.self),
        Fixture("settings", SettingsSnapshot.self),
        Fixture("calendar-status", GoogleCalendarStatus.self),
        Fixture("calendar-status-not-offered", GoogleCalendarStatus.self),
        Fixture("calendar-status-needs-reconnect", GoogleCalendarStatus.self),
        Fixture("calendar-connect", CalendarConnectLink.self),
        Fixture("calendar-disconnect", GoogleCalendarDisconnect.self),
        Fixture("doctor", DoctorReport.self),
        Fixture("projects", ProjectsSnapshot.self),
        Fixture("projects-tasks-error", ProjectsSnapshot.self),
        Fixture("project-preview", ProjectPreview.self),
        Fixture("project-save", ProjectSaveResult.self),
        Fixture("project-save-stale", ProjectSaveResult.self),
        Fixture("sign-in-start", DeviceSignIn.self),
        Fixture("sign-in-pending", SignInPoll.self),
        Fixture("sign-in-done", SignInPoll.self),
        Fixture("error-field", BridgeError.self),
        Fixture("error-unauthorized", BridgeError.self),
    ]

    /// Keys that Go writes under another name than the property that takes
    /// them in (`CodingKeys`).
    private static let renamed = ["id": "number"]

    private static let directory = URL(filePath: #filePath)
        .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        .appending(path: "tui/internal/bridge/testdata/wire")

    @Test func everyFileHasATypeHereAndEveryTypeAFile() throws {
        let files = try FileManager.default.contentsOfDirectory(atPath: Self.directory.path())
            .filter { $0.hasSuffix(".json") }.map { String($0.dropLast(".json".count)) }
        #expect(files.sorted() == Self.fixtures.map(\.name).sorted())
    }

    @Test(arguments: WireContractTests.fixtures)
    func theAppReadsEveryKeyTheBridgeSends(_ fixture: Fixture) throws {
        let data = try Data(contentsOf: Self.directory.appending(path: "\(fixture.name).json"))
        let value = try fixture.decode(data)
        let unread = Self.unread(try JSONSerialization.jsonObject(with: data), in: value)
        #expect(unread.isEmpty, "keys the app did not read: \(unread.joined(separator: ", "))")
    }

    /// The paths of the keys in json that value did not take in: the type has
    /// no property of that name, or an optional one stayed nil, as it does
    /// when Go renames the key. The keys of a dictionary are data and are
    /// not checked.
    static func unread(_ json: Any, in value: Any, path: String = "") -> [String] {
        guard let value = unwrapped(value) else { return [path] }
        let mirror = Mirror(reflecting: value)
        switch json {
        case let object as [String: Any]:
            if mirror.displayStyle == .dictionary { return [] }
            return object.keys.sorted().flatMap { key -> [String] in
                let label = renamed[key] ?? key
                guard let child = mirror.children.first(where: { $0.label == label }) else { return ["\(path)/\(key)"] }
                return unread(object[key]!, in: child.value, path: "\(path)/\(key)")
            }
        case let array as [Any]:
            let items = mirror.children.map(\.value)
            guard items.count == array.count else { return ["\(path) (\(array.count) items sent, \(items.count) read)"] }
            return zip(array, items).enumerated().flatMap { unread($1.0, in: $1.1, path: "\(path)[\($0)]") }
        default:
            return []
        }
    }

    /// The value inside an optional, or nil when it is empty.
    private static func unwrapped(_ value: Any) -> Any? {
        let mirror = Mirror(reflecting: value)
        guard mirror.displayStyle == .optional else { return value }
        return mirror.children.first?.value
    }
}
