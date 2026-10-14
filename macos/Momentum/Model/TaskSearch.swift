import Foundation

/// A port of the TUI's local filter (internal/ui/search.go): fuzzy text over
/// description, project and tags, plus an exact `project:<name>` term where
/// `project:none` selects tasks without a project.
enum TaskSearch {
    static func matches(_ task: TaskItem, query: String) -> Bool {
        let (project, text) = parse(query)
        if let project {
            let matchesProject = project == "none"
                ? (task.project ?? "").isEmpty
                : (task.project ?? "").caseInsensitiveCompare(project) == .orderedSame
            if !matchesProject { return false }
        }
        if text.isEmpty { return true }
        let values = [task.description, task.project ?? "", task.tags.joined(separator: " ")]
        // The TUI keeps a task only when its best score is non-negative.
        return values.compactMap { score(text, in: $0) }.contains { $0 >= 0 }
    }

    static func parse(_ query: String) -> (project: String?, text: String) {
        var project: String?
        var terms: [String] = []
        for term in query.lowercased().split(whereSeparator: \.isWhitespace).map(String.init) {
            if project == nil, term.hasPrefix("project:") {
                project = String(term.dropFirst("project:".count))
            } else {
                terms.append(term)
            }
        }
        return (project, terms.joined(separator: " "))
    }

    static func score(_ query: String, in value: String) -> Int? {
        let query = Array(query.lowercased().unicodeScalars)
        let value = Array(value.lowercased().unicodeScalars)
        if query.isEmpty { return 0 }
        if value.starts(with: query) { return 10_000 - value.count }
        var matched = 0, score = 0, last = -2
        for (index, scalar) in value.enumerated() where matched < query.count && scalar == query[matched] {
            score += index == last + 1 ? 35 : 10
            if index == 0 || CharacterSet.whitespaces.contains(value[index - 1]) || "._-".unicodeScalars.contains(value[index - 1]) {
                score += 20
            }
            last = index
            matched += 1
        }
        guard matched == query.count else { return nil }
        return score - (value.count - query.count)
    }
}
