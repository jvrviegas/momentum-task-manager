import Foundation
import Testing
@testable import Momentum

/// The links the app opened in the browser, in order.
@MainActor
final class OpenedLinks {
    var urls: [URL] = []
}

/// Waits, a thousandth of a second at a time, for something the model does on
/// its own to happen.
@MainActor
private func until(_ condition: () -> Bool) async {
    for _ in 0..<3000 where !condition() {
        try? await Task.sleep(for: .milliseconds(1))
    }
}

private let connectedStatus = GoogleCalendarStatus(offered: true, connected: true, email: "joao@gmail.com", needsReconnect: false)
/// The connection is still there, but Google no longer accepts it.
private let refusedStatus = GoogleCalendarStatus(offered: true, connected: true, email: "joao@gmail.com", needsReconnect: true)

@MainActor
struct GoogleCalendarTests {
    private func makeStore(
        signedIn: Bool = true, link: CalendarConnectLink? = nil
    ) async -> (AppStore, FakeBridge, OpenedLinks, UserDefaults) {
        let bridge = FakeBridge(snapshots: [snapshot(today: [.fixture("a")])])
        if let link { await bridge.setGoogle(link: link) }
        let defaults = UserDefaults(suiteName: "momentum-tests-\(UUID().uuidString)")!
        let signIns = FakeSignInStore()
        if signedIn { signIns.saved[backendURL] = signIn("mtk_test") }
        let opened = OpenedLinks()
        let store = AppStore(
            bridge: bridge, signIns: signIns, calendarDefaults: defaults,
            sleep: { _ in try await Task.sleep(for: .milliseconds(2)) }, openURL: { opened.urls.append($0) })
        await store.start()
        await store.settings.reload()
        return (store, bridge, opened, defaults)
    }

    // MARK: Where the account stands

    @Test func aSignedOutMacHasNoAccountToAskAbout() async {
        let (store, bridge, _, _) = await makeStore(signedIn: false)
        await store.googleCalendar.refresh()
        #expect(store.googleCalendar.phase == .signedOut)
        #expect(await !bridge.calls.contains("calendarStatus"))
    }

    @Test func aServerThatOffersNothingSaysSo() async {
        let (store, bridge, _, _) = await makeStore()
        await bridge.setGoogle(status: GoogleCalendarStatus(offered: false, connected: false, email: nil, needsReconnect: false))
        await store.googleCalendar.refresh()
        #expect(store.googleCalendar.phase == .notOffered)
    }

    @Test func anAccountWithNoConnectionOffersToConnect() async {
        let (store, _, _, _) = await makeStore()
        await store.googleCalendar.refresh()
        #expect(store.googleCalendar.phase == .notConnected)
    }

    @Test func theConnectionBelongsToTheAccountSoAnotherMacSeesItWithoutTheBrowser() async {
        let (store, bridge, opened, _) = await makeStore()
        await bridge.setGoogle(status: connectedStatus)
        await store.googleCalendar.refresh()
        #expect(store.googleCalendar.phase == .connected(email: "joao@gmail.com"))
        #expect(opened.urls.isEmpty)
        #expect(await !bridge.calls.contains("connectCalendar"))
    }

    @Test func aConnectionGoogleRefusedNeedsReconnecting() async {
        let (store, bridge, _, _) = await makeStore()
        await bridge.setGoogle(status: refusedStatus)
        await store.googleCalendar.refresh()
        #expect(store.googleCalendar.phase == .needsReconnecting(email: "joao@gmail.com"))
    }

    // MARK: Reconnecting

    @Test func reconnectingFollowsTheConnectFlowAndMayUseAnotherAccount() async {
        let link = CalendarConnectLink(url: "https://momentum.example/calendar/connect/new", expiresAt: .now.addingTimeInterval(600))
        let (store, bridge, opened, _) = await makeStore(link: link)
        await bridge.setGoogle(status: refusedStatus)
        let google = store.googleCalendar
        await google.refresh()
        let loadsBefore = await bridge.calls.filter { $0 == "load" }.count

        let wait = google.connect()
        await until { google.phase == .waiting(link) }
        #expect(opened.urls == [URL(string: link.url)!])
        // Still refused while the person is in the browser: the tab keeps waiting.
        try? await Task.sleep(for: .milliseconds(20))
        #expect(google.phase == .waiting(link))

        await bridge.setGoogle(status: GoogleCalendarStatus(offered: true, connected: true, email: "joao@work.example", needsReconnect: false))
        await wait.value
        #expect(google.phase == .connected(email: "joao@work.example"))
        // The sidebar's meter and the next Plan Today count the meetings again.
        #expect(await bridge.calls.filter { $0 == "load" }.count == loadsBefore + 1)
    }

