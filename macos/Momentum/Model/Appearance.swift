import SwiftUI

/// Whether this Mac shows the app light or dark. A Mac-only preference kept
/// in UserDefaults: the terminal app's `theme` key is neither read nor
/// written (settings plan S2).
enum AppearanceChoice: String, CaseIterable, Identifiable {
    case system, light, dark

    static let key = "appearance"

    var id: Self { self }

    var title: String {
        switch self {
        case .system: "System"
        case .light: "Light"
        case .dark: "Dark"
        }
    }

    /// nil leaves the choice to the Mac.
    var colorScheme: ColorScheme? {
        switch self {
        case .system: nil
        case .light: .light
        case .dark: .dark
        }
    }
}

/// Applies the saved appearance to a window's content, so a change shows in
/// every window at once.
struct AppearancePreference: ViewModifier {
    @AppStorage(AppearanceChoice.key) private var choice = AppearanceChoice.system

    func body(content: Content) -> some View {
        content.preferredColorScheme(choice.colorScheme)
    }
}
