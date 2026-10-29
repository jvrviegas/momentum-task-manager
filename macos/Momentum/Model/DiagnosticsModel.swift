import Foundation
import Observation

/// What the Diagnostics tab shows: the bridge's checks, and a report to copy
/// for a bug report. The report is built from the checks alone, which the
/// bridge has already stripped of secrets and holds no event title, and from
/// the versions, so no token or title can get into it.
@MainActor
@Observable
final class DiagnosticsModel {
    private(set) var report: DoctorReport?
    private(set) var error: String?
    private(set) var running = false
    @ObservationIgnored weak var store: AppStore?

    func run() async {
        guard let store, !running else { return }
        running = true
        defer { running = false }
        do {
            report = try await store.bridge.doctor()
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// The bridge's checks, and one for macOS Calendar when it is this Mac's
    /// source, since only the app can ask the Mac about it.
    var checks: [DoctorCheck] {
        guard let report else { return [] }
        guard let mac = store?.macCalendar, mac.source == .mac else { return report.checks }
        let check: DoctorCheck
        switch mac.access {
        case .granted:
            let count = mac.calendars.count
            check = DoctorCheck(name: "macOS Calendar", status: "ok", detail: "\(count) \(count == 1 ? "calendar" : "calendars"); read-only")
        case .denied:
            check = DoctorCheck(name: "macOS Calendar", status: "fail", detail: "no access; allow it in System Settings ▸ Privacy & Security ▸ Calendars")
        case .notDetermined:
            check = DoctorCheck(name: "macOS Calendar", status: "skip", detail: "the Mac has not been asked for access yet")
        }
        return report.checks + [check]
    }

    var ready: Bool { checks.allSatisfy { $0.status != "fail" } }

    /// The report to copy: the versions, the config path and each check.
    func reportText(app: String = DiagnosticsModel.appVersion(), system: String = ProcessInfo.processInfo.operatingSystemVersionString) -> String {
        guard let report else { return "" }
        var lines = [
            "Momentum Task Manager \(app)", "Bridge \(report.bridgeVersion)", "macOS \(system)", "Config \(report.configPath)", "",
        ]
        for check in checks {
            let status = check.status == "ok" ? "ok" : check.status == "skip" ? "skip" : "fail"
            lines.append("[\(status)] " + check.name.padding(toLength: max(24, check.name.count), withPad: " ", startingAt: 0) + " " + check.detail)
        }
        lines.append(ready ? "Doctor: ready" : "Doctor: issues found")
        return lines.joined(separator: "\n")
    }

    /// 0.1.0 (1), from the app's Info.plist.
    nonisolated static func appVersion(_ info: [String: Any]? = Bundle.main.infoDictionary) -> String {
        let short = info?["CFBundleShortVersionString"] as? String ?? "unknown"
        let build = info?["CFBundleVersion"] as? String
        return build.map { "\(short) (\($0))" } ?? short
    }
}