    @Test func cancelWhileReconnectingGoesBackToNeedsReconnecting() async {
        let link = CalendarConnectLink(url: "https://momentum.example/calendar/connect/new", expiresAt: .now.addingTimeInterval(600))
        let (store, bridge, _, _) = await makeStore(link: link)
        await bridge.setGoogle(status: refusedStatus)
        let google = store.googleCalendar
        await google.refresh()
        let wait = google.connect()
        await until { google.phase == .waiting(link) }

        google.cancel()
        #expect(google.phase == .needsReconnecting(email: "joao@gmail.com"))
        await wait.value
        #expect(google.phase == .needsReconnecting(email: "joao@gmail.com"))
    }

    @Test func anExpiredReconnectLinkAsksToReconnectNotToConnect() async {
        let link = CalendarConnectLink(url: "https://momentum.example/calendar/connect/new", expiresAt: .now.addingTimeInterval(0.05))
        let (store, bridge, _, _) = await makeStore(link: link)
        await bridge.setGoogle(status: refusedStatus)
        let google = store.googleCalendar
        await google.refresh()
        await google.connect().value
        #expect(google.phase == .expired(reconnecting: true))
    }

    @Test func aFailedCheckSaysWhy() async {
        let (store, bridge, _, _) = await makeStore()
        await bridge.setGoogle(failure: BridgeError(message: "Momentum can’t reach https://momentum.example. Check your connection.", code: "unreachable"))
        await store.googleCalendar.refresh()
        #expect(store.googleCalendar.phase == .failed("Momentum can’t reach https://momentum.example. Check your connection."))
    }

    // MARK: Connecting

    @Test func connectingOpensTheLinkAndWaitsUntilTheAccountIsConnected() async {
        let link = CalendarConnectLink(url: "https://momentum.example/calendar/connect/abc", expiresAt: .now.addingTimeInterval(600))
        let (store, bridge, opened, _) = await makeStore(link: link)
        let google = store.googleCalendar
        let loadsBefore = await bridge.calls.filter { $0 == "load" }.count

        let wait = google.connect()
        await until { google.phase == .waiting(link) }
        #expect(google.phase == .waiting(link))
        #expect(opened.urls == [URL(string: link.url)!])

        await bridge.setGoogle(status: connectedStatus)
        await wait.value
        #expect(google.phase == .connected(email: "joao@gmail.com"))
        // The sidebar's meter and the next Plan Today use it now.
        #expect(await bridge.calls.filter { $0 == "load" }.count == loadsBefore + 1)
    }

    @Test func cancelStopsWaitingAndConnectsNothing() async {
        let link = CalendarConnectLink(url: "https://momentum.example/calendar/connect/abc", expiresAt: .now.addingTimeInterval(600))
        let (store, bridge, _, _) = await makeStore(link: link)
        let google = store.googleCalendar
        let wait = google.connect()
        await until { google.phase == .waiting(link) }

        google.cancel()
        #expect(google.phase == .notConnected)
        // Whatever happens at Google now, the tab is no longer listening.
        await bridge.setGoogle(status: connectedStatus)
        await wait.value
        #expect(google.phase == .notConnected)
    }

    @Test func aLinkThatExpiresEndsTheWait() async {
        let link = CalendarConnectLink(url: "https://momentum.example/calendar/connect/abc", expiresAt: .now.addingTimeInterval(0.05))
        let (store, _, _, _) = await makeStore(link: link)
        let google = store.googleCalendar
        await google.connect().value
        #expect(google.phase == .expired(reconnecting: false))
    }

    @Test func openBrowserOpensTheSameLinkAgain() async {
        let link = CalendarConnectLink(url: "https://momentum.example/calendar/connect/abc", expiresAt: .now.addingTimeInterval(600))
        let (store, _, opened, _) = await makeStore(link: link)
        let google = store.googleCalendar
        google.connect()
        await until { google.phase == .waiting(link) }
        google.openBrowserAgain()
        #expect(opened.urls == [URL(string: link.url)!, URL(string: link.url)!])
        google.cancel()
    }

    @Test func aConnectThatFailsSaysWhyAndOpensNothing() async {
        let (store, bridge, opened, _) = await makeStore()
        await bridge.setGoogle(failure: BridgeError(message: "Google Calendar isn’t available on this server"))
        await store.googleCalendar.connect().value
        #expect(store.googleCalendar.phase == .failed("Google Calendar isn’t available on this server"))
        #expect(opened.urls.isEmpty)
    }

