import AppKit
import SwiftUI

struct CaptureView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var model: CaptureModel

    init(store: AppStore) {
        _model = State(initialValue: CaptureModel(store: store))
    }

    var body: some View {
        Group {
            if model.review != nil {
                CaptureReviewView(model: model)
                    .frame(width: 620)
                    .background(Theme.panel)
            } else {
                capture
                    .frame(width: 680)
                    .background(Theme.elevated)
            }
        }
    }

    /// Lines up with the text, past the lead circle.
    private static let inset: CGFloat = 50

    /// Marks, chips, preview and hints show the latest interpretation, even
    /// while the one for the text just typed is on its way, so the sheet
    /// doesn't shrink and grow, or the marks blink, on every keystroke.
    private var capture: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .top, spacing: 12) {
                Circle()
                    .strokeBorder(Theme.text3, lineWidth: 1.4)
                    .frame(width: 20, height: 20)
                    .padding(.top, 7)
                    .accessibilityHidden(true)
                ZStack(alignment: .topLeading) {
                    CaptureTextView(model: model, text: model.input, revision: model.textRevision, interpretation: model.interpretation) {
                        dismiss()
                    }
                    .frame(height: 54)
                    if model.input.isEmpty {
                        Text("Prepare proposal tomorrow at 3pm, p1, every Friday, about 1h #work.client")
                            .font(.system(size: CaptureTextView.font.pointSize))
                            .foregroundStyle(Theme.text3)
                            .lineLimit(1)
                            .padding(.top, 4)
                            .allowsHitTesting(false)
                    }
                }
            }
            .padding(.horizontal, 18)
            .padding(.top, 16)

            VStack(alignment: .leading, spacing: 12) {
                if !model.visibleSuggestions.isEmpty {
                    suggestions
                } else if let interpretation = model.interpretation, !interpretation.candidates.isEmpty {
                    chips(interpretation)
                }
                if let error = model.error ?? model.parseError {
                    Label(error, systemImage: "exclamationmark.triangle.fill")
                        .font(.system(size: 12))
                        .foregroundStyle(Theme.danger)
                        .textSelection(.enabled)
                } else if let interpretation = model.interpretation {
                    preview(interpretation)
                }
            }
            .padding(.leading, Self.inset)
            .padding(.trailing, 18)
            .padding(.bottom, 12)

            Rectangle().fill(Theme.separator).frame(height: 0.5)

            HStack(spacing: 14) {
                hint("↩", model.interpretation?.valid == false ? "Review" : "Add")
                hint("esc", model.interpretation?.candidates.contains { $0.isInferred && $0.isHighlighted } == true ? "Keep phrase as text / Close" : "Close")
                Spacer()
                let triggers = "# project · ! priority · @ due · > scheduled · + tag · ~ estimate · ^ repeat"
                Text(triggers)
                    .lineLimit(1)
                    .truncationMode(.tail)
                    .help("Triggers: \(triggers)")
                if model.submitting {
                    ProgressView().controlSize(.small)
                }
            }
            .font(.system(size: 11.5))
            .foregroundStyle(Theme.text3)
            .padding(.horizontal, 16)
            .padding(.vertical, 9)
        }
    }

    private var suggestions: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(model.visibleSuggestions.prefix(8).enumerated()), id: \.offset) { index, suggestion in
                let selected = index == model.highlightedSuggestion
                HStack(spacing: 10) {
                    Text(suggestion.label ?? suggestion.value)
                        .lineLimit(1)
                    Spacer(minLength: 0)
                    if suggestion.label != nil {
                        Text(suggestion.value)
                            .font(.system(size: 11.5, design: .monospaced))
                            .foregroundStyle(selected ? Theme.accentInk.opacity(0.8) : Theme.text3)
                    }
                }
                .foregroundStyle(selected ? Theme.accentInk : Theme.text)
                .padding(.horizontal, 9)
                .frame(height: 30)
                .background(selected ? Theme.accent : .clear, in: RoundedRectangle(cornerRadius: 6))
                .contentShape(Rectangle())
                .onTapGesture { model.accept(index) }
            }
            HStack(spacing: 4) {
                KeyCap("↑")
                KeyCap("↓")
                Text("select ·")
                KeyCap("⇥")
                Text("accept")
                if let kind = model.suggestions.kind {
                    Text("· \(FieldStyle.label(for: kind))")
                }
            }
            .font(.system(size: 11))
            .foregroundStyle(Theme.text3)
            .padding(.horizontal, 9)
            .padding(.top, 5)
            .padding(.bottom, 3)
            .frame(maxWidth: .infinity, alignment: .leading)
            .overlay(alignment: .top) { Rectangle().fill(Theme.separator).frame(height: 0.5) }
            .padding(.top, 3)
        }
        .padding(4)
        .frame(maxWidth: 420)
        .background(Theme.elevated, in: RoundedRectangle(cornerRadius: 9))
        .overlay(RoundedRectangle(cornerRadius: 9).strokeBorder(Theme.separatorStrong, lineWidth: 0.5))
        .shadow(color: .black.opacity(0.14), radius: 12, y: 8)
    }

    private func chips(_ interpretation: Interpretation) -> some View {
        FlowLayout(spacing: 6) {
            ForEach(interpretation.candidates.filter(\.isHighlighted), id: \.self) { candidate in
                HStack(spacing: 5) {
                    Text(FieldStyle.label(for: candidate.field))
                        .foregroundStyle(candidate.blocking ? Theme.danger : Theme.text3)
                    Text(candidate.field == "tag" ? "+\(candidate.value)" : candidate.text)
                        .foregroundStyle(Theme.text)
                    Button {
                        model.keepAsText(candidate)
                    } label: {
                        Image(systemName: "xmark")
                            .font(.system(size: 9, weight: .semibold))
                            .frame(width: 18, height: 18)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .foregroundStyle(Theme.text3)
                    .help(candidate.isInferred ? "Keep “\(candidate.text)” as text" : "Remove \(candidate.text)")
                }
                .font(.system(size: 12))
                .padding(.leading, 8)
                .padding(.trailing, 4)
                .frame(height: 24)
                .background(candidate.blocking ? Theme.dangerSoft : Theme.sunken, in: RoundedRectangle(cornerRadius: 6))
                .overlay(
                    RoundedRectangle(cornerRadius: 6)
                        .strokeBorder(candidate.blocking || candidate.conflict ? Theme.danger : Theme.separator, lineWidth: 0.5)
                )
                .help(candidate.diagnostic ?? (candidate.isInferred ? "Recognized from “\(candidate.text)”" : candidate.text))
            }
        }
    }

    private func preview(_ interpretation: Interpretation) -> some View {
        HStack(spacing: 6) {
            Text("Saves as").foregroundStyle(Theme.text3)
            Text("“\(interpretation.description.isEmpty ? "…" : interpretation.description)”")
                .foregroundStyle(Theme.text2)
                .lineLimit(1)
            if !interpretation.valid {
                Text("· needs review").foregroundStyle(Theme.danger)
            }
        }
        .font(.system(size: 12))
    }

    private func hint(_ key: String, _ label: String) -> some View {
        HStack(spacing: 5) {
            KeyCap(key)
            Text(label)
        }
    }
}

