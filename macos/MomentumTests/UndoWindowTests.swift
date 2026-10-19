import Foundation
import Testing
@testable import Momentum

// Mirrors internal/app/undo_test.go so the port cannot drift silently.
struct UndoWindowTests {
    let now = Date(timeIntervalSince1970: 1_800_000_000)

    @Test func opensForTheDelayAndCloses() {
        var window = UndoWindow(delay: 15)
        window.open(at: now)
        #expect(window.isAvailable(at: now) && window.isAvailable(at: now.addingTimeInterval(14)))
        #expect(!window.isAvailable(at: now.addingTimeInterval(15)))
        window.open(at: now.addingTimeInterval(10))
        #expect(window.until == now.addingTimeInterval(25))
        window.close()
        #expect(!window.isAvailable(at: now) && window.delay == 15)
    }

    @Test func aZeroDelayNeverOpens() {
        var window = UndoWindow(delay: 0)
        window.open(at: now)
        #expect(!window.isAvailable(at: now))
    }
}
