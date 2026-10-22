import SwiftUI

/// Where the keyboard is inside Plan Today.
enum PlanFocus: Hashable {
    case list, filter
}

/// Plan Today: tick the day's tasks against a capacity column built from
/// their estimates. Confirming writes only the day's plan tag.
struct PlanTodayView: View {
    @Environment(AppStore.self) private var store
    @FocusState private var focus: PlanFocus?
    @State private var stacked = false

    var body: some View {
        if let sheet = store.planSheet {
            VStack(spacing: 0) {
                header(sheet)
                if let notice = sheet.notice {
                    NoticeBanner(notice: notice)
                }
                Group {
                    if stacked {
                        VStack(spacing: 0) {
                            list(sheet)
                            Rectangle().fill(Theme.separator).frame(height: 0.5)
                            CapacityPanel(sheet: sheet).frame(maxHeight: 260)
                        }
                    } else {
                        HStack(spacing: 0) {
                            list(sheet)
                            Rectangle().fill(Theme.separator).frame(width: 0.5)
                            CapacityPanel(sheet: sheet).frame(width: 320)
                        }
                    }
                }
                .overlay(alignment: .top) {
                    Rectangle().fill(Theme.separator).frame(height: 0.5)
                }
            }
            .background(Theme.content)
            .onGeometryChange(for: Bool.self) { $0.size.width < 900 } action: { stacked = $0 }
            .onAppear { focus = .list }
        }
    }

    private func header(_ sheet: PlanSheet) -> some View {
        HStack(alignment: .bottom, spacing: 14) {
            VStack(alignment: .leading, spacing: 3) {
                Text("Plan Today")
                    .font(.system(size: 17, weight: .semibold))
                Text(Format.planDay(sheet.plan.date)?.formatted(.dateTime.weekday(.wide).month(.wide).day().year()) ?? sheet.plan.date)
                    .font(.system(size: 12))
                    .foregroundStyle(Theme.text3)
            }
            Spacer(minLength: 8)
            Text("Writes \(Text("+\(sheet.plan.tag)").monospaced()); never changes dates")
                .font(.system(size: 12))
                .foregroundStyle(Theme.text3)
                .multilineTextAlignment(.trailing)
            Button("Cancel") { store.cancelPlan() }
            Button {
                Task { await store.confirmPlan() }
            } label: {
                HStack(spacing: 6) {
                    Text(sheet.saving ? "Saving…" : "Confirm Plan")
                    Text("↩").opacity(0.72)
                }
            }
            .buttonStyle(.borderedProminent)
            .tint(Theme.tint)
            .disabled(sheet.saving)
        }
        .padding(.horizontal, 22)
        .padding(.top, 18)
        .padding(.bottom, 14)
    }

    private func list(_ sheet: PlanSheet) -> some View {
        ScrollViewReader { proxy in
            ScrollView {
                // Not lazy, so the filter below the fold can take focus.
                VStack(alignment: .leading, spacing: 0) {
                    if !sheet.plan.obligations.isEmpty {
                        groupHead("Obligations", tag: "Fixed", why: "Overdue, due today or scheduled today — already committed.", items: sheet.plan.obligations)
                        rows(sheet.plan.obligations, in: sheet)
                    }
                    if !sheet.plan.rollover.isEmpty {
                        groupHead("Rollover", why: "Planned on an earlier day and still open. Keep or let go.", items: sheet.plan.rollover)
                        rows(sheet.plan.rollover, in: sheet)
                    }
                    groupHead("Candidates", why: "Other pending work you could take on today.", items: sheet.plan.candidates)
                    if !sheet.plan.candidates.isEmpty {
                        filterField(sheet)
                    }
                    rows(sheet.visibleCandidates, in: sheet)
                    if sheet.visibleCandidates.isEmpty {
                        Text(sheet.plan.candidates.isEmpty ? "No other pending tasks" : "No candidates match")
                            .font(.system(size: 12))
                            .foregroundStyle(Theme.text3)
                            .padding(.horizontal, 22)
                            .padding(.vertical, 8)
                    }
                }
                .padding(.top, 4)
                .padding(.bottom, 24)
            }
            .focusable()
            .focused($focus, equals: .list)
            .focusEffectDisabled()
            .onKeyPress(phases: .down) { press in
                // The filter field is inside the list; its keys are text.
                guard focus == .list else { return .ignored }
                return PlanKeyCommands.handle(
                    key: press.key, characters: press.characters, modifiers: press.modifiers, store: store,
                    focusFilter: { focus = .filter })
            }
            .onChange(of: sheet.cursor) { _, cursor in
                if let cursor { proxy.scrollTo(cursor) }
            }
            .onChange(of: focus) { _, focus in
                if focus == .filter { proxy.scrollTo(PlanFocus.filter, anchor: .center) }
            }
        }
    }

