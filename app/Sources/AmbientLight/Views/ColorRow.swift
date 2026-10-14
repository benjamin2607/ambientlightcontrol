import SwiftUI

/// Preset swatches plus a custom colour well. The chosen colour is also the automation's daytime colour.
struct ColorRow: View {
    let selected: Int
    let isColorMode: Bool
    let theme: Theme
    let onSelect: (Int) -> Void

    static let presets: [(name: String, rgb: Int)] = [
        ("Warm white", 0xFFB46B),
        ("Amber", 0xFF8A1F),
        ("Red", 0xFF2A2A),
        ("Pink", 0xFF4FA3),
        ("Purple", 0x8F4BFF),
        ("Blue", 0x2E6BFF),
        ("Cyan", 0x22D3EE),
        ("Green", 0x34D058),
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Daytime color").font(.caption).foregroundStyle(.secondary)
            HStack(spacing: 0) {
                ForEach(Self.presets, id: \.rgb) { preset in
                    swatch(preset.rgb, name: preset.name)
                    Spacer(minLength: 0)
                }
                customButton
            }
        }
    }

    /// Rainbow ring; opens the system colour panel. Shows the current colour when it is not a preset.
    private var customButton: some View {
        let isCustom = isColorMode && !Self.presets.contains { $0.rgb == selected }
        return Button { ColorPanelBridge.shared.show(initial: selected, onChange: onSelect) } label: {
            Circle()
                .strokeBorder(AngularGradient(colors: Theme.spectrum, center: .center), lineWidth: 4)
                .background(Circle().fill(isCustom ? RGB(hex: selected).color : .clear).padding(5))
                .frame(width: 24, height: 24)
                .overlay { if !isCustom { Image(systemName: "plus").font(.system(size: 9, weight: .bold)) } }
                .padding(2)
                .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .help("Custom color")
        .accessibilityLabel("Custom color")
    }

    private func swatch(_ rgb: Int, name: String) -> some View {
        let isSelected = isColorMode && rgb == selected
        return Button { onSelect(rgb) } label: {
            Circle()
                .fill(RGB(hex: rgb).color)
                .frame(width: 24, height: 24)
                .overlay { Circle().strokeBorder(.white.opacity(isSelected ? 0.95 : 0.15), lineWidth: isSelected ? 2 : 1) }
                .padding(2)
                .overlay { if isSelected { Circle().strokeBorder(.black.opacity(0.4), lineWidth: 1).padding(4) } }
                .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .help(name)
        .accessibilityLabel(name)
    }
}

/// Connects the shared NSColorPanel to a closure. SwiftUI's ColorPicker draws a large well that
/// doesn't fit the popover design.
@MainActor
final class ColorPanelBridge: NSObject {
    static let shared = ColorPanelBridge()
    private var onChange: ((Int) -> Void)?

    func show(initial rgb: Int, onChange: @escaping (Int) -> Void) {
        let panel = NSColorPanel.shared
        self.onChange = nil // don't echo the initial colour back
        panel.showsAlpha = false
        panel.isContinuous = true
        panel.color = NSColor(RGB(hex: rgb).color)
        panel.setTarget(self)
        panel.setAction(#selector(colorChanged(_:)))
        self.onChange = onChange
        panel.level = .popUpMenu + 1 // above the menu bar panel
        panel.orderFrontRegardless()
    }

    @objc private func colorChanged(_ sender: NSColorPanel) {
        onChange?(RGB(Color(nsColor: sender.color)).hex)
    }
}