    @Test func theTabLeavesAWaitAloneWhenItRefreshes() async {
        let link = CalendarConnectLink(url: "https://momentum.example/calendar/connect/abc", expiresAt: .now.addingTimeInterval(600))
        let (store, _, _, _) = await makeStore(link: link)
        let google = store.googleCalendar
        google.connect()
        await until { google.phase == .waiting(link) }
        await google.refresh()
        #expect(google.phase == .waiting(link))
        google.cancel()
    }

    // MARK: Disconnecting

    @Test func disconnectingEndsTheConnectionLoadsAgainAndLeavesTheSourceAlone() async {
        let (store, bridge, _, _) = await makeStore()
        await store.settings.chooseCalendarSource(.google)
        await bridge.setGoogle(status: connectedStatus)
        let google = store.googleCalendar
        await google.refresh()
        let loadsBefore = await bridge.calls.filter { $0 == "load" }.count
        let savesBefore = await bridge.saves.count
        #expect(google.email == "joao@gmail.com")

        google.confirmingDisconnect = true
        await google.disconnect()
        #expect(google.confirmingDisconnect == false)
        #expect(google.phase == .notConnected, "the tab offers Connect… again")
        #expect(google.notRevoked == false && google.disconnectError == nil)
        #expect(await bridge.calls.filter { $0 == "disconnectCalendar" }.count == 1)
        // Plan Today and the meter give up the meetings at once.
        #expect(await bridge.calls.filter { $0 == "load" }.count == loadsBefore + 1)
        // The Source stays Google until the person chooses another.
        #expect(store.settings.calendarChoice == .google)
        #expect(await bridge.saves.count == savesBefore)
    }

    @Test func aDisconnectGoogleCouldNotRevokeSaysSoUntilTheNextConnect() async {
        let link = CalendarConnectLink(url: "https://momentum.example/calendar/connect/new", expiresAt: .now.addingTimeInterval(600))
        let (store, bridge, _, _) = await makeStore(link: link)
        await bridge.setGoogle(status: connectedStatus, revoked: false)
        let google = store.googleCalendar
        await google.refresh()

        await google.disconnect()
        #expect(google.phase == .notConnected)
        #expect(google.notRevoked, "the access may still show in the Google account's settings")

        // Looking again changes nothing; connecting again clears it.
        await google.refresh()
        #expect(google.notRevoked)
        google.connect()
        await until { google.phase == .waiting(link) }
        #expect(google.notRevoked == false)
        google.cancel()
    }

    @Test func aDisconnectThatFailsChangesNothingAndTheButtonTriesAgain() async {
        let (store, bridge, _, _) = await makeStore()
        await bridge.setGoogle(status: connectedStatus)
        let google = store.googleCalendar
        await google.refresh()

        await bridge.setGoogle(failure: BridgeError(message: "Momentum can’t reach https://momentum.example. Check your connection.", code: "unreachable"))
        await google.disconnect()
        #expect(google.phase == .connected(email: "joao@gmail.com"))
        #expect(google.disconnectError == "Momentum can’t reach https://momentum.example. Check your connection.")

        await bridge.setGoogle(failure: nil)
        await google.disconnect()
        #expect(google.phase == .notConnected)
        #expect(google.disconnectError == nil)
    }

    @Test func cancellingThePromptAsksTheServerNothing() async {
        let (store, bridge, _, _) = await makeStore()
        await bridge.setGoogle(status: connectedStatus)
        let google = store.googleCalendar
        await google.refresh()
        google.confirmingDisconnect = true
        google.confirmingDisconnect = false
        #expect(google.phase == .connected(email: "joao@gmail.com"))
        #expect(await !bridge.calls.contains("disconnectCalendar"))
    }

    @Test func aConnectionThatNeedsReconnectingCanBeDisconnected() async {
        let (store, bridge, _, _) = await makeStore()
        await bridge.setGoogle(status: refusedStatus)
        let google = store.googleCalendar
        await google.refresh()
        #expect(google.email == "joao@gmail.com")
        await google.disconnect()
        #expect(google.phase == .notConnected)
    }

    @Test func thereIsNothingToDisconnectBeforeAConnection() async {
        let (store, bridge, _, _) = await makeStore()
        let google = store.googleCalendar
        await google.refresh()
        #expect(google.email == nil)
        await google.disconnect()
        #expect(await !bridge.calls.contains("disconnectCalendar"))
    }

