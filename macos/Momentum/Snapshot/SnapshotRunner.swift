#if DEBUG
import AppKit
import SwiftUI

/// What the app shows while it is taking snapshots: one store at a time, each
/// built on a fake bridge. A new `generation` rebuilds the window's content,
/// which closes any sheet the last page left open.
@MainActor
@Observable
final class SnapshotHost {
    private(set) var store = SnapshotHost.makeStore(SnapshotScene(name: ""))
    private(set) var generation = 0
    private let directory: URL
    private let only: Set<String>?

    /// MOMENTUM_SNAPSHOT names the folder for the images; MOMENTUM_SNAPSHOT_PAGES
    /// limits the run to some pages. Without the first, the app runs normally.
    static func fromEnvironment() -> SnapshotHost? {
        let environment = ProcessInfo.processInfo.environment
        guard let path = environment["MOMENTUM_SNAPSHOT"], !path.isEmpty else { return nil }
        let pages = environment["MOMENTUM_SNAPSHOT_PAGES"].map { Set($0.split(separator: ",").map(String.init)) }
        return SnapshotHost(directory: URL(fileURLWithPath: path), only: pages)
    }

    private init(directory: URL, only: Set<String>?) {
        self.directory = directory
        self.only = only
    }

    static func makeStore(_ scene: SnapshotScene) -> AppStore {
        let url = scene.config.backendURL ?? ""
        let saved = scene.signedIn
            ? [SavedSignIn(server: url, token: "snapshot-token", email: "joao@example.com", expiresAt: .now.addingTimeInterval(86400))] : []
        return AppStore(bridge: SnapshotBridge(scene.config), signIns: SnapshotSignInStore(saved))
    }

    /// Takes every page in light and dark, then quits.
    func run() async {
        let watchdog = Task {
            try await Task.sleep(for: .seconds(420))
            fputs("SNAPSHOT timed out\n", stderr)
            exit(2)
        }
        defer { watchdog.cancel() }
        setvbuf(stdout, nil, _IOLBF, 0)
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        NSApp.activate(ignoringOtherApps: true)
        // SwiftUI sizes the first window after it appears, so wait before
        // asking for the size.
        try? await Task.sleep(for: .seconds(2))
        var failures = 0
        for (look, appearance) in [("light", NSAppearance.Name.aqua), ("dark", .darkAqua)] {
            NSApp.appearance = NSAppearance(named: appearance)
            for scene in SnapshotScene.all(now: .now) where only?.contains(scene.name) ?? true {
                do {
                    try await take(scene, look: look)
                } catch {
                    failures += 1
                    print("SNAPSHOT FAIL \(scene.name)-\(look): \(error)")
                }
            }
        }
        print("SNAPSHOT done \(directory.path)\(failures == 0 ? "" : " (\(failures) failed)")")
        exit(failures == 0 ? 0 : 1)
    }

    private func take(_ scene: SnapshotScene, look: String) async throws {
        store = Self.makeStore(scene)
        generation += 1
        let session = SnapshotSession(store: store)
        session.window?.setContentSize(SnapshotSession.windowSize)
        await session.settle()
        if scene.waitsForStart {
            await store.start()
        } else {
            Task { await store.start() }
        }
        await session.settle()
        await scene.run(session)
        await session.settle()
        let file = directory.appending(path: "\(scene.name)-\(look).png")
        try await session.capture(to: file)
        let frame = session.window?.frame ?? .zero
        print("SNAPSHOT \(file.path) \(Int(frame.width))x\(Int(frame.height))")
    }
}

/// The window's content while snapshots run. Links do nothing, so the
/// sign-in page does not open a browser.
struct SnapshotRoot: View {
    let host: SnapshotHost

    var body: some View {
        ContentView()
            .environment(host.store)
            .environment(\.openURL, OpenURLAction { _ in .handled })
            .id(host.generation)
    }
}

/// The pieces a page's steps use: the store, the window, and the text field a
/// capture sheet types into.
@MainActor
struct SnapshotSession {
    let store: AppStore

    struct Failure: Error, CustomStringConvertible {
        var description: String
    }

    /// The window's default size, so every image has the same frame.
    static let windowSize = NSSize(width: 1120, height: 720)

    func settle(_ milliseconds: Int = 700) async {
        try? await Task.sleep(for: .milliseconds(milliseconds))
    }

    var window: NSWindow? {
        NSApp.windows.first { $0.isVisible && !$0.isSheet && $0.canBecomeMain }
    }

    func task(_ prefix: String) -> TaskItem {
        store.tasksByUUID.values.first { $0.description.hasPrefix(prefix) }!
    }

    /// Types into the capture sheet's text field the way a person does, so
    /// the sheet interprets and suggests as it would.
    func type(_ text: String) async {
        guard let field = textView(in: window?.attachedSheet?.contentView) else { return }
        window?.attachedSheet?.makeFirstResponder(field)
        field.insertText(text, replacementRange: NSRange(location: NSNotFound, length: 0))
        await settle()
    }

    func pressReturn() async {
        textView(in: window?.attachedSheet?.contentView)?.doCommand(by: #selector(NSResponder.insertNewline(_:)))
        await settle()
    }

    private func textView(in view: NSView?) -> NSTextView? {
        guard let view else { return nil }
        if let text = view as? NSTextView { return text }
        return view.subviews.lazy.compactMap { textView(in: $0) }.first
    }

    /// Saves the app's own window, with its sheet when one is open, and
    /// nothing else on the screen.
    func capture(to file: URL) async throws {
        guard let window else { throw Failure(description: "no window") }
        for _ in 0..<3 where window.frame.size != Self.windowSize {
            window.setContentSize(Self.windowSize)
            await settle(1000)
        }
        guard window.frame.size == Self.windowSize else {
            throw Failure(description: "window is \(Int(window.frame.width))x\(Int(window.frame.height)), not \(Int(Self.windowSize.width))x\(Int(Self.windowSize.height))")
        }
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        await settle(150)
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/sbin/screencapture")
        process.arguments = ["-x", "-o", "-l", String(window.windowNumber), file.path]
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, any Error>) in
            process.terminationHandler = { finished in
                if finished.terminationStatus == 0 {
                    continuation.resume()
                } else {
                    continuation.resume(throwing: Failure(description: "screencapture exited \(finished.terminationStatus)"))
                }
            }
            do {
                try process.run()
            } catch {
                continuation.resume(throwing: error)
            }
        }
        let size = (try? FileManager.default.attributesOfItem(atPath: file.path)[.size] as? Int) ?? 0
        guard size > 0 else { throw Failure(description: "empty image") }
    }
}
#endif
