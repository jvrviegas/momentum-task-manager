#if DEBUG
import Foundation
import Testing
@testable import Momentum

/// The snapshot run must never reach a real server, Keychain entry or
/// config.toml, and must stop when the app doesn't build.
@MainActor
struct SnapshotTests {
    @Test func everyPageHasOneName() {
        let names = SnapshotScene.all(now: .now).map(\.name)
        #expect(Set(names).count == names.count)
        #expect(names.allSatisfy { !$0.isEmpty && $0 == $0.lowercased() && !$0.contains(" ") })
    }

    @Test func everyPageRunsOnTheFakeBridgeAndAnInMemorySignInStore() {
        for scene in SnapshotScene.all(now: .now) {
            let store = SnapshotHost.makeStore(scene)
            #expect(store.bridge is SnapshotBridge, "\(scene.name) uses the fake bridge")
            #expect(store.signIns is SnapshotSignInStore, "\(scene.name) keeps its sign-in in memory")
        }
    }

    @Test func theSeedFillsEveryViewAndSignsIn() async throws {
        let store = SnapshotHost.makeStore(SnapshotScene(name: "seed"))
        await store.start()
        #expect(store.account?.email == "joao@example.com")
        #expect(store.view == .today)
        #expect(store.overdueCount > 0)
        #expect(store.count(.today) > 0 && store.count(.inbox) > 0)
        let snapshot = try #require(store.snapshot)
        #expect(!snapshot.completed.isEmpty)
        #expect(snapshot.tasks.contains { $0.start != nil }, "one task is active")
        #expect(snapshot.tasks.contains { $0.recur != nil }, "one task repeats")
        #expect(snapshot.tasks.contains { !$0.annotations.isEmpty }, "one task has notes")
    }

    @Test func completingATaskMovesItToCompleted() async throws {
        let store = SnapshotHost.makeStore(SnapshotScene(name: "undo"))
        await store.start()
        let task = try #require(store.tasksByUUID.values.first { $0.description.hasPrefix("Review pull request") })
        await store.complete(task)
        #expect(store.tasksByUUID[task.uuid]?.status == "completed")
        store.view = .completed
        #expect(store.visibleTasks.first?.uuid == task.uuid)
    }

    /// A stand-in `xcodebuild` that fails makes the script fail with its
    /// status, and nothing after the build runs.
    @Test func aFailedBuildStopsTheScript() throws {
        let fileManager = FileManager.default
        let root = fileManager.temporaryDirectory.appending(path: "snap-\(UUID().uuidString)")
        let stubs = root.appending(path: "bin")
        try fileManager.createDirectory(at: stubs, withIntermediateDirectories: true)
        defer { try? fileManager.removeItem(at: root) }
        for (name, status) in [("xcodegen", 0), ("xcodebuild", 65)] {
            let stub = stubs.appending(path: name)
            try "#!/bin/sh\nexit \(status)\n".write(to: stub, atomically: true, encoding: .utf8)
            try fileManager.setAttributes([.posixPermissions: 0o755], ofItemAtPath: stub.path)
        }
        let script = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().appending(path: "tools/snap.sh")
        let out = root.appending(path: "shots")
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/sh")
        process.arguments = [script.path]
        process.environment = [
            "PATH": "\(stubs.path):/usr/bin:/bin", "HOME": root.path, "SNAP_OUT": out.path,
            "SNAP_DERIVED": root.appending(path: "derived").path,
        ]
        try process.run()
        process.waitUntilExit()
        #expect(process.terminationStatus == 65)
        #expect(!fileManager.fileExists(atPath: out.path), "no run starts after a failed build")
    }
}
#endif
