import Foundation

struct BridgeError: Error, LocalizedError, Decodable, Sendable, Equatable {
    var message: String
    var field: String?

    var errorDescription: String? { message }
}

/// Everything the app asks of Taskwarrior. ProcessBridge implements it over
/// the bundled Go `momentum-bridge`; tests use a fake.
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
    func sync() async throws
}

/// Runs `momentum-bridge` as a child process and exchanges one JSON object per
/// line. The process starts on first use and restarts after it exits.
actor ProcessBridge: TaskBridge {
    private let executable: URL
    private var process: Process?
    private var input: FileHandle?
    private var nextID: Int64 = 0
    private var waiting: [Int64: CheckedContinuation<Data, any Error>] = [:]

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

    func sync() async throws {
        let _: Changed = try await call("sync")
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
        var environment = ProcessInfo.processInfo.environment
        // Apps opened from Finder get a minimal PATH; Taskwarrior is usually
        // installed with Homebrew.
        environment["PATH"] = ["/opt/homebrew/bin", "/usr/local/bin", environment["PATH"] ?? "/usr/bin:/bin"]
            .joined(separator: ":")
        process.environment = environment
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
private struct SuggestParams: Encodable, Sendable { var input: String; var cursor: Int }
private struct CaptureParams: Encodable, Sendable { var input: String; var literals: [TextSpan] }

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