/// Single-purpose text view for capture: colors recognized metadata and
/// routes Return, Tab, arrows and Esc to the capture model.
struct CaptureTextView: NSViewRepresentable {
    let model: CaptureModel
    let text: String
    let revision: Int
    let interpretation: Interpretation?
    let close: () -> Void

    static let font = NSFont.systemFont(ofSize: 19)

    func makeCoordinator() -> Coordinator {
        Coordinator(model: model, close: close)
    }

    func makeNSView(context: Context) -> NSScrollView {
        let scroll = NSTextView.scrollableTextView()
        scroll.drawsBackground = false
        scroll.hasVerticalScroller = false
        let textView = scroll.documentView as! NSTextView
        textView.delegate = context.coordinator
        textView.isRichText = false
        textView.allowsUndo = true
        textView.drawsBackground = false
        textView.font = Self.font
        textView.insertionPointColor = NSColor(Theme.accent)
        textView.textContainerInset = NSSize(width: 0, height: 4)
        textView.textContainer?.lineFragmentPadding = 0
        textView.isAutomaticQuoteSubstitutionEnabled = false
        textView.isAutomaticDashSubstitutionEnabled = false
        textView.isAutomaticTextReplacementEnabled = false
        textView.isAutomaticSpellingCorrectionEnabled = false
        textView.isContinuousSpellCheckingEnabled = false
        textView.setAccessibilityLabel("New task")
        DispatchQueue.main.async {
            textView.window?.makeFirstResponder(textView)
        }
        return scroll
    }

