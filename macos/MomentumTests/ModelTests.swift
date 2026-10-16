import Foundation
import Testing
@testable import Momentum

struct TaskSearchTests {
    @Test func projectTermIsExactAndCaseInsensitive() {
        let task = TaskItem.fixture("Send invoice", project: "work.Client")
        #expect(TaskSearch.matches(task, query: "project:WORK.client"))
        #expect(!TaskSearch.matches(task, query: "project:work"))
        #expect(TaskSearch.matches(TaskItem.fixture("Loose"), query: "project:none"))
        #expect(!TaskSearch.matches(task, query: "project:none"))
    }

    @Test func fuzzyTextCoversDescriptionProjectAndTags() {
        let task = TaskItem.fixture("Renew the TLS certificate", project: "ops.infra", tags: ["security"])
        #expect(TaskSearch.matches(task, query: "renew"))
        #expect(TaskSearch.matches(task, query: "infra"))
        #expect(TaskSearch.matches(task, query: "secu"))
        #expect(TaskSearch.matches(task, query: "project:ops.infra tls"))
        #expect(!TaskSearch.matches(task, query: "zebra"))
    }

    @Test func scoresMatchTheTUI() {
        #expect(TaskSearch.score("ren", in: "renew") == 10_000 - 5)
        #expect(TaskSearch.score("rn", in: "renew") == (10 + 20) + 10 - 3)
        // A scattered match in a long value scores below zero and is dropped.
        #expect(TaskSearch.score("ae", in: "a very long description with an e at the end")! < 0)
        #expect(!TaskSearch.matches(.fixture("a very long description with an e at the end"), query: "ae"))
    }
}

struct TextOffsetTests {
    @Test func convertsBetweenScalarsAndUTF16() {
        let text = "Fix 🐛 #work"
        #expect(text.utf16Offset(ofScalar: 5) == 6)
        #expect(text.scalarOffset(ofUTF16: 6) == 5)
        #expect(text.nsRange(ofScalars: 6, 11) == NSRange(location: 7, length: 5))
    }

    @Test func shiftsKeptPhrasesAcrossEdits() {
        let kept = [TextSpan(start: 5, end: 13)]
        #expect(shiftLiterals(from: "Call tomorrow", to: "XX Call tomorrow", kept) == [TextSpan(start: 8, end: 16)])
        #expect(shiftLiterals(from: "Call tomorrow", to: "Call tomorrow!", kept) == kept)
        #expect(shiftLiterals(from: "Call tomorrow", to: "Call tomorow", kept).isEmpty)
    }
}

struct WireTests {
    @Test func decodesBridgeTasks() throws {
        let json = """
        {"tasks":[{"uuid":"u1","id":3,"description":"Send invoice","status":"pending","project":"work",
        "due":"2026-10-14T16:00:00Z","tags":[],"annotations":[{"description":"note","entry":"2026-10-13T09:00:00Z"}],
        "depends":[],"urgency":9.5,"todayGroup":"Due Today","edit":{"description":"Send invoice","project":"work",
        "priority":"","due":"2026-10-14 17:00","scheduled":"","tags":"","estimate":"","recurrence":""},"extra":{"client":"\\"acme\\""}}],
        "today":[{"title":"Due Today","uuids":["u1"]}],"inbox":[],"completed":[],"projects":[{"value":"work","label":"Work"}]}
        """
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let snapshot = try decoder.decode(Snapshot.self, from: Data(json.utf8))
        let task = try #require(snapshot.tasks.first)
        #expect(task.number == 3 && task.id == "u1")
        #expect(task.due == Date(timeIntervalSince1970: 1_791_993_600))
        #expect(task.edit.due == "2026-10-14 17:00" && task.annotations.first?.entry != nil)
        #expect(task.extra?["client"] == "\"acme\"")
    }

    @Test func decodesBackendStatusSignInAndErrors() throws {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let status = try decoder.decode(BridgeStatus.self, from: Data("""
        {"taskwarriorVersion":"","syncConfigured":false,"estimateUDA":"configured","configPath":"/tmp/config.toml",
        "sync":{"enabled":true,"startup":true,"shutdown":true,"intervalSeconds":300,"mutationDelaySeconds":15},
        "refreshIntervalSeconds":60,"backendUrl":"https://momentum.example"}
        """.utf8))
        #expect(status.backendUrl == "https://momentum.example")
        let start = try decoder.decode(DeviceSignIn.self, from: Data("""
        {"deviceCode":"dc","userCode":"BCDFGHJK","verificationUrl":"https://m.example/device",
        "verificationUrlComplete":"https://m.example/device?code=BCDFGHJK","expiresAt":"2026-10-16T11:10:00Z","intervalSeconds":5}
        """.utf8))
        #expect(start.userCode == "BCDFGHJK" && start.expiresAt == Date(timeIntervalSince1970: 1_792_149_000) && start.intervalSeconds == 5)
        let poll = try decoder.decode(SignInPoll.self, from: Data("""
        {"state":"signedIn","token":"mtk_new","email":"ana@example.com","expiresAt":"2027-01-14T12:00:00Z"}
        """.utf8))
        #expect(poll.token == "mtk_new" && poll.expiresAt == Date(timeIntervalSince1970: 1_799_928_000))
        #expect(try decoder.decode(SignInPoll.self, from: Data(#"{"state":"pending"}"#.utf8)) == SignInPoll(state: "pending"))
        let error = try decoder.decode(BridgeError.self, from: Data(#"{"message":"sign in again","code":"unauthorized"}"#.utf8))
        #expect(error == BridgeError(message: "sign in again", code: "unauthorized"))
        let tw = try decoder.decode(BridgeStatus.self, from: Data("""
        {"taskwarriorVersion":"3.5.0","syncConfigured":true,"estimateUDA":"configured","configPath":"/tmp/config.toml",
        "sync":{"enabled":true,"startup":true,"shutdown":true,"intervalSeconds":300,"mutationDelaySeconds":15},"refreshIntervalSeconds":60}
        """.utf8))
        #expect(tw.backendUrl == nil)
    }
}

extension TaskItem {
    static func fixture(
        _ description: String, uuid: String? = nil, status: String = "pending", project: String? = nil,
        tags: [String] = [], due: Date? = nil, recur: String? = nil, parent: String? = nil
    ) -> TaskItem {
        TaskItem(
            uuid: uuid ?? description, number: 1, description: description, status: status, project: project,
            due: due, tags: tags, annotations: [], depends: [], recur: recur, parent: parent,
            recurrenceTarget: recur == nil ? nil : (parent ?? uuid ?? description), urgency: 0,
            edit: EditFields(description: description, project: project ?? "", tags: tags.joined(separator: " ")))
    }
}

struct FormatTests {
    @Test func reviewDatesIgnoreEmptyValues() {
        #expect(Format.reviewDate("") == nil)
        #expect(Format.reviewDate("tomorrow") == nil)
        #expect(Format.reviewDate("20261015T150000") != nil)
    }
}
