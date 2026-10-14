import SwiftUI

/// Stylised TV with the backlight glowing behind it: a steady colour, or slowly shifting hues in video mode.
struct BacklightView: View {
    let light: ServerStatus.Light
    let tvActive: Bool
    let theme: Theme

    private var intensity: Double { light.power ? 0.35 + 0.65 * Double(light.brightness) / 100 : 0 }

    var body: some View {
        ZStack {
            glow
            tv
        }
        .frame(maxWidth: .infinity)
        .animation(.easeInOut(duration: 0.6), value: light.power)
        .animation(.easeInOut(duration: 0.4), value: light.brightness)
    }

    @ViewBuilder
    private var glow: some View {
        if light.isVideo {
            // .periodic instead of .animation: the display-link schedule doesn't reliably tick in panels.
            TimelineView(.periodic(from: .now, by: 1.0 / 30)) { timeline in
                let angle = timeline.date.timeIntervalSinceReferenceDate.truncatingRemainder(dividingBy: 12) / 12 * 360
                glowShape(AngularGradient(colors: Theme.spectrum, center: .center, angle: .degrees(angle)))
            }
            .transition(.opacity)
        } else {
            glowShape(theme.accent)
        }
    }

    private func glowShape(_ style: some ShapeStyle) -> some View {
        RoundedRectangle(cornerRadius: 30, style: .continuous)
            .fill(style)
            .frame(width: 236, height: 128)
            .blur(radius: 26)
            .opacity(intensity)
    }

    private var tv: some View {
        VStack(spacing: 0) {
            RoundedRectangle(cornerRadius: 6, style: .continuous)
                .fill(LinearGradient(colors: [Color(white: 0.16), Color(white: 0.07)], startPoint: .top, endPoint: .bottom))
                .overlay {
                    RoundedRectangle(cornerRadius: 6, style: .continuous)
                        .strokeBorder(Color(white: 0.28), lineWidth: 1.5)
                }
                .overlay {
                    if tvActive { screenContent }
                }
                .frame(width: 200, height: 112)
            Capsule().fill(Color(white: 0.25)).frame(width: 64, height: 4).padding(.top, 6)
        }
        .shadow(color: .black.opacity(0.5), radius: 8, y: 4)
    }

    /// A hint of a picture when the TV is on.
    private var screenContent: some View {
        LinearGradient(colors: [Color(red: 0.20, green: 0.32, blue: 0.55), Color(red: 0.55, green: 0.30, blue: 0.40)],
                       startPoint: .topLeading, endPoint: .bottomTrailing)
            .opacity(0.55)
            .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
            .padding(4)
            .overlay {
                Image(systemName: "play.fill").font(.title2).foregroundStyle(.white.opacity(0.7))
            }
            .transition(.opacity)
    }
}
