import SwiftUI

/// Dynamic palette: the popover is tinted with the colour the backlight currently shows.
struct Theme {
    let light: RGB
    let isOn: Bool
    let isVideo: Bool

    /// Popover corner radius; larger than the system default so the panel sits softly under the menu bar.
    static let cornerRadius: CGFloat = 26

    private static let offTop = RGB(0.17, 0.17, 0.19), offBottom = RGB(0.09, 0.09, 0.10)
    private static let videoAccent = RGB(0.56, 0.62, 1.00)

    /// Hues of the animated glow in video mode.
    static let spectrum: [Color] = [
        RGB(1.00, 0.36, 0.36), RGB(1.00, 0.70, 0.25), RGB(0.45, 0.90, 0.55),
        RGB(0.30, 0.70, 1.00), RGB(0.70, 0.45, 1.00), RGB(1.00, 0.36, 0.36),
    ].map(\.color)

    init(status: ServerStatus.Light) {
        light = RGB(hex: status.color)
        isOn = status.power
        isVideo = status.isVideo
    }

    private var accentRGB: RGB {
        if isVideo { return Self.videoAccent }
        return light.vivid
    }

    var accent: Color { isOn ? accentRGB.color : .gray }
    var backgroundTop: Color { (isOn ? Self.offTop.lerp(accentRGB, 0.30) : Self.offTop).color }
    var backgroundBottom: Color { (isOn ? Self.offBottom.lerp(accentRGB, 0.10) : Self.offBottom).color }
    var sliderColors: [Color] {
        isOn ? [accentRGB.lerp(RGB(0.1, 0.1, 0.1), 0.6).color, accentRGB.color] : [Color(white: 0.45), Color(white: 0.6)]
    }

    static let offBackground = LinearGradient(colors: [offTop.color, offBottom.color], startPoint: .top, endPoint: .bottom)

    var background: LinearGradient {
        LinearGradient(colors: [backgroundTop, backgroundBottom], startPoint: .top, endPoint: .bottom)
    }
}

struct RGB: Equatable {
    var r, g, b: Double
    init(_ r: Double, _ g: Double, _ b: Double) { (self.r, self.g, self.b) = (r, g, b) }

    init(hex: Int) {
        self.init(Double((hex >> 16) & 0xFF) / 255, Double((hex >> 8) & 0xFF) / 255, Double(hex & 0xFF) / 255)
    }

    init(_ color: Color) {
        let c = NSColor(color).usingColorSpace(.sRGB) ?? .white
        self.init(Double(c.redComponent), Double(c.greenComponent), Double(c.blueComponent))
    }

    var hex: Int {
        func channel(_ v: Double) -> Int { Int((min(max(v, 0), 1) * 255).rounded()) }
        return channel(r) << 16 | channel(g) << 8 | channel(b)
    }

    /// Same hue at full brightness, so dark colours still give a visible accent.
    var vivid: RGB {
        let peak = max(r, g, b)
        guard peak > 0.02 else { return RGB(1, 1, 1) }
        return RGB(r / peak, g / peak, b / peak)
    }

    func lerp(_ other: RGB, _ t: Double) -> RGB {
        let t = min(max(t, 0), 1)
        return RGB(r + (other.r - r) * t, g + (other.g - g) * t, b + (other.b - b) * t)
    }

    var color: Color { Color(.sRGB, red: r, green: g, blue: b) }
}
