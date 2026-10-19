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
                    // Unit tests run inside the app; they must not start the bridge.
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
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }
}
