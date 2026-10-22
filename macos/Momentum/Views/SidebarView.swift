import SwiftUI

struct SidebarView: View {
    @Environment(AppStore.self) private var store
    let focus: FocusState<FocusedPane?>.Binding

    var body: some View {
        List(selection: Binding<ViewName?>(get: { store.view }, set: { if let view = $0 { store.view = view } })) {
            ForEach(ViewName.allCases) { view in
                HStack(spacing: 8) {
                    Label {
                        Text(view.title).foregroundStyle(Theme.text)
                    } icon: {
                        Image(systemName: view.symbol).foregroundStyle(Theme.accentText)
                    }
                    Spacer()
                    Group {
                        if view == .today, store.overdueCount > 0 {
                            Text("\(store.overdueCount)")
                                .fontWeight(.semibold)
                                .foregroundStyle(Theme.danger)
                                .help("\(store.overdueCount) overdue")
                        }
                        if view != .completed, store.snapshot != nil {
                            Text("\(store.count(view))")
                                .foregroundStyle(Theme.text3)
                        }
                    }
                    .font(.system(size: 12))
                    .monospacedDigit()
                }
                .tag(view)
            }
            if let plan = store.snapshot?.plan, plan.enabled {
                TodayPlanBox(plan: plan)
                    .selectionDisabled()
                    .listRowInsets(EdgeInsets(top: 10, leading: 0, bottom: 4, trailing: 0))
            }
        }
        .scrollContentBackground(.hidden)
        .background(Theme.sidebar)
        .focused(focus, equals: .sidebar)
        .taskKeyCommands(focusSearch: { focus.wrappedValue = .search })
        .safeAreaInset(edge: .bottom) {
            VStack(alignment: .leading, spacing: 2) {
                if let url = store.backendURL {
                    footerRow(store.account?.email ?? "Signed out", systemImage: "person.crop.circle")
                        .help(store.account.map { "Signed in until \($0.expiresAt.formatted(date: .abbreviated, time: .omitted))" } ?? "")
                    footerRow(URL(string: url)?.host() ?? url, systemImage: "server.rack")
                        .help(url)
                }
                Button {
                    store.showShortcuts = true
                } label: {
                    HStack(spacing: 9) {
                        Image(systemName: "keyboard").foregroundStyle(Theme.text3).frame(width: 16)
                        Text("Keyboard Shortcuts")
                        Spacer()
                        KeyCap("?")
                    }
                    .foregroundStyle(Theme.text2)
                    .padding(.horizontal, 9)
                    .frame(height: 28)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 10)
            .padding(.top, 8)
            .padding(.bottom, 12)
            .overlay(alignment: .top) {
                Rectangle().fill(Theme.separator).frame(height: 0.5)
            }
            .background(Theme.sidebar)
        }
    }

    private func footerRow(_ title: String, systemImage: String) -> some View {
        HStack(spacing: 8) {
            Image(systemName: systemImage).foregroundStyle(Theme.text3)
            Text(title).foregroundStyle(Theme.text2)
        }
        .font(.system(size: 12))
        .padding(.horizontal, 9)
        .padding(.vertical, 6)
    }
}

/// The day's plan at a glance (D7): the tasks tagged for today outside the
/// obligations, a meter over the day's capacity, and the Plan Today button.
private struct TodayPlanBox: View {
    @Environment(AppStore.self) private var store
    let plan: PlanSummary

    private var tasksMinutes: Int { plan.fixedMinutes + plan.chosenMinutes }
    private var freeMinutes: Int { plan.capacityMinutes - plan.calendarMinutes - plan.bufferMinutes - tasksMinutes }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .firstTextBaseline) {
                Text("Today’s plan")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(Theme.text)
                Spacer()
                Text("\(plan.chosenCount) chosen")
            }
            meter
                .padding(.top, 8)
            HStack(alignment: .firstTextBaseline) {
                Text("\(Format.minutes(tasksMinutes)) of tasks")
                    .help(unestimated)
                Spacer()
                Text(freeMinutes < 0 ? "\(Format.minutes(freeMinutes)) over" : "\(Format.minutes(freeMinutes)) free")
                    .foregroundStyle(freeMinutes < 0 ? Theme.danger : Theme.text3)
            }
            .padding(.top, 6)
            Button {
                Task { await store.openPlan() }
            } label: {
                HStack(spacing: 6) {
                    Label("Plan Today", systemImage: "text.badge.plus")
                    Text("⇧P").opacity(0.72)
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(Theme.tint)
            .disabled(store.account == nil || store.hasOverlay)
            .padding(.top, 10)
        }
        .font(.system(size: 11.5))
        .foregroundStyle(Theme.text3)
        .monospacedDigit()
        .padding(.horizontal, 12)
        .padding(.top, 11)
        .padding(.bottom, 12)
        .background(Theme.elevated, in: RoundedRectangle(cornerRadius: 10))
        .overlay(RoundedRectangle(cornerRadius: 10).strokeBorder(Theme.separatorStrong, lineWidth: 0.5))
    }

    /// Obligations, calendar, buffer and chosen work as shares of the day's
    /// capacity.
    private var meter: some View {
        GeometryReader { geometry in
            let width = { (minutes: Int) in geometry.size.width * CGFloat(minutes) / CGFloat(max(plan.capacityMinutes, 1)) }
            HStack(spacing: 0) {
                Theme.barFixed.frame(width: width(plan.fixedMinutes))
                Theme.barCalendar.frame(width: width(plan.calendarMinutes))
                Theme.barBufferStripe.frame(width: width(plan.bufferMinutes))
                Theme.accent.frame(width: width(plan.chosenMinutes))
                Spacer(minLength: 0)
            }
        }
        .frame(height: 5)
        .background(Theme.sunken)
        .clipShape(RoundedRectangle(cornerRadius: 3))
        .accessibilityElement()
        .accessibilityLabel(
            "Obligations \(Format.minutes(plan.fixedMinutes)), calendar \(Format.minutes(plan.calendarMinutes)), buffer \(Format.minutes(plan.bufferMinutes)), chosen \(Format.minutes(plan.chosenMinutes)) of \(Format.minutes(plan.capacityMinutes))")
    }

    /// Tasks without an estimate, which the minutes leave out.
    private var unestimated: String {
        let count = plan.unestimatedFixed + plan.unestimatedChosen
        return count == 0 ? "" : "\(count) \(count == 1 ? "task has" : "tasks have") no estimate and \(count == 1 ? "isn’t" : "aren’t") counted"
    }
}