    private func groupHead(_ title: String, tag: String? = nil, why: String, items: [PlanItem]) -> some View {
        let minutes = items.compactMap(\.task.estimateMinutes).reduce(0, +)
        return HStack(spacing: 8) {
            Text(title)
                .font(.system(size: 12.5, weight: .semibold))
                .foregroundStyle(Theme.text)
            if let tag {
                Text(tag.uppercased())
                    .font(.system(size: 10.5, weight: .semibold))
                    .tracking(0.2)
                    .foregroundStyle(Theme.text3)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 1)
                    .overlay(RoundedRectangle(cornerRadius: 4).strokeBorder(Theme.separatorStrong, lineWidth: 0.5))
            }
            Text(why)
                .foregroundStyle(Theme.text3)
                .lineLimit(1)
            Spacer(minLength: 8)
            Text("\(items.count) · \(Format.minutes(minutes))")
                .foregroundStyle(Theme.text3)
                .monospacedDigit()
        }
        .font(.system(size: 11.5))
        .padding(.horizontal, 22)
        .padding(.top, 14)
        .padding(.bottom, 6)
    }

    private func rows(_ items: [PlanItem], in sheet: PlanSheet) -> some View {
        ForEach(items) { item in
            PlanRow(
                item: item, fixed: sheet.isFixed(item), selected: sheet.isSelected(item),
                focused: focus == .list && sheet.cursor == item.id, now: store.now
            ) {
                store.planSheet?.point(at: item)
                store.planSheet?.toggle(item)
                focus = .list
            }
            .id(item.id)
        }
    }

    private func filterField(_ sheet: PlanSheet) -> some View {
        HStack(spacing: 6) {
            Image(systemName: "magnifyingglass")
                .font(.system(size: 11))
                .foregroundStyle(Theme.text3)
            TextField("Filter candidates", text: Binding(get: { store.planSheet?.filter ?? "" }, set: { store.planSheet?.filter = $0 }))
                .textFieldStyle(.plain)
                .focused($focus, equals: .filter)
                // ↩ keeps the filter and Esc clears it; either way the list
                // takes the keys back, as the main search does.
                .onSubmit { focus = .list }
                .onKeyPress(.escape) {
                    store.planSheet?.filter = ""
                    focus = .list
                    return .handled
                }
            Text("\(sheet.visibleCandidates.count)/\(sheet.plan.candidates.count)")
                .font(.system(size: 11))
                .foregroundStyle(Theme.text3)
                .monospacedDigit()
        }
        .font(.system(size: 12.5))
        .padding(.horizontal, 9)
        .frame(height: 28)
        .background(Theme.sunken, in: RoundedRectangle(cornerRadius: 7))
        .overlay(RoundedRectangle(cornerRadius: 7).strokeBorder(focus == .filter ? Theme.accent : Theme.separator, lineWidth: focus == .filter ? 1.5 : 0.5))
        .padding(.horizontal, 22)
        .padding(.top, 2)
        .padding(.bottom, 6)
        .id(PlanFocus.filter)
    }
}

