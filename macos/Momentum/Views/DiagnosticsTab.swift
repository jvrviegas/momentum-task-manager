import AppKit
import SwiftUI

/// Settings ▸ Diagnostics: the checks of the server, the sign-in, the time
/// zone, the config and the calendar, the versions and the config path, and a
/// report to copy for a bug report.
struct DiagnosticsTab: View {
    @Environment(AppStore.self) private var store
    @State private var copied = false

    var body: some View {
        let model = store.diagnostics
        Group {
            if let report = model.report {
                Form {
                    Section {
                        ForEach(model.checks) { check in
                            CheckRow(check: check)
                        }
                    } header: {
                        Text("Checks")
                    } footer: {
                        Label(
                            model.ready ? "Everything is working." : "Something needs attention.",
                            systemImage: model.ready ? "checkmark.circle.fill" : "exclamationmark.triangle.fill")
                    }
                    Section("About") {
                        LabeledContent("App", value: DiagnosticsModel.appVersion())
                        LabeledContent("Bridge", value: report.bridgeVersion)
                        LabeledContent("config.toml") {
                            Text(report.configPath)
                                .font(.system(size: 12.5, design: .monospaced))
                                .lineLimit(1)
                                .truncationMode(.middle)
                                .textSelection(.enabled)
                                .help(report.configPath)
                        }
                    }
                    Section {
                        HStack {
                            Button("Show in Finder") { ConfigFile.reveal(report.configPath) }
                                .help("Show config.toml in Finder")
                            Button("Copy Report") { copy(model) }
                                .help("Copy these checks and versions to paste into a bug report. They hold no sign-in code or event title.")
                            if copied {
                                Label("Copied", systemImage: "checkmark").foregroundStyle(Theme.text2)
                            }
                            Spacer()
                            if model.running { ProgressView().controlSize(.small) }
                            Button("Check Again") { Task { await model.run() } }
                                .disabled(model.running)
                        }
                    }
                }
                .formStyle(.grouped)
            } else if let error = model.error {
                ContentUnavailableView {
                    Label("Unable to run the checks", systemImage: "exclamationmark.triangle")
                } description: {
                    Text(error)
                } actions: {
                    Button("Check Again") { Task { await model.run() } }
                }
            } else {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task { await model.run() }
    }

    private func copy(_ model: DiagnosticsModel) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(model.reportText(), forType: .string)
        copied = true
        Task {
            try? await Task.sleep(for: .seconds(2))
            copied = false
        }
    }
}

/// One check: a symbol and a word, so the result is never a color alone.
private struct CheckRow: View {
    let check: DoctorCheck

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            Label {
                Text(word).font(.callout).foregroundStyle(Theme.text2)
            } icon: {
                Image(systemName: symbol).foregroundStyle(color)
            }
            .frame(width: 92, alignment: .leading)
            VStack(alignment: .leading, spacing: 2) {
                Text(check.name).fontWeight(.semibold)
                Text(check.detail).font(.callout).foregroundStyle(Theme.text2).textSelection(.enabled)
            }
            Spacer(minLength: 0)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(check.name), \(word)")
        .accessibilityValue(check.detail)
    }

    private var word: String {
        switch check.status {
        case "ok": "OK"
        case "skip": "Skipped"
        default: "Problem"
        }
    }

    private var symbol: String {
        switch check.status {
        case "ok": "checkmark.circle.fill"
        case "skip": "minus.circle.fill"
        default: "exclamationmark.triangle.fill"
        }
    }

    private var color: Color {
        switch check.status {
        case "ok": Theme.statusOK
        case "skip": Theme.text2
        default: Theme.danger
        }
    }
}
