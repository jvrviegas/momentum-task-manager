import Foundation

// The bridge counts text offsets in Unicode scalars (Go runes); AppKit counts
// UTF-16 code units. These helpers convert between the two.
extension String {
    func utf16Offset(ofScalar offset: Int) -> Int {
        let scalars = unicodeScalars
        let index = scalars.index(scalars.startIndex, offsetBy: max(0, min(offset, scalars.count)))
        return utf16.distance(from: utf16.startIndex, to: index)
    }

    func scalarOffset(ofUTF16 offset: Int) -> Int {
        let clamped = max(0, min(offset, utf16.count))
        var index = utf16.index(utf16.startIndex, offsetBy: clamped)
        // An offset inside a surrogate pair belongs to the scalar it starts.
        while index.samePosition(in: unicodeScalars) == nil, index > utf16.startIndex {
            index = utf16.index(before: index)
        }
        return unicodeScalars.distance(from: unicodeScalars.startIndex, to: index)
    }

    func nsRange(ofScalars start: Int, _ end: Int) -> NSRange {
        let lower = utf16Offset(ofScalar: start)
        return NSRange(location: lower, length: max(0, utf16Offset(ofScalar: end) - lower))
    }
}

/// Moves phrases the user kept as text across an edit, like the TUI's
/// shiftLiterals: spans before or after the edit survive, spans it touched
/// become recognizable again.
func shiftLiterals(from before: String, to after: String, _ literals: [TextSpan]) -> [TextSpan] {
    guard !literals.isEmpty else { return [] }
    let old = Array(before.unicodeScalars), new = Array(after.unicodeScalars)
    var prefix = 0
    while prefix < old.count, prefix < new.count, old[prefix] == new[prefix] { prefix += 1 }
    var suffix = 0
    while suffix < old.count - prefix, suffix < new.count - prefix, old[old.count - 1 - suffix] == new[new.count - 1 - suffix] {
        suffix += 1
    }
    let editEnd = old.count - suffix, delta = new.count - old.count
    return literals.compactMap { span in
        if span.end <= prefix { return span }
        if span.start >= editEnd { return TextSpan(start: span.start + delta, end: span.end + delta) }
        return nil
    }
}