    func updateNSView(_ scroll: NSScrollView, context: Context) {
        guard let textView = scroll.documentView as? NSTextView else { return }
        context.coordinator.model = model
        context.coordinator.close = close
        if context.coordinator.revision != revision || (textView.string.isEmpty && !text.isEmpty) {
            context.coordinator.revision = revision
            if textView.string != text {
                textView.string = text
                textView.setSelectedRange(NSRange(location: text.utf16Offset(ofScalar: model.cursor), length: 0))
                // The new text takes its first character's marks; start it plain.
                context.coordinator.highlight(textView, nil)
            }
        }
        context.coordinator.highlight(textView, interpretation)
    }

    @MainActor
    final class Coordinator: NSObject, NSTextViewDelegate {
        var model: CaptureModel
        var close: () -> Void
        var revision = 0

        init(model: CaptureModel, close: @escaping () -> Void) {
            self.model = model
            self.close = close
        }

        func textDidChange(_ notification: Notification) {
            guard let textView = notification.object as? NSTextView else { return }
            model.textChanged(textView.string, cursor: cursor(in: textView))
        }

        func textViewDidChangeSelection(_ notification: Notification) {
            guard let textView = notification.object as? NSTextView, textView.string == model.input else { return }
            model.cursorMoved(cursor(in: textView))
        }

        func textView(_ textView: NSTextView, doCommandBy selector: Selector) -> Bool {
            let suggesting = !model.visibleSuggestions.isEmpty
            switch selector {
            case #selector(NSResponder.insertNewline(_:)), #selector(NSResponder.insertLineBreak(_:)), #selector(NSResponder.insertNewlineIgnoringFieldEditor(_:)):
                // Enter adds the whole task; it never accepts a suggestion.
                Task { await model.submit() }
                return true
            case #selector(NSResponder.insertTab(_:)):
                if suggesting { model.accept() }
                return true
            case #selector(NSResponder.insertBacktab(_:)):
                return true
            case #selector(NSResponder.moveUp(_:)) where suggesting:
                model.moveHighlight(-1)
                return true
            case #selector(NSResponder.moveDown(_:)) where suggesting:
                model.moveHighlight(1)
                return true
            case #selector(NSResponder.cancelOperation(_:)):
                if !model.escape() { close() }
                return true
            default:
                return false
            }
        }

        func highlight(_ textView: NSTextView, _ interpretation: Interpretation?) {
            guard !textView.hasMarkedText(), let storage = textView.textStorage else { return }
            // While the interpretation for the text just typed is on its way,
            // the marks stay: they move with the text as it is edited.
            if let interpretation, interpretation.source != textView.string { return }
            let base: [NSAttributedString.Key: Any] = [.font: CaptureTextView.font, .foregroundColor: NSColor(Theme.text)]
            storage.beginEditing()
            storage.setAttributes(base, range: NSRange(location: 0, length: storage.length))
            if let interpretation {
                for candidate in interpretation.candidates where candidate.isHighlighted {
                    let range = textView.string.nsRange(ofScalars: candidate.start, candidate.end)
                    let flagged = candidate.blocking || candidate.conflict
                    storage.addAttribute(.backgroundColor, value: NSColor(flagged ? Theme.markError : FieldStyle.mark(for: candidate.field)), range: range)
                    if candidate.blocking {
                        storage.addAttributes([
                            .underlineStyle: NSUnderlineStyle.thick.rawValue,
                            .underlineColor: NSColor(Theme.danger),
                        ], range: range)
                    }
                }
            }
            storage.endEditing()
            textView.typingAttributes = base
        }

