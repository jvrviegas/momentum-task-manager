import Foundation
import Observation

/// State for one quick-capture session. Interpretation, suggestions and the
/// final add all go through the bridge, so capture behaves like the TUI.
@MainActor
@Observable
final class CaptureModel {
    let store: AppStore

    private(set) var input = ""
    /// Cursor position in Unicode scalars.
    private(set) var cursor = 0
    /// Bumped when the model changes the text, so the text view replaces it.
    private(set) var textRevision = 0
    private(set) var literals: [TextSpan] = []
    private(set) var interpretation: Interpretation?
    private(set) var parseError: String?
    private(set) var suggestions = SuggestResult.none
    var highlightedSuggestion = 0
    private(set) var suggestionsDismissed = false
    private(set) var submitting = false
    private(set) var error: String?
    var review: ReviewDraft?

    @ObservationIgnored private var interpretRevision = 0
    @ObservationIgnored private var suggestRevision = 0

    init(store: AppStore) {
        self.store = store
    }

    var visibleSuggestions: [Suggestion] {
        suggestionsDismissed || review != nil ? [] : suggestions.suggestions
    }

    /// The interpretation for exactly the current text, if it has arrived.
    var currentInterpretation: Interpretation? {
        interpretation?.source == input ? interpretation : nil
    }

    func textChanged(_ text: String, cursor: Int) {
        guard text != input else {
            cursorMoved(cursor)
            return
        }
        literals = shiftLiterals(from: input, to: text, literals)
        input = text
        self.cursor = cursor
        suggestionsDismissed = false
        error = nil
        reinterpret()
        refreshSuggestions()
    }

    func cursorMoved(_ cursor: Int) {
        guard cursor != self.cursor else { return }
        self.cursor = cursor
        refreshSuggestions()
    }

    func moveHighlight(_ delta: Int) {
        let count = visibleSuggestions.count
        guard count > 0 else { return }
        highlightedSuggestion = (highlightedSuggestion + delta + count) % count
    }

    func accept(_ index: Int? = nil) {
        let list = visibleSuggestions
        let index = index ?? highlightedSuggestion
        guard list.indices.contains(index) else { return }
        replaceText(list[index].input, cursor: list[index].cursor)
    }

    /// Esc on or right after a recognized phrase keeps it as text; otherwise
    /// it dismisses suggestions. Returns false when capture should close.
    func escape() -> Bool {
        if let phrase = currentInterpretation?.candidates.first(where: {
            $0.isInferred && $0.isHighlighted && cursor >= $0.start && cursor <= $0.end
        }) {
            keepAsText(phrase)
            return true
        }
        if !visibleSuggestions.isEmpty {
            suggestionsDismissed = true
            return true
        }
        return false
    }

    /// Keeps a recognized phrase as description text, or removes an explicit
    /// trigger token from the input.
    func keepAsText(_ candidate: Candidate) {
        if candidate.isInferred {
            literals.append(TextSpan(start: candidate.start, end: candidate.end))
            reinterpret()
            return
        }
        var scalars = Array(input.unicodeScalars)
        let start = max(0, min(candidate.start, scalars.count)), end = max(start, min(candidate.end, scalars.count))
        scalars.removeSubrange(start..<end)
        if start < scalars.count, scalars[start] == " " {
            scalars.remove(at: start)
        }
        var text = ""
        text.unicodeScalars.append(contentsOf: scalars)
        replaceText(text, cursor: start)
    }

    func submit() async {
        guard !input.trimmingCharacters(in: .whitespaces).isEmpty, !submitting else { return }
        submitting = true
        defer { submitting = false }
        let description = currentInterpretation?.description ?? input
        let known = Set(store.tasksByUUID.keys)
        do {
            let result = try await store.exclusive { [input, literals] in
                try await store.bridge.capture(input, literals: literals)
            }
            if result.added {
                await finish("Added “\(description)”", known: known)
            } else if let review = result.review {
                self.review = ReviewDraft(review)
            }
        } catch {
            self.error = error.localizedDescription
        }
    }

    func submitReview() async {
        guard let review, !submitting else { return }
        submitting = true
        defer { submitting = false }
        let known = Set(store.tasksByUUID.keys)
        do {
            try await store.exclusive { try await store.bridge.captureReview(review.submission) }
            await finish("Added “\(review.values[ReviewDraft.description])”", known: known)
        } catch {
            self.review?.error = error.localizedDescription
        }
    }

