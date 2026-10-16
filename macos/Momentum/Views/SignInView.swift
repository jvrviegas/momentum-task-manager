import AppKit
import SwiftUI

/// The device sign-in (D1): shows the code to enter and opens the browser,
/// where the person signs in with Google or GitHub.
struct SignInView: View {
    @Environment(AppStore.self) private var store
    @Environment(\.openURL) private var openURL

    var body: some View {
        VStack(spacing: 16) {
            VStack(spacing: 4) {
                Text("Sign in to Momentum").font(.title2.bold())
                if let server = store.signIn?.server {
                    Text(server).foregroundStyle(.secondary).textSelection(.enabled)
                }
            }
            switch store.signIn?.phase ?? .starting {
            case .starting:
                ProgressView("Starting sign-in…")
                    .frame(minHeight: 120)
            case .waiting(let start):
                waiting(start)
            case .failed(let message):
                VStack(spacing: 12) {
                    Label(message, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.orange)
                        .multilineTextAlignment(.center)
                        .textSelection(.enabled)
                    Button("Try Again") { store.beginSignIn() }
                        .keyboardShortcut(.defaultAction)
                }
                .frame(minHeight: 120)
            }
            HStack {
                Spacer()
                Button("Cancel", role: .cancel) { store.cancelSignIn() }
                    .keyboardShortcut(.cancelAction)
            }
        }
        .padding(24)
        .frame(width: 440)
    }

    private func waiting(_ start: DeviceSignIn) -> some View {
        VStack(spacing: 12) {
            Text("Enter this code in your browser, then sign in with Google or GitHub.")
                .multilineTextAlignment(.center)
            Text(start.userCode)
                .font(.system(size: 34, weight: .semibold, design: .monospaced))
                .kerning(4)
                .textSelection(.enabled)
                .accessibilityLabel("Code \(start.userCode.map(String.init).joined(separator: " "))")
            HStack {
                Button("Open Browser") { open(start) }
                    .keyboardShortcut(.defaultAction)
                Button("Copy Code") {
                    NSPasteboard.general.clearContents()
                    NSPasteboard.general.setString(start.userCode, forType: .string)
                }
            }
            Text("Or go to \(start.verificationUrl)")
                .font(.callout)
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
            HStack(spacing: 8) {
                ProgressView().controlSize(.small)
                Text("Waiting for you to finish signing in…").foregroundStyle(.secondary)
            }
        }
        // Opens the browser once per code, as `momentum login` does.
        .task(id: start.deviceCode) { open(start) }
    }

    private func open(_ start: DeviceSignIn) {
        if let url = URL(string: start.verificationUrlComplete) {
            openURL(url)
        }
    }
}
