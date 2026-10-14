// Renders the app icon (1024×1024 PNG). Used by scripts/make-icon.sh.
// Usage: swiftc -parse-as-library -o /tmp/make-icon scripts/make-icon.swift && /tmp/make-icon out.png
import AppKit
import SwiftUI

struct AppIcon: View {
    var body: some View {
        ZStack {
            // macOS icon grid: 824 pt squircle centred on a 1024 canvas.
            RoundedRectangle(cornerRadius: 185, style: .continuous)
                .fill(LinearGradient(
                    colors: [Color(red: 0.13, green: 0.12, blue: 0.22), Color(red: 0.05, green: 0.05, blue: 0.09)],
                    startPoint: .top, endPoint: .bottom
                ))
                .frame(width: 824, height: 824)
                .shadow(color: .black.opacity(0.3), radius: 18, y: 10)

            // Backlight glow
            RoundedRectangle(cornerRadius: 90, style: .continuous)
                .fill(AngularGradient(
                    colors: [.orange, .pink, .purple, .blue, .cyan, .orange],
                    center: .center
                ))
                .frame(width: 680, height: 450)
                .blur(radius: 45)
                .opacity(1)

            // TV
            VStack(spacing: 22) {
                RoundedRectangle(cornerRadius: 26, style: .continuous)
                    .fill(LinearGradient(colors: [Color(white: 0.16), Color(white: 0.04)], startPoint: .top, endPoint: .bottom))
                    .overlay {
                        RoundedRectangle(cornerRadius: 26, style: .continuous).strokeBorder(Color(white: 0.32), lineWidth: 8)
                    }
                    .frame(width: 520, height: 300)
                Capsule().fill(Color(white: 0.35)).frame(width: 180, height: 16)
            }
            .offset(y: 20)
            .shadow(color: .black.opacity(0.45), radius: 20, y: 10)
        }
        .frame(width: 1024, height: 1024)
    }
}

@main
struct MakeIcon {
    @MainActor static func main() {
        let renderer = ImageRenderer(content: AppIcon())
        renderer.scale = 1
        guard let image = renderer.cgImage,
              let png = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:]) else {
            fatalError("render failed")
        }
        try! png.write(to: URL(fileURLWithPath: CommandLine.arguments[1]))
    }
}
