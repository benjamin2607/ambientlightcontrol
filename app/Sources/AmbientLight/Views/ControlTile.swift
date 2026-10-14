import SwiftUI

/// Square tile (Mode / Automation / TV). Click opens a small picker; the picker content is supplied by the caller.
struct ControlTile<Icon: View, Picker: View>: View {
    let title: String
    let value: String
    let theme: Theme
    @ViewBuilder let icon: () -> Icon
    /// Receives a dismiss action so the picker can close itself after a selection.
    @ViewBuilder let picker: (_ dismiss: @escaping () -> Void) -> Picker

    @State private var showPicker = false

    var body: some View {
        Button { showPicker.toggle() } label: {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    icon()
                        .font(.title3)
                        .frame(width: 22, height: 22)
                        .foregroundStyle(theme.accent)
                    Spacer()
                    Image(systemName: "chevron.down").font(.caption2).foregroundStyle(.secondary)
                }
                VStack(alignment: .leading, spacing: 2) {
                    Text(title).font(.caption).foregroundStyle(.secondary)
                    Text(value).font(.callout.weight(.semibold)).lineLimit(1)
                }
            }
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(RoundedRectangle(cornerRadius: 14, style: .continuous).fill(.white.opacity(0.07)))
        }
        .buttonStyle(.plain)
        .popover(isPresented: $showPicker, arrowEdge: .bottom) {
            picker { showPicker = false }
        }
    }
}

extension ControlTile where Icon == Image, Picker == OptionList {
    /// Standard tile with an SF Symbol and a list of options (each with its own symbol).
    init(
        title: String, value: String, symbol: String, theme: Theme,
        options: [(id: String, label: String, symbol: String)], selected: String?, onSelect: @escaping (String) -> Void
    ) {
        self.init(title: title, value: value, theme: theme) {
            Image(systemName: symbol)
        } picker: { dismiss in
            OptionList(options: options, selected: selected, accent: theme.accent) { onSelect($0); dismiss() }
        }
    }
}

struct OptionList: View {
    let options: [(id: String, label: String, symbol: String)]
    let selected: String?
    let accent: Color
    let onSelect: (String) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            ForEach(options, id: \.id) { option in
                Button { onSelect(option.id) } label: {
                    HStack(spacing: 10) {
                        Image(systemName: option.symbol)
                            .foregroundStyle(accent)
                            .frame(width: 20)
                        Text(option.label)
                        Spacer(minLength: 16)
                        if option.id == selected { Image(systemName: "checkmark").foregroundStyle(accent) }
                    }
                    .padding(.horizontal, 10).padding(.vertical, 6)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
        }
        .padding(6)
        .frame(minWidth: 160)
    }
}