/// Plan Today's keys on the sheet's list (D5). The filter field handles its
/// own ↩ and Esc.
enum PlanKeyCommands {
    /// Handles one key press. It takes the press's parts rather than a
    /// KeyPress, which tests cannot make.
    @MainActor
    static func handle(
        key: KeyEquivalent, characters: String, modifiers: EventModifiers, store: AppStore, focusFilter: () -> Void
    ) -> KeyPress.Result {
        guard store.planSheet != nil else { return .ignored }
        if modifiers.subtracting(.numericPad) == .command, characters == "f" {
            focusFilter()
            return .handled
        }
        guard modifiers.subtracting([.shift, .numericPad, .function]).isEmpty else { return .ignored }
        switch key {
        case .upArrow: store.planSheet?.move(by: -1)
        case .downArrow: store.planSheet?.move(by: 1)
        case .space: store.planSheet?.toggleCursor()
        case .return: Task { await store.confirmPlan() }
        case .escape: store.cancelPlan()
        default:
            switch characters {
            case "j": store.planSheet?.move(by: 1)
            case "k": store.planSheet?.move(by: -1)
            case "g": store.planSheet?.moveToFirst()
            case "G": store.planSheet?.moveToLast()
            case "/": focusFilter()
            default: return .ignored
            }
        }
        return .handled
    }
}

private struct PlanRow: View {
    let item: PlanItem
    let fixed: Bool
    let selected: Bool
    let focused: Bool
    let now: Date
    let toggle: () -> Void
    @State private var hovering = false

    var body: some View {
        HStack(spacing: 10) {
            mark
            VStack(alignment: .leading, spacing: 1) {
                Text(item.task.description)
                    .foregroundStyle(Theme.text)
                    .lineLimit(1)
                    .truncationMode(.tail)
                details
            }
            Spacer(minLength: 8)
            if let estimate = item.task.estimate {
                Text(estimate)
                    .font(.system(size: 12))
                    .foregroundStyle(Theme.text2)
                    .monospacedDigit()
                    .frame(minWidth: 52, alignment: .trailing)
            } else {
                Text("unknown")
                    .font(.system(size: 12))
                    .italic()
                    .foregroundStyle(Theme.text3)
                    .frame(minWidth: 52, alignment: .trailing)
                    .help("No estimate; not counted in the capacity")
            }
        }
        .font(.system(size: 13))
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .frame(minHeight: 36)
        .background(background, in: RoundedRectangle(cornerRadius: 8))
        .overlay {
            if focused {
                RoundedRectangle(cornerRadius: 8).strokeBorder(Theme.accent, lineWidth: 1.5)
            }
        }
        .contentShape(Rectangle())
        .onHover { hovering = $0 && !fixed }
        .onTapGesture { if !fixed { toggle() } }
        .padding(.horizontal, 12)
        .accessibilityElement(children: .combine)
        .accessibilityLabel(fixed ? "Fixed obligation “\(item.task.description)”" : "Plan “\(item.task.description)” for today")
        .accessibilityValue(fixed ? "" : selected ? "selected" : "not selected")
        .accessibilityAddTraits(fixed ? [] : .isToggle)
        .accessibilityAction { if !fixed { toggle() } }
    }

    private var background: Color {
        if selected && !fixed { return Theme.accentSofter }
        return hovering ? Theme.hover : .clear
    }

    @ViewBuilder
    private var mark: some View {
        if fixed {
            Image(systemName: "lock")
                .font(.system(size: 11))
                .foregroundStyle(Theme.text3)
                .frame(width: 18, height: 18)
                .help("Fixed obligation")
        } else {
            ZStack {
                RoundedRectangle(cornerRadius: 5)
                    .fill(selected ? Theme.accent : Theme.elevated)
                RoundedRectangle(cornerRadius: 5)
                    .strokeBorder(selected ? Theme.accent : Theme.text3, lineWidth: 1.3)
                if selected {
                    Image(systemName: "checkmark")
                        .font(.system(size: 9, weight: .bold))
                        .foregroundStyle(Theme.accentInk)
                }
            }
            .frame(width: 17, height: 17)
        }
    }