        private func cursor(in textView: NSTextView) -> Int {
            textView.string.scalarOffset(ofUTF16: textView.selectedRange().location)
        }
    }
}

/// Correction review: shown when Momentum can't add a capture unambiguously.
/// Values are exactly what the server receives.
struct CaptureReviewView: View {
    @Bindable var model: CaptureModel
    @Environment(\.dismiss) private var dismiss

    /// The design's field order; values keep quickadd's order.
    private static let order = [
        ReviewDraft.description, ReviewDraft.project, ReviewDraft.priority, ReviewDraft.due,
        ReviewDraft.scheduled, ReviewDraft.recurrence, ReviewDraft.estimate, ReviewDraft.tags,
    ]
    private static let prompts = [
        ReviewDraft.project: "none", ReviewDraft.priority: "none", ReviewDraft.due: "none", ReviewDraft.scheduled: "none",
        ReviewDraft.recurrence: "does not repeat", ReviewDraft.estimate: "e.g. 45m", ReviewDraft.tags: "space separated",
    ]

    var body: some View {
        if let review = model.review {
            VStack(alignment: .leading, spacing: 0) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Review before adding").font(.system(size: 14, weight: .semibold))
                    Text("Momentum couldn’t add this unambiguously. Correct the highlighted fields, or keep a field as it is.")
                        .font(.system(size: 12.5))
                        .foregroundStyle(Theme.text2)
                }
                .padding(.horizontal, 20)
                .padding(.top, 16)
                .padding(.bottom, 10)
                Text(sourceMarks(review.interpretation))
                    .font(.system(size: 12, design: .monospaced))
                    .lineSpacing(6)
                    .padding(.horizontal, 11)
                    .padding(.vertical, 9)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Theme.sunken, in: RoundedRectangle(cornerRadius: 8))
                    .padding(.horizontal, 20)
                    .accessibilityLabel("Your input: \(review.interpretation.source)")
                let diagnostics = review.interpretation.diagnostics.filter(\.blocking)
                if !diagnostics.isEmpty {
                    VStack(alignment: .leading, spacing: 4) {
                        ForEach(diagnostics, id: \.self) { diagnostic in
                            HStack(alignment: .firstTextBaseline, spacing: 8) {
                                Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Theme.danger)
                                Text(diagnostic.message)
                            }
                        }
                    }
                    .font(.system(size: 12.5))
                    .padding(.horizontal, 12)
                    .padding(.vertical, 9)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Theme.dangerSoft, in: RoundedRectangle(cornerRadius: 8))
                    .padding(.horizontal, 20)
                    .padding(.top, 10)
                }
                VStack(alignment: .leading, spacing: 9) {
                    ForEach(Self.order, id: \.self) { index in
                        field(index, review)
                    }
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 14)
                if let error = review.error {
                    Label(error, systemImage: "exclamationmark.triangle.fill")
                        .font(.system(size: 12))
                        .foregroundStyle(Theme.danger)
                        .textSelection(.enabled)
                        .padding(.horizontal, 20)
                        .padding(.bottom, 12)
                }
                HStack(spacing: 8) {
                    Button("Edit Text", systemImage: "chevron.left") { model.leaveReview() }
                        .buttonStyle(.borderless)
                        .foregroundStyle(Theme.text2)
                    Button("Add Using Triggers Only") { Task { await model.submitExplicitOnly() } }
                        .help("Ignore natural language and add the text with # ! @ > + ~ ^ triggers only")
                    Spacer()
                    Button("Cancel", role: .cancel) { dismiss() }
                        .keyboardShortcut(.cancelAction)
                    Button("Add Task") { Task { await model.submitReview() } }
                        .keyboardShortcut(.defaultAction)
                        .tint(Theme.tint)
                        .disabled(model.submitting)
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 12)
                .overlay(alignment: .top) { Rectangle().fill(Theme.separator).frame(height: 0.5) }
            }
        }
    }

    /// Labels sit right-aligned in a column; hints and problems line up
    /// under the field.
    private static let labelWidth: CGFloat = 110, labelSpacing: CGFloat = 14

    private func field(_ index: Int, _ review: ReviewDraft) -> some View {
        let problems = review.problems(for: index)
        let monospaced = index == ReviewDraft.project || index == ReviewDraft.recurrence
        return VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline, spacing: Self.labelSpacing) {
                Text(ReviewDraft.labels[index])
                    .font(.system(size: 12.5))
                    .foregroundStyle(Theme.text2)
                    .frame(width: Self.labelWidth, alignment: .trailing)
                TextField(ReviewDraft.labels[index], text: Binding(
                    get: { model.review?.values[index] ?? "" },
                    set: { model.review?.set(index, $0) }
                ), prompt: Self.prompts[index].map { Text($0) })
                .labelsHidden()
                .textFieldStyle(.roundedBorder)
                .font(monospaced ? .system(size: 12, design: .monospaced) : nil)
            }
            Group {
                if index == ReviewDraft.due, let date = Format.reviewDate(review.values[index]) {
                    Text(date).font(.system(size: 11.5)).foregroundStyle(Theme.text3)
                }
                if !problems.isEmpty {
                    HStack(alignment: .firstTextBaseline) {
                        Text(problems.joined(separator: " "))
                            .font(.system(size: 11.5))
                            .foregroundStyle(Theme.danger)
                            .fixedSize(horizontal: false, vertical: true)
                        Spacer()
                        if review.decisions[index] {
                            Label("Kept", systemImage: "checkmark").font(.system(size: 11.5)).foregroundStyle(Theme.text3)
                        } else {
                            Button("Keep as Is") { model.review?.keep(index) }
                                .controlSize(.small)
                        }
                    }
                }
            }
            .padding(.leading, Self.labelWidth + Self.labelSpacing)
        }
    }

    private func sourceMarks(_ interpretation: Interpretation) -> AttributedString {
        var text = AttributedString(interpretation.source)
        let source = interpretation.source
        for candidate in interpretation.candidates where candidate.isHighlighted {
            let range = source.nsRange(ofScalars: candidate.start, candidate.end)
            guard let bounds = Range(range, in: source), let lower = AttributedString.Index(bounds.lowerBound, within: text),
                  let upper = AttributedString.Index(bounds.upperBound, within: text) else { continue }
            text[lower..<upper].backgroundColor = candidate.blocking || candidate.conflict ? Theme.markError : FieldStyle.mark(for: candidate.field)
        }
        return text
    }
}

