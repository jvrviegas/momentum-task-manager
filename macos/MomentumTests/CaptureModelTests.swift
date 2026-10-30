import AppKit
import Testing
@testable import Momentum

/// Waits, a thousandth of a second at a time, for something the model does on
/// its own to happen.
@MainActor
private func until(_ condition: () -> Bool) async {
    for _ in 0..<3000 where !condition() {
        try? await Task.sleep(for: .milliseconds(1))
    }
}

@MainActor
struct CaptureModelTests {
    /// The sheet keeps showing the last interpretation while the next one is
    /// on its way, but its chips only act on the text they were made for.
    @Test func chipsForOlderTextStayOnScreenAndDoNothing() async {
        let bridge = FakeBridge(snapshots: [snapshot()])
        let p1 = Candidate(
            field: "priority", text: "p1", value: "H", start: 9, end: 11,
            provenance: "explicit", accepted: true, conflict: false, blocking: false)
        await bridge.setCandidates([p1])
        let model = CaptureModel(store: signedInStore(bridge))
        model.textChanged("Call mom p1", cursor: 11)
        await until { model.currentInterpretation != nil }

        model.textChanged("Call my mom p1", cursor: 7)
        #expect(model.currentInterpretation == nil)
        #expect(model.interpretation?.source == "Call mom p1")
        model.keepAsText(p1)
        #expect(model.input == "Call my mom p1")

        model.textChanged("Call mom p1", cursor: 11)
        await until { model.currentInterpretation != nil }
        model.keepAsText(p1)
        #expect(model.input == "Call mom ")
    }

    /// Typing keeps the marks on their words until the next interpretation
    /// arrives, instead of clearing them for a moment.
    @Test func marksMoveWithTheTextWhileTheNextInterpretationIsOnItsWay() {
        let textView = NSTextView()
        textView.string = "Call mom p1"
        let coordinator = CaptureTextView.Coordinator(
            model: CaptureModel(store: signedInStore(FakeBridge(snapshots: [snapshot()]))), close: {})
        let p1 = Candidate(
            field: "priority", text: "p1", value: "H", start: 9, end: 11,
            provenance: "explicit", accepted: true, conflict: false, blocking: false)
        let older = Interpretation(
            token: nil, source: "Call mom p1", valid: true, requiresReview: false, description: "Call mom",
            candidates: [p1], diagnostics: [], reviewValues: [])
        coordinator.highlight(textView, older)
        #expect(marked(textView) == [9, 10])

        textView.textStorage?.replaceCharacters(in: NSRange(location: 5, length: 0), with: "my ")
        coordinator.highlight(textView, older)
        #expect(marked(textView) == [12, 13])

        coordinator.highlight(textView, nil)
        #expect(marked(textView).isEmpty)
    }

    private func marked(_ textView: NSTextView) -> [Int] {
        guard let storage = textView.textStorage else { return [] }
        return (0..<storage.length).filter { storage.attribute(.backgroundColor, at: $0, effectiveRange: nil) != nil }
    }
}
