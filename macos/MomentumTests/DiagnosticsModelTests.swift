import Foundation
import Testing
@testable import Momentum

@MainActor
struct DiagnosticsModelTests {
    private func makeStore(source: FakeCalendarSource? = nil) async -> (AppStore, FakeBridge) {
        let bridge = FakeBridge(snapshots: [snapshot(today: [.fixture("a")])])
        let signIns = FakeSignInStore()
        signIns.saved[backendURL] = signIn("mtk_test")
        let defaults = UserDefaults(suiteName: "momentum-tests-\(UUID().uuidString)")!
        let store = AppStore(bridge: bridge, signIns: signIns, calendarSource: source ?? NoCalendarSource(), calendarDefaults: defaults)
        await store.start()
        await store.diagnostics.run()
        return (store, bridge)
    }

    @Test func theChecksAreWhatTheBridgeAnswers() async {
        let (store, _) = await makeStore()
        #expect(store.diagnostics.checks.map(\.name) == ["Backend server", "Sign-in", "Calendar source"])
        #expect(store.diagnostics.checks.map(\.status) == ["ok", "fail", "skip"])
        #expect(!store.diagnostics.ready, "a failed check means something needs attention")
        #expect(store.diagnostics.error == nil)
    }

    @Test func theReportListsTheVersionsThePathAndEachCheck() async {
        let (store, _) = await makeStore()
        let text = store.diagnostics.reportText(app: "0.1.0 (1)", system: "Version 15.0")
        for line in [
            "Momentum Task Manager 0.1.0 (1)", "Bridge v0.1.0-test", "macOS Version 15.0", "Config /tmp/config.toml",
            "[ok] Backend server", "https://momentum.example answers; version 1.4.0", "[fail] Sign-in", "unauthorized",
            "[skip] Calendar source", "Doctor: issues found",
        ] {
            #expect(text.contains(line), "the report should hold “\(line)”:\n\(text)")
        }
    }

    @Test func theReportHoldsNoTokenAndNoEventTitle() async {
        let source = FakeCalendarSource(access: .granted)
        source.events = ["home": [CalendarEvent(start: .now, end: .now.addingTimeInterval(3600), allDay: false, free: false, title: "Secret merger talks")]]
        let (store, _) = await makeStore(source: source)
        await store.macCalendar.choose(.mac)
        await store.diagnostics.run()
        let text = store.diagnostics.reportText(app: "0.1.0 (1)", system: "Version 15.0")
        #expect(!text.contains("mtk_test") && !text.contains("Secret merger talks"))
        #expect(text.contains("macOS Calendar"), "the Mac source is in the report, without its events")
    }

    @Test func macOSCalendarIsOneMoreCheckOnlyWhenItIsTheSource() async {
        let source = FakeCalendarSource(access: .granted)
        let (store, _) = await makeStore(source: source)
        #expect(!store.diagnostics.checks.contains { $0.name == "macOS Calendar" }, "calendar files are the source")

        await store.macCalendar.choose(.mac)
        let granted = store.diagnostics.checks.last
        #expect(granted?.name == "macOS Calendar" && granted?.status == "ok" && granted?.detail == "3 calendars; read-only")

        source.access = .denied
        await store.macCalendar.refresh()
        let denied = store.diagnostics.checks.last
        #expect(denied?.status == "fail" && denied?.detail.hasPrefix("no access") == true)
    }

    @Test func aReadyBridgeStaysReadyUnlessTheMacCalendarFails() async {
        let (store, bridge) = await makeStore(source: FakeCalendarSource(access: .denied))
        await bridge.setDoctor(DoctorReport(
            configPath: "/tmp/config.toml", bridgeVersion: "v1", checks: [DoctorCheck(name: "Backend server", status: "ok", detail: "answers")], ready: true))
        await store.diagnostics.run()
        #expect(store.diagnostics.ready)
        await store.macCalendar.choose(.mac)
        #expect(!store.diagnostics.ready)
    }

    @Test func aRunThatFailedSaysWhy() async {
        let bridge = FakeBridge(snapshots: [snapshot()])
        await bridge.setDoctorFailure(BridgeError(message: "momentum-bridge exited unexpectedly."))
        let store = signedInStore(bridge)
        await store.diagnostics.run()
        #expect(store.diagnostics.error == "momentum-bridge exited unexpectedly." && store.diagnostics.report == nil)
        #expect(store.diagnostics.reportText().isEmpty)

        await bridge.setDoctorFailure(nil)
        await store.diagnostics.run()
        #expect(store.diagnostics.error == nil && store.diagnostics.report != nil, "Check Again recovers")
    }

    @Test func theAppVersionReadsTheInfoPlistAndTheBridgesAnswerDecodes() throws {
        #expect(DiagnosticsModel.appVersion(["CFBundleShortVersionString": "0.1.0", "CFBundleVersion": "7"]) == "0.1.0 (7)")
        #expect(DiagnosticsModel.appVersion(["CFBundleShortVersionString": "0.1.0"]) == "0.1.0")
        #expect(DiagnosticsModel.appVersion(nil) == "unknown")

        let report = try JSONDecoder().decode(DoctorReport.self, from: Data("""
        {"configPath":"/tmp/config.toml","bridgeVersion":"dev","ready":false,
         "checks":[{"name":"Sign-in","status":"fail","detail":"unauthorized"}]}
        """.utf8))
        #expect(report.checks == [DoctorCheck(name: "Sign-in", status: "fail", detail: "unauthorized")] && !report.ready)
    }
}
