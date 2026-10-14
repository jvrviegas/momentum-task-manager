import Foundation
import Testing
@testable import Momentum

// Mirrors internal/app/sync_state_test.go so the port cannot drift silently.
struct SyncStateTests {
    let settings = SyncSettings(enabled: true, startup: true, shutdown: true, intervalSeconds: 300, mutationDelaySeconds: 15)
    let now = Date(timeIntervalSince1970: 1_800_000_000)

    @Test func startupOrPeriodicScheduling() {
        #expect(SyncState(settings: settings, configured: true, now: now).nextAt == now)
        var periodic = settings
        periodic.startup = false
        #expect(SyncState(settings: periodic, configured: true, now: now).nextAt == now.addingTimeInterval(300))
    }

    @Test func unconfiguredSyncIsLocalOnly() {
        var state = SyncState(settings: settings, configured: false, now: now)
        #expect(state.phase == .disabled && state.nextAt == nil)
        #expect(state.apply(.timer, at: now).action == .none)
    }

    @Test func localOnlyMutationKeepsUndoWithoutSyncing() {
        var state = SyncState(settings: settings, configured: false, now: now)
        #expect(state.apply(.mutation, at: now).action == .none)
        #expect(state.undoAvailable && !state.unsynced && state.nextAt == nil && state.undoUntil == nil)
        _ = state.apply(.timer, at: now.addingTimeInterval(3600))
        #expect(state.undoAvailable)
        #expect(state.apply(.undo, at: now).action == .refresh)
        #expect(!state.undoAvailable && state.phase == .disabled)
    }

    @Test func mutationResetsUndoDelay() {
        var state = SyncState(settings: settings, configured: true, now: now)
        _ = state.apply(.mutation, at: now)
        _ = state.apply(.mutation, at: now.addingTimeInterval(10))
        #expect(state.phase == .grace && state.undoUntil == now.addingTimeInterval(25) && state.nextAt == now.addingTimeInterval(25))
    }

    @Test func graceExpiryClosesUndoBeforeSyncing() {
        var state = SyncState(settings: settings, configured: true, now: now)
        _ = state.apply(.mutation, at: now)
        #expect(state.apply(.timer, at: now.addingTimeInterval(14)).action == .none)
        #expect(state.undoAvailable)
        #expect(state.apply(.timer, at: now.addingTimeInterval(15)).action == .run)
        #expect(!state.undoAvailable && state.phase == .inFlight)
    }

    @Test func retryProgressionCapsAtFiveMinutes() {
        var state = SyncState(settings: settings, configured: true, now: now)
        let delays = (0..<7).map { _ in state.apply(.failed, at: now).retryDelay }
        #expect(delays == [15, 30, 60, 120, 300, 300, 300])
        #expect(state.phase == .retrying)
    }

    @Test func successResetsBackoffAndRefreshes() {
        var state = SyncState(settings: settings, configured: true, now: now)
        _ = state.apply(.failed, at: now)
        _ = state.apply(.failed, at: now)
        let effect = state.apply(.succeeded, at: now)
        #expect(effect.action == .refresh && state.retryIndex == 0 && state.nextAt == now.addingTimeInterval(300))
    }

    @Test func manualSyncClosesUndoWindow() {
        var state = SyncState(settings: settings, configured: true, now: now)
        _ = state.apply(.mutation, at: now)
        #expect(state.apply(.manual, at: now).action == .run)
        #expect(!state.undoAvailable)
    }

    @Test func quitFlows() {
        var clean = SyncState(settings: settings, configured: true, now: now)
        #expect(clean.apply(.quitRequested, at: now).action == .quit)

        var dirty = SyncState(settings: settings, configured: true, now: now)
        _ = dirty.apply(.mutation, at: now)
        #expect(dirty.apply(.quitRequested, at: now).action == .promptQuit)
        #expect(dirty.apply(.syncAndQuit, at: now).action == .run)
        #expect(dirty.apply(.succeeded, at: now).action == .quit)

        var cancelled = SyncState(settings: settings, configured: true, now: now)
        _ = cancelled.apply(.mutation, at: now)
        _ = cancelled.apply(.quitRequested, at: now)
        #expect(cancelled.apply(.quitCancelled, at: now).action == .cancelQuit)
        #expect(cancelled.phase == .grace && !cancelled.quitPending)
    }

    @Test func undoOnlyDuringGrace() {
        var state = SyncState(settings: settings, configured: true, now: now)
        #expect(state.apply(.undo, at: now).action == .none)
        _ = state.apply(.mutation, at: now)
        #expect(state.apply(.undo, at: now).action == .refresh)
        #expect(!state.unsynced && state.phase == .waiting)
    }
}
