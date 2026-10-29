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
    /// Opens the Settings window, as ⌘, does. The window's content registers
    /// it, since only a view can reach the scene's action.
    var openSettings: () -> Void = {}
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
        return AppStore(
            bridge: SnapshotBridge(scene.config), signIns: SnapshotSignInStore(saved),
            calendarSource: SnapshotCalendarSource(access: scene.macAccess))
    }

    /// Takes every page in light and dark, then quits.
    func run() async {
        // Ten seconds for each image, since a page takes about five and the
        // number of pages grows with the app.
        let images = SnapshotScene.all(now: .now).filter { only?.contains($0.name) ?? true }.count * 2
        let watchdog = Task {
            try await Task.sleep(for: .seconds(max(420, 60 + 10 * Double(images))))
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
        for check in await checkSettingsEntryPoints() {
            print("SNAPSHOT check \(check.passed ? "ok" : "FAIL"): \(check.name)")
            if !check.passed { failures += 1 }
        }
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

    /// Opens Settings the two ways a person does, on a signed-in window: the
    /// app menu's Settings… item (⌘,) and the sidebar's Settings row.
    private func checkSettingsEntryPoints() async -> [(name: String, passed: Bool)] {
        SnapshotSession.reset()
        store = Self.makeStore(SnapshotScene(name: ""))
        generation += 1
        let session = SnapshotSession(store: store, openSettings: openSettings)
        await store.start()
        await session.settle(1500)
        var checks: [(name: String, passed: Bool)] = []

        let menu = NSApp.mainMenu?.items.compactMap(\.submenu).first { $0.items.contains { $0.keyEquivalent == "," } }
        let index = menu?.items.firstIndex { $0.keyEquivalent == "," && $0.keyEquivalentModifierMask == .command }
        checks.append(("the app menu has Settings… with ⌘,", menu != nil && index != nil && menu?.items[index!].title == "Settings…"))
        if let menu, let index {
            menu.performActionForItem(at: index)
            await session.settle(1200)
            checks.append(("the app menu's Settings… opens the window", NSApp.windows.contains { $0.isVisible && SnapshotSession.isSettings($0) }))
        }

        SnapshotSession.reset()
        try? await Task.sleep(for: .milliseconds(600))
        // The footer's rows are 28 points tall with 2 between, 12 above the
        // bottom edge: Keyboard Shortcuts is the lowest, Settings the one above.
        let settingsOpen = { NSApp.windows.contains { $0.isVisible && SnapshotSession.isSettings($0) } }
        let wasClosed = !settingsOpen()
        session.click(x: 60, y: 12 + 28 + 2 + 14)
        await session.settle(1200)
        checks.append(("a click on the sidebar's Settings row opens the window", wasClosed && settingsOpen()))
        SnapshotSession.reset()
        return checks
    }

    private func take(_ scene: SnapshotScene, look: String) async throws {
        SnapshotSession.reset()
        try? await Task.sleep(for: .milliseconds(400))
        store = Self.makeStore(scene)
        generation += 1
        let session = SnapshotSession(store: store, openSettings: openSettings)
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
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        ContentView()
            .environment(host.store)
            .environment(\.openURL, OpenURLAction { _ in .handled })
            .id(host.generation)
            .onAppear { host.openSettings = { openSettings() } }
    }
}

/// The Settings window's content while snapshots run.
struct SnapshotSettingsRoot: View {
    let host: SnapshotHost

    var body: some View {
        // No `.id` here: rebuilding the tab view inside a window that already
        // exists loses its toolbar tabs, and the store in the environment
        // already updates the content.
        SettingsView()
            .environment(host.store)
    }
}

/// The pieces a page's steps use: the store, the window, and the text field a
/// capture sheet types into.
@MainActor
final class SnapshotSession {
    enum Target { case main, settings }

    let store: AppStore
    private let showSettings: () -> Void
    /// The window the image shows: the main window, or Settings.
    var target = Target.main

    init(store: AppStore, openSettings: @escaping () -> Void) {
        self.store = store
        showSettings = openSettings
    }

    struct Failure: Error, CustomStringConvertible {
        var description: String
    }

    /// The window's default size, so every image has the same frame.
    static let windowSize = NSSize(width: 1120, height: 720)

    func settle(_ milliseconds: Int = 700) async {
        try? await Task.sleep(for: .milliseconds(milliseconds))
    }

    /// Clicks the main window at a point measured from its bottom-left corner,
    /// as the pointer would.
    func click(x: CGFloat, y: CGFloat) {
        guard let window else { return }
        for type in [NSEvent.EventType.leftMouseDown, .leftMouseUp] {
            guard let event = NSEvent.mouseEvent(
                with: type, location: CGPoint(x: x, y: y), modifierFlags: [], timestamp: ProcessInfo.processInfo.systemUptime,
                windowNumber: window.windowNumber, context: nil, eventNumber: 0, clickCount: 1, pressure: 1)
            else { return }
            window.sendEvent(event)
        }
    }

    static func isSettings(_ window: NSWindow) -> Bool {
        window.identifier?.rawValue == "com_apple_SwiftUI_Settings_window"
    }

    var window: NSWindow? {
        NSApp.windows.first {
            $0.isVisible && !$0.isSheet && $0.canBecomeMain && (Self.isSettings($0) == (target == .settings))
        }
    }

    /// Starts a page from nothing: no Settings window open, and the Mac's own
    /// appearance chosen.
    static func reset() {
        NSApp.windows.filter(isSettings).forEach { $0.performClose(nil) }
        UserDefaults.standard.removeObject(forKey: AppearanceChoice.key)
        UserDefaults.standard.removeObject(forKey: "settings.tab")
        UserDefaults.standard.removeObject(forKey: MacCalendarModel.sourceKey)
        UserDefaults.standard.removeObject(forKey: MacCalendarModel.offKey)
    }

    /// Closes Settings as ⌘W does, then opens it again, as ⌘, would.
    func reopenSettings(_ tab: SettingsTab) async {
        await openSettings(tab)
        window?.performClose(nil)
        await settle()
        await openSettings(tab)
    }

    func chooseAppearance(_ choice: AppearanceChoice) async {
        UserDefaults.standard.set(choice.rawValue, forKey: AppearanceChoice.key)
        await settle()
    }

    /// Opens Settings on Projects, with the list read.
    func openProjects() async {
        await openSettings(.projects)
        await store.catalog.reload()
        await settle()
    }

    /// A row of the Projects list, by value.
    func projectRow(_ value: String) -> ProjectRow {
        guard let row = store.catalog.rows.first(where: { $0.value == value }) else { fatalError("no project row \(value)") }
        return row
    }

    /// Opens Settings on a tab, as ⌘, does, and makes it the window the image shows.
    func openSettings(_ tab: SettingsTab) async {
        UserDefaults.standard.set(tab.rawValue, forKey: "settings.tab")
        showSettings()
        target = .settings
        await settle(1200)
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

    /// Types into a text field of the window the image shows and presses
    /// Return, as a person would. Fields are counted in the order the window
    /// lists them: on the Planning tab Daily capacity is 0, Buffer 1, then
    /// Monday to Sunday.
    func typeInField(at index: Int, _ text: String) async {
        func fields(_ view: NSView?) -> [NSTextField] {
            guard let view else { return [] }
            return (view as? NSTextField).map { [$0] } ?? view.subviews.flatMap(fields)
        }
        guard let window, case let all = fields(window.contentView), all.indices.contains(index) else { return }
        let field = all[index]
        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(field)
        await settle(300)
        guard let editor = field.currentEditor() as? NSTextView else { return }
        editor.insertText(text, replacementRange: NSRange(location: 0, length: editor.string.utf16.count))
        await settle(300)
        editor.doCommand(by: #selector(NSResponder.insertNewline(_:)))
        await settle()
    }

    func pressReturn() async {
        textView(in: window?.attachedSheet?.contentView)?.doCommand(by: #selector(NSResponder.insertNewline(_:)))
        await settle()
    }

    private func textView(in view: NSView?) -> NSTextView? {
        guard let view else { return nil }
        if let text = view as? NSTextView { return text }
        return view.subviews.lazy.compactMap { self.textView(in: $0) }.first
    }

    /// Saves the app's own window, with its sheet when one is open, and
    /// nothing else on the screen.
    func capture(to file: URL) async throws {
        guard let window else { throw Failure(description: "no window") }
        if target == .main {
            for _ in 0..<3 where window.frame.size != Self.windowSize {
                window.setContentSize(Self.windowSize)
                await settle(1000)
            }
            guard window.frame.size == Self.windowSize else {
                throw Failure(description: "window is \(Int(window.frame.width))x\(Int(window.frame.height)), not \(Int(Self.windowSize.width))x\(Int(Self.windowSize.height))")
            }
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
