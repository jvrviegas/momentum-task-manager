import Foundation

/// A port of internal/app/sync_state.go: when to run `task sync`, how long
/// undo stays available, and what quitting with unsynced changes does.
struct SyncState: Equatable {
    enum Phase: Equatable { case disabled, waiting, inFlight, grace, retrying, quitWait }
    enum Action: Equatable { case none, run, refresh, promptQuit, quit, cancelQuit }
    enum Event: Equatable {
        case startup, timer, mutation, manual, succeeded, failed
        case quitRequested, syncAndQuit, quitLocally, quitCancelled, undo
    }

    struct Effect: Equatable {
        var action: Action = .none
        var nextAt: Date?
        var retryDelay: TimeInterval = 0
        var message = ""
    }

    static let retryDelays: [TimeInterval] = [15, 30, 60, 120, 300]

    var enabled: Bool
    var startupEnabled: Bool
    var shutdown: Bool
    var interval: TimeInterval
    var mutationDelay: TimeInterval
    var phase: Phase = .waiting
    var nextAt: Date?
    var retryIndex = 0
    var unsynced = false
    var undoAvailable = false
    var undoUntil: Date?
    var quitPending = false

    /// Sync runs only when Momentum's config enables it and Taskwarrior has a
    /// server configured; otherwise Momentum is local-only.
    init(settings: SyncSettings, configured: Bool, now: Date) {
        enabled = settings.enabled && configured
        startupEnabled = settings.startup
        shutdown = settings.shutdown
        interval = settings.intervalSeconds > 0 ? TimeInterval(settings.intervalSeconds) : 300
        mutationDelay = TimeInterval(max(0, settings.mutationDelaySeconds))
        if !enabled {
            phase = .disabled
            return
        }
        nextAt = startupEnabled ? now : now.addingTimeInterval(interval)
    }

    static let localOnly = SyncState(
        settings: SyncSettings(enabled: false, startup: false, shutdown: false, intervalSeconds: 0, mutationDelaySeconds: 0),
        configured: false, now: .now)

    mutating func apply(_ event: Event, at: Date) -> Effect {
        if !enabled {
            phase = .disabled
            nextAt = nil
            unsynced = false
            undoUntil = nil
            switch event {
            case .mutation:
                undoAvailable = true
            case .undo where undoAvailable:
                undoAvailable = false
                return Effect(action: .refresh, message: "Last change undone")
            default:
                break
            }
            return Effect()
        }

        switch event {
        case .startup:
            if phase == .inFlight { return Effect() }
            if startupEnabled && !unsynced && !quitPending { return beginSync() }
            return Effect()
        case .timer:
            guard [.waiting, .retrying, .grace].contains(phase), let nextAt, nextAt <= at else { return Effect() }
            if phase == .grace {
                undoAvailable = false
                undoUntil = nil
            }
            return beginSync()
        case .mutation:
            unsynced = true
            undoAvailable = true
            undoUntil = at.addingTimeInterval(mutationDelay)
            nextAt = undoUntil
            phase = .grace
            return Effect(nextAt: nextAt, message: "Changes pending sync")
        case .manual:
            if phase == .inFlight { return Effect() }
            undoAvailable = false
            undoUntil = nil
            return beginSync()
        case .succeeded:
            unsynced = false
            undoAvailable = false
            undoUntil = nil
            retryIndex = 0
            phase = .waiting
            nextAt = at.addingTimeInterval(interval)
            if quitPending {
                quitPending = false
                return Effect(action: .quit, nextAt: nextAt, message: "Changes synced; quitting")
            }
            return Effect(action: .refresh, nextAt: nextAt, message: "Changes synced")
        case .failed:
            phase = .retrying
            retryIndex = min(max(retryIndex, 0), Self.retryDelays.count - 1)
            let delay = Self.retryDelays[retryIndex]
            if retryIndex < Self.retryDelays.count - 1 { retryIndex += 1 }
            nextAt = at.addingTimeInterval(delay)
            return Effect(nextAt: nextAt, retryDelay: delay, message: "Sync unavailable; retry scheduled")
        case .quitRequested:
            if !unsynced || !shutdown { return Effect(action: .quit) }
            quitPending = true
            phase = .quitWait
            return Effect(action: .promptQuit, message: "Unsynced changes")
        case .syncAndQuit:
            guard quitPending else { return Effect() }
            undoAvailable = false
            undoUntil = nil
            phase = .inFlight
            return Effect(action: .run, message: "Syncing before quit")
        case .quitLocally:
            quitPending = false
            return Effect(action: .quit)
        case .quitCancelled:
            quitPending = false
            phase = unsynced ? .grace : .waiting
            return Effect(action: .cancelQuit)
        case .undo:
            guard undoAvailable && unsynced else { return Effect() }
            unsynced = false
            undoAvailable = false
            undoUntil = nil
            phase = .waiting
            nextAt = at.addingTimeInterval(interval)
            return Effect(action: .refresh, nextAt: nextAt, message: "Last change undone")
        }
    }

    private mutating func beginSync() -> Effect {
        phase = .inFlight
        nextAt = nil
        return Effect(action: .run)
    }
}
