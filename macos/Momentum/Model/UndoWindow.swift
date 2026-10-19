import Foundation

/// A port of internal/app/undo.go: how long the latest change can be undone.
/// The server keeps one step of history per device, and the app offers to
/// undo it for mutation_delay after the change (backend plan D7).
struct UndoWindow: Equatable {
    var delay: TimeInterval
    /// When the window closes; nil while nothing can be undone.
    var until: Date?

    init(delay: TimeInterval) {
        self.delay = max(0, delay)
    }

    /// Starts the window for a change made at `at`. A zero delay keeps it
    /// closed.
    mutating func open(at: Date) {
        until = delay > 0 ? at.addingTimeInterval(delay) : nil
    }

    /// Ends the window, as an undo or signing out does.
    mutating func close() {
        until = nil
    }

    /// Whether undo is offered at `date`.
    func isAvailable(at date: Date) -> Bool {
        until.map { $0 > date } ?? false
    }
}
