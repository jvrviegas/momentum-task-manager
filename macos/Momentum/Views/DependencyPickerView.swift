import SwiftUI

/// The Add Dependency sheet (S-067): choose the task that the selected one
/// waits on. Typing narrows the list, ↑ ↓ choose, Return adds and Escape
/// cancels.
struct DependencyPickerView: View {
    @Environment(AppStore.self) private var store
    @FocusState private var searchFocused: Bool

    var body: some View {
        if let picker = store.dependencyPicker {
            let choices = picker.choices(in: store.dependencies)
            let chosen = picker.chosen(in: store.dependencies)
            VStack(alignment: .leading, spacing: 14) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Add Dependency").font(.title3.bold())
                    Text("“\(picker.task.description)” will wait on the task you choose.")
                        .foregroundStyle(Theme.text2)
                }
                TextField("Search tasks", text: Binding(
                    get: { store.dependencyPicker?.query ?? "" },
                    set: { store.dependencyPicker?.query = $0 }))
                    .textFieldStyle(.roundedBorder)
                    .focused($searchFocused)
                    .onKeyPress(.downArrow) { choose(1, among: choices, from: chosen) }
                    .onKeyPress(.upArrow) { choose(-1, among: choices, from: chosen) }
                list(choices, chosen: chosen, query: picker.query)
                    .frame(height: 260)
                HStack {
                    Spacer()
                    Button("Cancel", role: .cancel) { store.dependencyPicker = nil }
                        .keyboardShortcut(.cancelAction)
                    Button("Add", action: add)
                        .keyboardShortcut(.defaultAction)
                        .disabled(chosen == nil)
                }
            }
            .padding(20)
            .frame(width: 480)
            .onAppear { searchFocused = true }
        }
    }

    @ViewBuilder
    private func list(_ choices: [TaskItem], chosen: TaskItem?, query: String) -> some View {
        if choices.isEmpty {
            if query.trimmingCharacters(in: .whitespaces).isEmpty {
                ContentUnavailableView(
                    "No Tasks to Choose", systemImage: "link",
                    description: Text("Only pending tasks can be chosen, and not one that waits on this task."))
            } else {
                ContentUnavailableView.search(text: query)
            }
        } else {
            ScrollViewReader { proxy in
                List(choices, selection: Binding(get: { chosen?.uuid }, set: { store.dependencyPicker?.selection = $0 })) { task in
                    HStack(spacing: 8) {
                        Text(task.description).lineLimit(1).help(task.description)
                        Spacer(minLength: 8)
                        if let project = task.project {
                            Text(project).font(.callout).foregroundStyle(Theme.text3)
                        }
                    }
                    .tag(task.uuid)
                    .accessibilityElement(children: .combine)
                }
                .contextMenu(forSelectionType: String.self) { _ in } primaryAction: { _ in add() }
                .onChange(of: chosen?.uuid) { _, uuid in
                    if let uuid { proxy.scrollTo(uuid) }
                }
            }
        }
    }

    private func choose(_ step: Int, among choices: [TaskItem], from chosen: TaskItem?) -> KeyPress.Result {
        guard !choices.isEmpty else { return .ignored }
        let index = chosen.flatMap { chosen in choices.firstIndex { $0.uuid == chosen.uuid } } ?? 0
        store.dependencyPicker?.selection = choices[min(max(index + step, 0), choices.count - 1)].uuid
        return .handled
    }

    private func add() {
        Task { await store.addChosenDependency() }
    }
}