    private var details: some View {
        HStack(spacing: 8) {
            if let reason = Format.planReason(item, now: now) {
                Text(reason.text)
                    .foregroundStyle(reason.tone == .overdue ? Theme.danger : Theme.text2)
                    .help(reason.help)
            }
            if let project = item.task.project {
                Text(project)
            }
            if let priority = item.task.priority, Format.priority(priority) != nil {
                PriorityMark(priority: priority)
                    .font(.system(size: 11))
            }
        }
        .font(.system(size: 11.5))
        .foregroundStyle(Theme.text3)
        .lineLimit(1)
    }
}

/// Why the last save stopped (D4).
private struct NoticeBanner: View {
    let notice: PlanSheet.Notice

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(Theme.danger)
            VStack(alignment: .leading, spacing: 2) {
                Text(notice.message).foregroundStyle(Theme.text)
                if let reason = notice.reason {
                    Text(reason).foregroundStyle(Theme.text2)
                }
            }
            .textSelection(.enabled)
            Spacer(minLength: 0)
        }
        .font(.system(size: 12))
        .padding(.horizontal, 22)
        .padding(.vertical, 9)
        .background(Theme.dangerSoft)
    }
}

/// The capacity column: what is left or over, the bar, the arithmetic, and
/// notes about unknown time and going over.
private struct CapacityPanel: View {
    let sheet: PlanSheet

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(sheet.isOverCapacity ? "\(Format.minutes(sheet.remainingMinutes)) over" : "\(Format.minutes(sheet.remainingMinutes)) free")
                        .font(.system(size: 26, weight: .semibold))
                        .tracking(-0.5)
                        .monospacedDigit()
                        .foregroundStyle(sheet.isOverCapacity ? Theme.danger : Theme.text)
                    Text("\(sheet.chosen.count) voluntary \(sheet.chosen.count == 1 ? "task" : "tasks") selected · \(Format.minutes(sheet.chosenMinutes)) estimated")
                        .font(.system(size: 12))
                        .foregroundStyle(Theme.text3)
                }
                VStack(spacing: 3) {
                    CapacityBar(sheet: sheet)
                    HStack {
                        Text("0")
                        Spacer()
                        Text("\(Format.minutes(sheet.plan.capacityMinutes)) capacity")
                    }
                    .font(.system(size: 10.5))
                    .foregroundStyle(Theme.text3)
                    .monospacedDigit()
                }
                table
                if sheet.unestimatedFixed > 0 || sheet.unestimatedChosen > 0 {
                    unknownNote
                }
                if sheet.isOverCapacity {
                    note(icon: "exclamationmark.triangle", warn: true) {
                        Text("\(Text("Over capacity by \(Format.minutes(sheet.remainingMinutes)).").fontWeight(.semibold)) You can still confirm — consider deferring a candidate or rollover item.")
                    }
                }
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 18)
        }
        .background(Theme.panel)
    }

    private var table: some View {
        Grid(alignment: .leading, horizontalSpacing: 9, verticalSpacing: 7) {
            tableRow("Capacity", Format.minutes(sheet.plan.capacityMinutes)) {
                RoundedRectangle(cornerRadius: 3).strokeBorder(Theme.separatorStrong, lineWidth: 1)
            }
            tableRow("Obligations", "− \(Format.minutes(sheet.fixedMinutes))") { RoundedRectangle(cornerRadius: 3).fill(Theme.barFixed) }
            tableRow("Buffer", "− \(Format.minutes(sheet.plan.bufferMinutes))") { BufferStripes().clipShape(RoundedRectangle(cornerRadius: 3)) }
            Rectangle().fill(Theme.separator).frame(height: 0.5).gridCellUnsizedAxes(.horizontal)
            GridRow {
                Color.clear.frame(width: 10, height: 10)
                Text("Available for chosen work").fontWeight(.semibold).foregroundStyle(Theme.text)
                Text(signed(sheet.availableMinutes)).fontWeight(.semibold).foregroundStyle(Theme.text)
                    .gridColumnAlignment(.trailing)
            }
            tableRow("Selected", Format.minutes(sheet.chosenMinutes)) { RoundedRectangle(cornerRadius: 3).fill(Theme.accent) }
        }
        .font(.system(size: 12.5))
        .monospacedDigit()
        .accessibilityElement(children: .combine)
    }

    private func tableRow(_ title: String, _ value: String, swatch: () -> some View) -> some View {
        GridRow {
            swatch().frame(width: 10, height: 10)
            Text(title).foregroundStyle(Theme.text)
                .frame(maxWidth: .infinity, alignment: .leading)
            Text(value).foregroundStyle(Theme.text2)
                .gridColumnAlignment(.trailing)
        }
    }

    private var unknownNote: some View {
        note(icon: "questionmark.circle", warn: false) {
            VStack(alignment: .leading, spacing: 0) {
                Text("Unknown time").fontWeight(.semibold)
                if sheet.unestimatedFixed > 0 {
                    Text("\(sheet.unestimatedFixed) \(sheet.unestimatedFixed == 1 ? "obligation" : "obligations") without an estimate")
                }
                if sheet.unestimatedChosen > 0 {
                    Text("\(sheet.unestimatedChosen) selected \(sheet.unestimatedChosen == 1 ? "task" : "tasks") without an estimate")
                }
                Text("Not counted above — remaining time may be lower.")
                    .foregroundStyle(Theme.text3)
                    .padding(.top, 2)
            }
        }
    }

    private func note(icon: String, warn: Bool, @ViewBuilder content: () -> some View) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Image(systemName: icon).foregroundStyle(warn ? Theme.danger : Theme.text3)
            content()
                .foregroundStyle(warn ? Theme.text : Theme.text2)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .font(.system(size: 12))
        .padding(.horizontal, 11)
        .padding(.vertical, 9)
        .background(warn ? Theme.dangerSoft : Theme.elevated, in: RoundedRectangle(cornerRadius: 8))
        .overlay {
            if !warn {
                RoundedRectangle(cornerRadius: 8).strokeBorder(Theme.separator, lineWidth: 0.5)
            }
        }
    }

    private func signed(_ minutes: Int) -> String {
        minutes < 0 ? "− \(Format.minutes(minutes))" : Format.minutes(minutes)
    }
}

