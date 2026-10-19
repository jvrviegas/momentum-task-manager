import AppKit
import SwiftUI

@main
struct MomentumApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @State private var store = AppStore(bridge: ProcessBridge.bundled())

    var body: some Scene {
        Window("Momentum Task Manager", id: "main") {
            ContentView()
                .environment(store)
                .frame(minWidth: 760, minHeight: 460)
                .task {
                    delegate.store = store
                    // Unit tests run inside the app; they must not start Taskwarrior.
                    guard ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil else { return }
                    await store.start()
                }
        }
        .defaultSize(width: 1120, height: 720)
        .commands { MomentumCommands(store: store) }
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    weak var store: AppStore?

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    /// Mirrors the TUI's quit flow: with unsynced changes, offer to sync first.
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard let store, store.needsQuitPrompt else { return .terminateNow }
        let alert = NSAlert()
        alert.messageText = "You have changes that haven’t synced"
        alert.informativeText = "They are saved in Taskwarrior on this Mac. Sync now to share them with your other devices, or quit and sync them next time."
        alert.addButton(withTitle: "Sync and Quit")
        alert.addButton(withTitle: "Quit Without Syncing")
        alert.addButton(withTitle: "Cancel")
        present(alert, in: sender) { response in
            switch response {
            case .alertFirstButtonReturn:
                Task { await self.syncAndQuit(sender, store: store) }
            case .alertSecondButtonReturn:
                sender.reply(toApplicationShouldTerminate: true)
            default:
                sender.reply(toApplicationShouldTerminate: false)
            }
        }
        return .terminateLater
    }

    private func syncAndQuit(_ sender: NSApplication, store: AppStore) async {
        guard let failure = await store.syncBeforeQuit() else {
            sender.reply(toApplicationShouldTerminate: true)
            return
        }
        let failed = NSAlert()
        failed.messageText = "Sync failed"
        failed.informativeText = "\(failure)\n\nYour changes stay in Taskwarrior on this Mac."
        failed.addButton(withTitle: "Quit Anyway")
        failed.addButton(withTitle: "Cancel")
        present(failed, in: sender) { response in
            sender.reply(toApplicationShouldTerminate: response == .alertFirstButtonReturn)
        }
    }

    /// Shows the alert as a sheet on the main window, or modally without one.
    private func present(_ alert: NSAlert, in app: NSApplication, then handle: @escaping (NSApplication.ModalResponse) -> Void) {
        if let window = app.mainWindow ?? app.windows.first(where: \.isVisible) {
            alert.beginSheetModal(for: window, completionHandler: handle)
        } else {
            handle(alert.runModal())
        }
    }
}