/// Wraps chips onto as many lines as they need.
struct FlowLayout: Layout {
    var spacing: CGFloat = 6

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let rows = arrange(subviews, width: proposal.width ?? .infinity)
        return CGSize(width: proposal.width ?? rows.map(\.width).max() ?? 0, height: rows.last.map { $0.y + $0.height } ?? 0)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        for row in arrange(subviews, width: bounds.width) {
            var x = bounds.minX
            for index in row.indices {
                let size = subviews[index].sizeThatFits(.unspecified)
                subviews[index].place(at: CGPoint(x: x, y: bounds.minY + row.y), proposal: ProposedViewSize(size))
                x += size.width + spacing
            }
        }
    }

    private struct Row { var indices: [Int] = []; var y: CGFloat; var width: CGFloat = 0; var height: CGFloat = 0 }

    private func arrange(_ subviews: Subviews, width: CGFloat) -> [Row] {
        var rows = [Row(y: 0)]
        for index in subviews.indices {
            let size = subviews[index].sizeThatFits(.unspecified)
            if !rows[rows.count - 1].indices.isEmpty, rows[rows.count - 1].width + spacing + size.width > width {
                let last = rows[rows.count - 1]
                rows.append(Row(y: last.y + last.height + spacing))
            }
            let gap = rows[rows.count - 1].indices.isEmpty ? 0 : spacing
            rows[rows.count - 1].indices.append(index)
            rows[rows.count - 1].width += gap + size.width
            rows[rows.count - 1].height = max(rows[rows.count - 1].height, size.height)
        }
        return rows
    }
}