/// Obligations, buffer and chosen work against the day's capacity. Chosen
/// work past what is available shows in red, and the bar grows to fit it.
private struct CapacityBar: View {
    let sheet: PlanSheet

    var body: some View {
        let fixed = sheet.fixedMinutes, buffer = sheet.plan.bufferMinutes, chosen = sheet.chosenMinutes
        let within = max(0, min(chosen, sheet.availableMinutes))
        let total = max(sheet.plan.capacityMinutes, fixed + buffer + chosen, 1)
        GeometryReader { geometry in
            let width = { (minutes: Int) in geometry.size.width * CGFloat(minutes) / CGFloat(total) }
            HStack(spacing: 0) {
                Theme.barFixed.frame(width: width(fixed))
                BufferStripes().frame(width: width(buffer))
                Theme.accent.frame(width: width(within))
                Theme.danger.frame(width: width(chosen - within))
                Spacer(minLength: 0)
            }
        }
        .frame(height: 10)
        .background(Theme.sunken)
        .clipShape(RoundedRectangle(cornerRadius: 5))
        .overlay(RoundedRectangle(cornerRadius: 5).strokeBorder(Theme.separator, lineWidth: 0.5))
        .animation(.snappy, value: chosen)
        .accessibilityElement()
        .accessibilityLabel(
            "Obligations \(Format.minutes(fixed)), buffer \(Format.minutes(buffer)), selected \(Format.minutes(chosen)) of \(Format.minutes(sheet.plan.capacityMinutes))")
    }
}

/// The buffer's diagonal stripes.
private struct BufferStripes: View {
    var body: some View {
        Canvas { context, size in
            context.fill(Path(CGRect(origin: .zero, size: size)), with: .color(Theme.barBuffer))
            var stripes = Path()
            var x = -size.height
            while x < size.width {
                stripes.move(to: CGPoint(x: x, y: size.height))
                stripes.addLine(to: CGPoint(x: x + size.height, y: 0))
                x += 8
            }
            context.stroke(stripes, with: .color(Theme.barBufferStripe), lineWidth: 2.8)
        }
    }
}
