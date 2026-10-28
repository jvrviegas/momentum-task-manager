import Foundation

struct BridgeError: Error, LocalizedError, Decodable, Sendable, Equatable {
    var message: String
    var field: String?
    /// "unreachable" when the server gave no answer and "unauthorized" when
    /// it refused the sign-in.
    var code: String?

    var errorDescription: String? { message }
}

/// Everything the app asks of the Momentum server. ProcessBridge implements
/// it over the bundled Go `momentum-bridge`; tests use a fake.
protocol TaskBridge: Sendable {
    func status() async throws -> BridgeStatus
    func load() async throws -> Snapshot
    func interpret(_ input: String, literals: [TextSpan]) async throws -> Interpretation
    func suggest(_ input: String, cursor: Int) async throws -> SuggestResult
    func capture(_ input: String, literals: [TextSpan]) async throws -> CaptureResult
    func captureReview(_ submission: ReviewSubmission) async throws
    func captureExplicit(_ input: String) async throws
    func modify(uuid: String, before: EditFields, after: EditFields) async throws -> Bool
    func perform(_ action: TaskAction, uuid: String) async throws
    func undo() async throws
    // Plan Today.
    func plan() async throws -> DailyPlan
    func applyPlan(date: String, uuids: [String]) async throws -> PlanSaveResult
    // Settings.
    func settings() async throws -> SettingsSnapshot
    func savePlanning(_ change: PlanningChange) async throws -> SettingsSnapshot
    func saveCalendar(_ change: CalendarChange) async throws -> SettingsSnapshot
    // Sign-in.
    func setToken(_ token: String) async throws
    func startSignIn() async throws -> DeviceSignIn
    func pollSignIn(deviceCode: String) async throws -> SignInPoll
    func revoke(token: String) async throws
}