    // MARK: The wire

    /// What the Go bridge answers, byte for byte, read the way ProcessBridge reads it.
    @Test func theBridgesAnswersDecode() throws {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let status = try decoder.decode(
            GoogleCalendarStatus.self, from: Data(#"{"offered":true,"connected":true,"email":"joao@gmail.com","needsReconnect":false}"#.utf8))
        #expect(status == connectedStatus)
        // A server that offers nothing sends no address.
        let bare = try decoder.decode(GoogleCalendarStatus.self, from: Data(#"{"offered":false,"connected":false,"needsReconnect":false}"#.utf8))
        #expect(bare == GoogleCalendarStatus(offered: false, connected: false, email: nil, needsReconnect: false))
        let link = try decoder.decode(
            CalendarConnectLink.self, from: Data(#"{"url":"https://m.example/calendar/connect/abc","expiresAt":"2026-10-14T09:30:00Z"}"#.utf8))
        #expect(link.url == "https://m.example/calendar/connect/abc")
        #expect(link.expiresAt == ISO8601DateFormatter().date(from: "2026-10-14T09:30:00Z"))
    }

    @Test func theBridgesDisconnectAnswerDecodes() throws {
        let answer = try JSONDecoder().decode(GoogleCalendarDisconnect.self, from: Data(#"{"revoked":false}"#.utf8))
        #expect(answer == GoogleCalendarDisconnect(revoked: false))
    }

    @Test func aSourceChangeSendsOnlyTheRevisionAndTheSource() throws {
        let encoder = JSONEncoder()
        encoder.outputFormatting = .sortedKeys
        let data = try encoder.encode(CalendarChange(revision: "r1", source: "google"))
        #expect(String(decoding: data, as: UTF8.self) == #"{"revision":"r1","source":"google"}"#)
    }

    // MARK: Choosing the source

    @Test func choosingGoogleWritesTheSourceAndShowsItInThePicker() async {
        let (store, bridge, _, _) = await makeStore()
        #expect(store.settings.calendarChoice == .files)
        await store.settings.chooseCalendarSource(.google)
        #expect(await bridge.settingsValue.calendar.source == "google")
        #expect(store.settings.calendarChoice == .google)
    }

    @Test func choosingGoogleOrFilesClearsAMacOSCalendarChoiceThatWouldCountOverThem() async {
        let (store, bridge, _, defaults) = await makeStore()
        await store.macCalendar.choose(.mac)
        #expect(store.settings.calendarChoice == .mac)
        #expect(await bridge.saves.isEmpty, "macOS Calendar is this Mac's own choice and writes nothing")

        await store.settings.chooseCalendarSource(.google)
        #expect(defaults.string(forKey: MacCalendarModel.sourceKey) == "files")
        #expect(store.macCalendar.source == .files)
        #expect(store.settings.calendarChoice == .google)
        #expect(await bridge.settingsValue.calendar.source == "google")

        await store.macCalendar.choose(.mac)
        await store.settings.chooseCalendarSource(.files)
        #expect(store.macCalendar.source == .files)
        #expect(store.settings.calendarChoice == .files)
        #expect(await bridge.settingsValue.calendar.source == "files")
    }

    @Test func aMacOSCalendarChoiceCountsOverWhatConfigSays() async {
        let (store, bridge, _, _) = await makeStore()
        await store.settings.chooseCalendarSource(.google)
        await store.macCalendar.choose(.mac)
        #expect(store.settings.calendarChoice == .mac)
        #expect(await bridge.settingsValue.calendar.source == "google", "choosing macOS Calendar never changes config.toml")
    }

    @Test func choosingTheSourceAlreadyChosenWritesNothing() async {
        let (store, bridge, _, _) = await makeStore()
        await store.settings.chooseCalendarSource(.files)
        #expect(await bridge.saves.isEmpty)
    }

    @Test func aChoiceTheConfigRefusesShowsItsMessageUnderThePicker() async {
        let (store, bridge, _, _) = await makeStore()
        await store.settings.chooseCalendarSource(.google)
        await bridge.setSaveFailure(BridgeError(message: "Add a calendar file first, or turn off Use calendar files.", field: "source"))
        await store.settings.chooseCalendarSource(.files)
        #expect(store.settings.error("calendar", "source") == "Add a calendar file first, or turn off Use calendar files.")
        #expect(store.settings.calendarChoice == .google)
    }
}
