import AppKit
import SwiftUI

/// The "Momentum for macOS" design: warm neutral surfaces and one teal
/// accent, each with a light and a dark value.
enum Theme {
    /// Primary buttons: the design's teal even when the system accent is not
    /// Multicolor, deeper in dark mode so white text on it stays legible.
    static let tint = Color("AccentColor")

    static let sidebar = Color(light: 0xEBE7E0, dark: 0x22201D)
    static let content = Color(light: 0xFCFBF9, dark: 0x191715)
    static let panel = Color(light: 0xF7F5F1, dark: 0x1F1D1A)
    static let elevated = Color(light: 0xFFFFFF, dark: 0x2A2724)
    static let sunken = Color(light: 0xF1EEE9, dark: 0x151412)

    static let text = Color(light: 0x1E1B18, dark: 0xEDE9E3)
    static let text2 = Color(light: 0x585149, dark: 0xB9B1A7)
    static let text3 = Color(light: 0x736B62, dark: 0x968E84)
    static let separator = Color(light: 0x2E2216, 0.1, dark: 0xFFF4E4, 0.085)
    static let separatorStrong = Color(light: 0x2E2216, 0.17, dark: 0xFFF4E4, 0.14)
    static let hover = Color(light: 0x2E2216, 0.045, dark: 0xFFF4E4, 0.05)

    static let accent = Color(light: 0x2B6A5E, dark: 0x6CBFAB)
    static let accentInk = Color(light: 0xFFFFFF, dark: 0x0E201B)
    static let accentText = Color(light: 0x22594F, dark: 0x7FCFBB)
    static let accentSoft = Color(light: 0x2B6A5E, 0.11, dark: 0x6CBFAB, 0.15)
    static let danger = Color(light: 0xB3361D, dark: 0xF08A72)
    static let dangerSoft = Color(light: 0xB3361D, 0.09, dark: 0xF08A72, 0.12)
    static let warn = Color(light: 0x8A5300, dark: 0xE2B45E)
    static let warnSoft = Color(light: 0xB07000, 0.11, dark: 0xE2B45E, 0.13)

    /// Capture highlights behind recognized phrases.
    static let markProject = Color(light: 0x2B6A5E, 0.16, dark: 0x6CBFAB, 0.2)
    static let markDate = Color(light: 0x4668AA, 0.15, dark: 0x82A0E6, 0.2)
    static let markMeta = Color(light: 0x966E28, 0.16, dark: 0xE2B45E, 0.18)
    static let markError = Color(light: 0xB3361D, 0.16, dark: 0xF08A72, 0.22)

    static func priority(_ value: String?) -> Color {
        switch value {
        case "H": danger
        case "M": Color(light: 0x9A5B00, dark: 0xE2B45E)
        default: Color(light: 0x6F6A62, dark: 0xA39B90)
        }
    }
}

private extension Color {
    init(light: UInt32, _ lightOpacity: CGFloat = 1, dark: UInt32, _ darkOpacity: CGFloat = 1) {
        self.init(nsColor: NSColor(name: nil) { appearance in
            appearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
                ? NSColor(rgb: dark, alpha: darkOpacity)
                : NSColor(rgb: light, alpha: lightOpacity)
        })
    }
}

private extension NSColor {
    convenience init(rgb: UInt32, alpha: CGFloat) {
        self.init(
            srgbRed: CGFloat((rgb >> 16) & 0xFF) / 255, green: CGFloat((rgb >> 8) & 0xFF) / 255,
            blue: CGFloat(rgb & 0xFF) / 255, alpha: alpha
        )
    }
}

/// A key cap, as the design shows shortcuts.
struct KeyCap: View {
    let key: String

    init(_ key: String) {
        self.key = key
    }

    var body: some View {
        Text(key)
            .font(.system(size: 11, weight: .medium))
            .foregroundStyle(Theme.text2)
            .padding(.horizontal, 5)
            .padding(.vertical, 2)
            .background(Theme.hover, in: RoundedRectangle(cornerRadius: 4))
            .overlay(RoundedRectangle(cornerRadius: 4).strokeBorder(Theme.separatorStrong, lineWidth: 0.5))
    }
}

/// A button's title followed by its shortcut, as in the design's buttons.
struct ShortcutLabel: View {
    let title: String
    let systemImage: String
    let key: String

    var body: some View {
        HStack(spacing: 6) {
            Label(title, systemImage: systemImage)
            Text(key).opacity(0.72)
        }
    }
}