    /// Adds the original text using trigger syntax only, ignoring natural
    /// language (the TUI's Ctrl+X in review).
    func submitExplicitOnly() async {
        guard let review, !submitting else { return }
        submitting = true
        defer { submitting = false }
        let known = Set(store.tasksByUUID.keys)
        do {
            try await store.exclusive { try await store.bridge.captureExplicit(review.interpretation.source) }
            await finish("Added “\(review.interpretation.source)”", known: known)
        } catch {
            self.review?.error = error.localizedDescription
        }
    }

    func leaveReview() {
        review = nil
        reinterpret()
    }

    private func finish(_ message: String, known: Set<String>) async {
        store.showCapture = false
        await store.didMutate(message)
        if let added = store.visibleTasks.first(where: { !known.contains($0.uuid) }) {
            store.selection = added.uuid
        }
    }

    private func replaceText(_ text: String, cursor: Int) {
        literals = shiftLiterals(from: input, to: text, literals)
        input = text
        self.cursor = cursor
        textRevision += 1
        reinterpret()
        refreshSuggestions()
    }

    private func reinterpret() {
        interpretRevision += 1
        let revision = interpretRevision, input = input, literals = literals
        guard !input.trimmingCharacters(in: .whitespaces).isEmpty else {
            interpretation = nil
            parseError = nil
            return
        }
        Task {
            do {
                let result = try await store.bridge.interpret(input, literals: literals)
                guard revision == interpretRevision else { return }
                interpretation = result
                parseError = nil
            } catch {
                guard revision == interpretRevision else { return }
                interpretation = nil
                parseError = error.localizedDescription
            }
        }
    }

    private func refreshSuggestions() {
        suggestRevision += 1
        let revision = suggestRevision, input = input, cursor = cursor
        Task {
            let result = (try? await store.bridge.suggest(input, cursor: cursor)) ?? .none
            guard revision == suggestRevision else { return }
            if result.suggestions.map(\.value) != suggestions.suggestions.map(\.value) {
                highlightedSuggestion = 0
            }
            suggestions = result
        }
    }
}

/// The correction review form, in quickadd.Review* field order. Values go
/// back to the bridge unchanged so Go can tell what the user resolved.
struct ReviewDraft: Equatable {
    static let description = 0, due = 1, recurrence = 2, estimate = 3, priority = 4, project = 5, scheduled = 6, tags = 7
    static let labels = ["Description", "Due", "Repeats", "Estimate", "Priority", "Project", "Scheduled", "Tags"]

    let interpretation: Interpretation
    private(set) var values: [String]
    private(set) var touched: [Bool]
    private(set) var decisions: [Bool]
    var error: String?

    init(_ interpretation: Interpretation) {
        self.interpretation = interpretation
        values = interpretation.reviewValues
        touched = Array(repeating: false, count: Self.labels.count)
        decisions = Array(repeating: false, count: Self.labels.count)
    }

    mutating func set(_ index: Int, _ value: String) {
        guard values[index] != value else { return }
        values[index] = value
        touched[index] = true
        error = nil
    }

    /// Explicitly keeps a flagged field as it is (the TUI's Ctrl+R).
    mutating func keep(_ index: Int) {
        decisions[index] = true
        error = nil
    }

    /// Problems recognized for one field, from blocking candidates.
    func problems(for index: Int) -> [String] {
        interpretation.candidates
            .filter { $0.blocking && Self.fieldIndex(for: $0.field) == index }
            .map { $0.diagnostic?.isEmpty == false ? $0.diagnostic! : "“\($0.text)” needs a decision." }
    }

    var submission: ReviewSubmission {
        ReviewSubmission(token: interpretation.token ?? 0, values: values, touched: touched, decisions: decisions)
    }

    /// Mirrors quickadd.ReviewFieldIndexes.
    static func fieldIndex(for field: String) -> Int? {
        switch field {
        case "due", "time": due
        case "recurrence": recurrence
        case "estimate": estimate
        case "priority": priority
        case "project": project
        case "scheduled": scheduled
        case "tag": tags
        default: nil
        }
    }
}
