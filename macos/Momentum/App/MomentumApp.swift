import AppKit
import SwiftUI

@main
struct MomentumApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @State private var store = AppStore(bridge: ProcessBridge.bundled())
    #if DEBUG
    /// Set by MOMENTUM_SNAPSHOT: the app walks its pages on a fake server.
    @State private var snapshots = SnapshotHost.fromEnvironment()
    #endif

    var body: some Scene {
        Window("Momentum Task Manager", id: "main") {
            content
                .frame(minWidth: 760, minHeight: 460)
                .task {
                    // Unit tests run inside the app; they must not start the bridge.
                    guard ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil else { return }
                    #if DEBUG
                    if let snapshots {
                        await snapshots.run()
                        return
                    }
                    #endif
                    await store.start()
                }
        }
        .defaultSize(width: 1120, height: 720)
        .commands { MomentumCommands(store: store) }
    }

    @ViewBuilder
    private var content: some View {
        #if DEBUG
        if let snapshots {
            SnapshotRoot(host: snapshots)
        } else {
            ContentView().environment(store)
        }
        #else
        ContentView().environment(store)
        #endif
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }
}
