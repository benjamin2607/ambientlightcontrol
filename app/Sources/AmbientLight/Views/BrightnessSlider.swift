import SwiftUI

/// Gradient slider (dim → full colour) with −/+ steppers. Reports whole percent values from 1 to 100.
struct BrightnessSlider: View {
    let value: Int
    let isEnabled: Bool
    let theme: Theme
    let onChange: (Int) -> Void
    var onEditingChanged: (Bool) -> Void = { _ in }

    @State private var isDragging = false
    private let step = 10

    var body: some View {
        VStack(spacing: 6) {
            HStack(spacing: 10) {
                stepButton("sun.min", delta: -step)
                track
                stepButton("sun.max", delta: step)
            }
            HStack {
                Text("1 %")
                Spacer()
                Text("100 %")
            }
            .font(.caption.weight(.semibold))
            .foregroundStyle(.secondary)
            .padding(.horizontal, 46)
        }
        .disabled(!isEnabled)
        .opacity(isEnabled ? 1 : 0.5)
    }

    private var track: some View {
        GeometryReader { geo in
            let fraction = Double(value - 1) / 99
            let thumb: CGFloat = 30
            let x = (geo.size.width - thumb) * fraction
            ZStack(alignment: .leading) {
                Capsule().fill(.black.opacity(0.35))
                Capsule()
                    .fill(LinearGradient(colors: theme.sliderColors, startPoint: .leading, endPoint: .trailing))
                    .mask(alignment: .leading) { Capsule().frame(width: x + thumb) }
                Circle()
                    .fill(.white)
                    .frame(width: thumb - 6, height: thumb - 6)
                    .shadow(radius: 3)
                    .offset(x: x + 3)
            }
            .contentShape(Rectangle())
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { drag in
                        if !isDragging { isDragging = true; onEditingChanged(true) }
                        let f = min(max((drag.location.x - thumb / 2) / (geo.size.width - thumb), 0), 1)
                        commit(1 + Int((f * 99).rounded()))
                    }
                    .onEnded { _ in
                        isDragging = false
                        onEditingChanged(false)
                    }
            )
        }
        .frame(height: 36)
    }

    private func stepButton(_ symbol: String, delta: Int) -> some View {
        Button {
            // Snap to the 10 % grid first, so steps land on round numbers.
            let snapped = delta > 0 ? (value / step + 1) * step : ((value - 1) / step) * step
            commit(snapped)
        } label: {
            Image(systemName: symbol)
                .font(.body.weight(.semibold))
                .frame(width: 36, height: 36)
                .background(Circle().fill(.white.opacity(0.08)))
        }
        .buttonStyle(.plain)
        .help(delta > 0 ? "Brighter" : "Dimmer")
    }

    private func commit(_ raw: Int) {
        let clamped = min(max(raw, 1), 100)
        if clamped != value { onChange(clamped) }
    }
}
