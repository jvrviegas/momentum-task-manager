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
            } else {
                capture
            }
        }
        .frame(width: 640)
    }

    private var capture: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: "plus.circle.fill")
                    .font(.title2)
                    .foregroundStyle(.tint)
                    .padding(.top, 4)
                ZStack(alignment: .topLeading) {
                    CaptureTextView(model: model, text: model.input, revision: model.textRevision, interpretation: model.currentInterpretation) {
                        dismiss()
                    }
                    .frame(height: 58)
                    if model.input.isEmpty {
                        Text("Prepare proposal tomorrow at 3pm, p1, every Friday, about 1h #work.client")
                            .font(.title3)
                            .foregroundStyle(.tertiary)
                            .padding(.top, 4)
                            .allowsHitTesting(false)
                    }
                }
            }
            .padding(16)

            Divider()

            VStack(alignment: .leading, spacing: 10) {
                if !model.visibleSuggestions.isEmpty {
                    suggestions
                } else if let interpretation = model.currentInterpretation, !interpretation.candidates.isEmpty {
                    chips(interpretation)
                } else {
                    Text("# project · ! priority · @ due · > scheduled · + tag · ~ estimate · ^ repeat")
                        .font(.callout.monospaced())
                        .foregroundStyle(.secondary)
                }
                if let error = model.error ?? model.parseError {
                    Label(error, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.red)
                        .textSelection(.enabled)
                } else if let interpretation = model.currentInterpretation {
                    preview(interpretation)
                }
            }
            .padding(16)

            Divider()

            HStack(spacing: 14) {
                hint("↩", model.currentInterpretation?.valid == false ? "Review" : "Add")
                hint("⇥", "Accept suggestion")
                hint("esc", model.currentInterpretation?.candidates.contains { $0.isInferred && $0.isHighlighted } == true ? "Keep phrase as text / Close" : "Close")
                Spacer()
                if let context = model.store.snapshot?.context {
                    Chip(text: "Context \(context)")
                        .help("Taskwarrior applies the active context to new tasks")
                }
                if model.submitting {
                    ProgressView().controlSize(.small)
                }
            }
            .font(.caption)
            .foregroundStyle(.secondary)
            .padding(.horizontal, 16)
            .padding(.vertical, 10)
        }
    }

    private var suggestions: some View {
        VStack(alignment: .leading, spacing: 2) {
            ForEach(Array(model.visibleSuggestions.prefix(8).enumerated()), id: \.offset) { index, suggestion in
                HStack {
                    Text(suggestion.label ?? suggestion.value)
                    if suggestion.label != nil {
                        Text(suggestion.value).font(.callout.monospaced()).foregroundStyle(.secondary)
                    }
                    Spacer()
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .background(index == model.highlightedSuggestion ? AnyShapeStyle(.selection) : AnyShapeStyle(.clear), in: RoundedRectangle(cornerRadius: 5))
                .contentShape(Rectangle())
                .onTapGesture { model.accept(index) }
            }
        }
    }

    private func chips(_ interpretation: Interpretation) -> some View {
        FlowLayout(spacing: 6) {
            ForEach(interpretation.candidates.filter(\.isHighlighted), id: \.self) { candidate in
                HStack(spacing: 4) {
                    Text(FieldStyle.label(for: candidate.field)).foregroundStyle(.secondary)
                    Text(candidate.field == "tag" ? "+\(candidate.value)" : candidate.text)
                    Button {
                        model.keepAsText(candidate)
                    } label: {
                        Image(systemName: "xmark")
                    }
                    .buttonStyle(.borderless)
                    .help(candidate.isInferred ? "Keep “\(candidate.text)” as text" : "Remove \(candidate.text)")
                }
                .font(.callout)
                .padding(.horizontal, 8)
                .padding(.vertical, 3)
                .background((candidate.blocking ? Color.red : FieldStyle.color(for: candidate.field)).opacity(0.15), in: Capsule())
                .help(candidate.diagnostic ?? (candidate.isInferred ? "Recognized from “\(candidate.text)”" : candidate.text))
            }
        }
    }

    private func preview(_ interpretation: Interpretation) -> some View {
        HStack(spacing: 6) {
            Text("Saves as").foregroundStyle(.secondary)
            Text("“\(interpretation.description.isEmpty ? "…" : interpretation.description)”").bold().lineLimit(1)
            if !interpretation.valid {
                Text("· needs review").foregroundStyle(.red)
            }
        }
        .font(.callout)
    }

    private func hint(_ key: String, _ label: String) -> some View {
        HStack(spacing: 4) {
            Text(key).font(.caption.monospaced()).padding(.horizontal, 4).background(.quaternary, in: RoundedRectangle(cornerRadius: 3))
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

    static let font = NSFont.systemFont(ofSize: NSFont.preferredFont(forTextStyle: .title3).pointSize)

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
            let base: [NSAttributedString.Key: Any] = [.font: CaptureTextView.font, .foregroundColor: NSColor.textColor]
            storage.beginEditing()
            storage.setAttributes(base, range: NSRange(location: 0, length: storage.length))
            if let interpretation, interpretation.source == textView.string {
                for candidate in interpretation.candidates where candidate.isHighlighted {
                    let color = NSColor(candidate.blocking || candidate.conflict ? .red : FieldStyle.color(for: candidate.field))
                    storage.addAttributes([
                        .foregroundColor: color,
                        .underlineStyle: NSUnderlineStyle.single.rawValue,
                        .underlineColor: color,
                    ], range: textView.string.nsRange(ofScalars: candidate.start, candidate.end))
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
/// Values are exactly what Taskwarrior receives.
struct CaptureReviewView: View {
    @Bindable var model: CaptureModel
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        if let review = model.review {
            VStack(alignment: .leading, spacing: 14) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Review before adding").font(.title3.bold())
                    Text("Momentum couldn’t add this unambiguously. Correct the highlighted fields, or keep a field as it is.")
                        .foregroundStyle(.secondary)
                }
                Text(sourceMarks(review.interpretation))
                    .padding(10)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(.quaternary.opacity(0.5), in: RoundedRectangle(cornerRadius: 6))
                ForEach(review.interpretation.diagnostics.filter(\.blocking), id: \.self) { diagnostic in
                    Label(diagnostic.message, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.red)
                }
                Form {
                    ForEach(ReviewDraft.labels.indices, id: \.self) { index in
                        field(index, review)
                    }
                }
                .formStyle(.grouped)
                .scrollDisabled(true)
                if let error = review.error {
                    Label(error, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.red).textSelection(.enabled)
                }
                HStack {
                    Button("Edit Text", systemImage: "chevron.left") { model.leaveReview() }
                    Button("Add Using Triggers Only") { Task { await model.submitExplicitOnly() } }
                        .help("Ignore natural language and add the text with # ! @ > + ~ ^ triggers only")
                    Spacer()
                    Button("Cancel", role: .cancel) { dismiss() }
                        .keyboardShortcut(.cancelAction)
                    Button("Add Task") { Task { await model.submitReview() } }
                        .keyboardShortcut(.defaultAction)
                        .disabled(model.submitting)
                }
            }
            .padding(20)
        }
    }

    private func field(_ index: Int, _ review: ReviewDraft) -> some View {
        let problems = review.problems(for: index)
        return VStack(alignment: .leading, spacing: 4) {
            TextField(ReviewDraft.labels[index], text: Binding(
                get: { model.review?.values[index] ?? "" },
                set: { model.review?.set(index, $0) }
            ))
            if index == ReviewDraft.due, let date = Format.reviewDate(review.values[index]) {
                Text(date).font(.caption).foregroundStyle(.secondary)
            }
            if !problems.isEmpty {
                HStack(alignment: .firstTextBaseline) {
                    Text(problems.joined(separator: " ")).font(.caption).foregroundStyle(.red)
                    Spacer()
                    if review.decisions[index] {
                        Label("Kept", systemImage: "checkmark").font(.caption).foregroundStyle(.secondary)
                    } else {
                        Button("Keep as Is") { model.review?.keep(index) }
                            .controlSize(.small)
                    }
                }
            }
        }
    }

    private func sourceMarks(_ interpretation: Interpretation) -> AttributedString {
        var text = AttributedString(interpretation.source)
        let source = interpretation.source
        for candidate in interpretation.candidates where candidate.isHighlighted {
            let range = source.nsRange(ofScalars: candidate.start, candidate.end)
            guard let bounds = Range(range, in: source), let lower = AttributedString.Index(bounds.lowerBound, within: text),
                  let upper = AttributedString.Index(bounds.upperBound, within: text) else { continue }
            text[lower..<upper].foregroundColor = candidate.blocking || candidate.conflict ? .red : FieldStyle.color(for: candidate.field)
            text[lower..<upper].underlineStyle = .single
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