/// Runs `momentum-bridge` as a child process and exchanges one JSON object per
/// line. The process starts on first use and restarts after it exits.
actor ProcessBridge: TaskBridge {
    private let executable: URL
    private var process: Process?
    private var input: FileHandle?
    private var nextID: Int64 = 0
    private var waiting: [Int64: CheckedContinuation<Data, any Error>] = [:]
    /// The token the bridge last accepted from setToken. A restarted bridge
    /// has none, so it is sent again before anything else. It goes only
    /// through the pipe, never in arguments, the environment, or a file.
    private var token = ""

    private static let encoder = JSONEncoder()
    private static let decoder: JSONDecoder = {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }()

    init(executable: URL) {
        self.executable = executable
    }

    static func bundled() -> ProcessBridge {
        let url = Bundle.main.url(forAuxiliaryExecutable: "momentum-bridge")
            ?? Bundle.main.bundleURL.appending(path: "Contents/MacOS/momentum-bridge")
        return ProcessBridge(executable: url)
    }

    func status() async throws -> BridgeStatus { try await call("status") }
    func load() async throws -> Snapshot { try await call("load") }

    func interpret(_ input: String, literals: [TextSpan]) async throws -> Interpretation {
        try await call("interpret", CaptureParams(input: input, literals: literals))
    }

    func suggest(_ input: String, cursor: Int) async throws -> SuggestResult {
        try await call("suggest", SuggestParams(input: input, cursor: cursor))
    }

    func capture(_ input: String, literals: [TextSpan]) async throws -> CaptureResult {
        try await call("capture", CaptureParams(input: input, literals: literals))
    }

    func captureReview(_ submission: ReviewSubmission) async throws {
        let _: Empty = try await call("captureReview", submission)
    }

    func captureExplicit(_ input: String) async throws {
        let _: Empty = try await call("captureExplicit", CaptureParams(input: input, literals: []))
    }

    func modify(uuid: String, before: EditFields, after: EditFields) async throws -> Bool {
        let result: Changed = try await call("modify", ModifyParams(uuid: uuid, before: before, after: after))
        return result.changed
    }

    func perform(_ action: TaskAction, uuid: String) async throws {
        let _: Empty = try await call(action.rawValue, UUIDParams(uuid: uuid))
    }

    func undo() async throws {
        let _: Empty = try await call("undo")
    }

    func plan() async throws -> DailyPlan { try await call("plan") }

    func settings() async throws -> SettingsSnapshot { try await call("settings") }
    func savePlanning(_ change: PlanningChange) async throws -> SettingsSnapshot { try await call("savePlanning", change) }
    func saveCalendar(_ change: CalendarChange) async throws -> SettingsSnapshot { try await call("saveCalendar", change) }

    func applyPlan(date: String, uuids: [String]) async throws -> PlanSaveResult {
        try await call("applyPlan", ApplyPlanParams(date: date, uuids: uuids))
    }

    func setToken(_ token: String) async throws {
        let _: Empty = try await call("setToken", TokenParams(token: token))
        self.token = token
    }

    func startSignIn() async throws -> DeviceSignIn { try await call("signInStart") }

    func pollSignIn(deviceCode: String) async throws -> SignInPoll {
        try await call("signInPoll", PollParams(deviceCode: deviceCode))
    }

    func revoke(token: String) async throws {
        let _: Empty = try await call("revoke", TokenParams(token: token))
    }

    // MARK: Transport

    private func call<Result: Decodable>(_ op: String, _ params: some Encodable & Sendable = Empty()) async throws -> Result {
        let line = try await send(op, params)
        return try Self.decoder.decode(Reply<Result>.self, from: line).result
    }

    private func send(_ op: String, _ params: some Encodable & Sendable) async throws -> Data {
        let input = try launch()
        nextID += 1
        let id = nextID
        let line = try Self.encoder.encode(Request(id: id, op: op, params: params)) + Data("\n".utf8)
        return try await withCheckedThrowingContinuation { continuation in
            waiting[id] = continuation
            do {
                try input.write(contentsOf: line)
            } catch {
                waiting.removeValue(forKey: id)
                continuation.resume(throwing: error)
            }
        }
    }

    private func launch() throws -> FileHandle {
        if let input, process?.isRunning == true {
            return input
        }
        // A write to a bridge that just exited must fail, not kill the app.
        signal(SIGPIPE, SIG_IGN)

        let process = Process()
        process.executableURL = executable
        let stdin = Pipe(), stdout = Pipe(), stderr = Pipe()
        process.standardInput = stdin
        process.standardOutput = stdout
        process.standardError = stderr
        process.terminationHandler = { [weak self] finished in
            let message = String(decoding: stderr.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
            Task { await self?.terminated(finished, stderr: message) }
        }
        do {
            try process.run()
        } catch {
            throw BridgeError(message: "Could not start momentum-bridge: \(error.localizedDescription)")
        }
        self.process = process
        self.input = stdin.fileHandleForWriting
        if !token.isEmpty {
            // Nobody waits for this answer. The bridge runs requests in order,
            // so the ones that follow are signed in.
            nextID += 1
            let line = try Self.encoder.encode(Request(id: nextID, op: "setToken", params: TokenParams(token: token)))
            try? stdin.fileHandleForWriting.write(contentsOf: line + Data("\n".utf8))
        }

        let output = stdout.fileHandleForReading
        Task.detached { [weak self] in
            do {
                for try await line in output.bytes.lines {
                    await self?.deliver(Data(line.utf8))
                }
            } catch {}
        }
        return stdin.fileHandleForWriting
    }

    private func deliver(_ line: Data) {
        guard let header = try? Self.decoder.decode(Header.self, from: line),
              let continuation = waiting.removeValue(forKey: header.id) else { return }
        if let error = header.error {
            continuation.resume(throwing: error)
        } else {
            continuation.resume(returning: line)
        }
    }

    private func terminated(_ finished: Process, stderr: String) {
        guard finished === process else { return }
        process = nil
        input = nil
        let reason = stderr.trimmingCharacters(in: .whitespacesAndNewlines)
        let error = BridgeError(message: reason.isEmpty ? "momentum-bridge stopped (exit \(finished.terminationStatus))." : reason)
        for continuation in waiting.values {
            continuation.resume(throwing: error)
        }
        waiting.removeAll()
    }
}

private struct Request<Params: Encodable>: Encodable {
    var id: Int64
    var op: String
    var params: Params
}

private struct Header: Decodable {
    var id: Int64
    var error: BridgeError?
}

private struct Reply<Result: Decodable>: Decodable {
    var result: Result
}

private struct Empty: Codable, Sendable {}
private struct Changed: Decodable, Sendable { var changed: Bool }
private struct UUIDParams: Encodable, Sendable { var uuid: String }
private struct TokenParams: Encodable, Sendable { var token: String }
private struct PollParams: Encodable, Sendable { var deviceCode: String }
private struct SuggestParams: Encodable, Sendable { var input: String; var cursor: Int }
private struct CaptureParams: Encodable, Sendable { var input: String; var literals: [TextSpan] }
private struct ApplyPlanParams: Encodable, Sendable { var date: String; var uuids: [String] }

private struct ModifyParams: Encodable, Sendable {
    var uuid: String
    var before: EditFields
    var after: EditFields
}

extension BridgeError {
    init(message: String) {
        self.init(message: message, field: nil)
    }
}
