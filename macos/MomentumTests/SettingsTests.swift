import Foundation
import SwiftUI
import Testing
@testable import Momentum

struct AppearanceTests {
    @Test func eachChoiceMapsToAColorScheme() {
        #expect(AppearanceChoice.system.colorScheme == nil)
        #expect(AppearanceChoice.light.colorScheme == .light)
        #expect(AppearanceChoice.dark.colorScheme == .dark)
    }

    @Test func theChoicesReadSystemLightDark() {
        #expect(AppearanceChoice.allCases.map(\.title) == ["System", "Light", "Dark"])
    }

    /// The choice is stored by its name under a key of its own: it neither
    /// reads nor writes the terminal app's `theme` in config.toml.
    @Test func theChoiceIsKeptOnThisMacUnderItsOwnKey() throws {
        #expect(AppearanceChoice.key == "appearance")
        let defaults = try #require(UserDefaults(suiteName: "momentum.tests.\(UUID().uuidString)"))
        defaults.set(AppearanceChoice.dark.rawValue, forKey: AppearanceChoice.key)
        #expect(defaults.string(forKey: AppearanceChoice.key).flatMap(AppearanceChoice.init) == .dark)
        #expect(defaults.string(forKey: "theme") == nil)
        #expect(AppearanceChoice(rawValue: "") == nil, "an unknown value falls back to the default")
    }
}

@MainActor
struct ConfigReloadTests {
    private func signedInStoreOnToday() async -> (AppStore, FakeBridge) {
        let bridge = FakeBridge(snapshots: [snapshot(today: [.fixture("a")])])
        let store = signedInStore(bridge)
        await store.start()
        return (store, bridge)
    }

    @Test func statusDecodesWhatTheConfigFileIs() throws {
        let decoder = JSONDecoder()
        let plain = try decoder.decode(BridgeStatus.self, from: Data("""
        {"configPath":"/tmp/config.toml","mutationDelaySeconds":15,"refreshIntervalSeconds":60,"backendUrl":"https://m.example"}
        """.utf8))
        #expect(plain.configError == nil && plain.restartRequired == nil)
        let refused = try decoder.decode(BridgeStatus.self, from: Data("""
        {"configPath":"/tmp/config.toml","mutationDelaySeconds":15,"refreshIntervalSeconds":60,
         "configError":"config key \\"calendar.paths\\" requires at least one local ICS path","restartRequired":true}
        """.utf8))
        #expect(refused.configError?.contains("calendar.paths") == true && refused.restartRequired == true)
    }

    @Test func aRefreshReportsAFileThatWasNotApplied() async {
        let (store, bridge) = await signedInStoreOnToday()
        #expect(store.configProblem == nil && !store.restartRequired)

        var status = await bridge.statusValue
        status.configError = "config key \"calendar.paths\" requires at least one local ICS path"
        status.restartRequired = true
        await bridge.setStatus(status)
        await store.refresh()
        #expect(store.configProblem == status.configError)
        #expect(store.restartRequired)

        status.configError = nil
        status.restartRequired = nil
        await bridge.setStatus(status)
        await store.refresh()
        #expect(store.configProblem == nil && !store.restartRequired, "a good file clears the notice")
    }

    @Test func theUndoDelayAndRefreshIntervalFollowTheFile() async {
        let (store, bridge) = await signedInStoreOnToday()
        #expect(store.undoWindow.delay == 15)

        var status = await bridge.statusValue
        status.mutationDelaySeconds = 30
        status.refreshIntervalSeconds = 120
        await bridge.setStatus(status)
        await store.refresh()
        #expect(store.undoWindow.delay == 30)
        #expect(store.status?.refreshIntervalSeconds == 120)
    }

    /// Settings asks for the status when it opens, with no load in between.
    @Test func openingSettingsReadsTheFileAgain() async {
        let (store, bridge) = await signedInStoreOnToday()
        var status = await bridge.statusValue
        status.configError = "decode config: bad"
        await bridge.setStatus(status)
        await store.refreshStatus()
        #expect(store.configProblem == "decode config: bad")
    }
}
