import SwiftUI

struct PopoverView: View {
    @Environment(LightStore.self) private var store

    var body: some View {
        Group {
            if let status = store.status, status.light.ready {
                LightView(status: status)
            } else {
                PlaceholderView()
            }
        }
        .frame(width: 340)
        .clipShape(RoundedRectangle(cornerRadius: Theme.cornerRadius, style: .continuous))
        .overlay {
            RoundedRectangle(cornerRadius: Theme.cornerRadius, style: .continuous)
                .strokeBorder(.white.opacity(0.08), lineWidth: 1)
        }
        .preferredColorScheme(.dark)
    }
}

private struct LightView: View {
    @Environment(LightStore.self) private var store
    @Environment(UpdateChecker.self) private var updates
    let status: ServerStatus

    private var light: ServerStatus.Light { status.light }
    private var theme: Theme { Theme(status: light) }

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            if let update = updates.available { updateBanner(update) }
            header
            BacklightView(light: light, tvActive: status.tv.active, theme: theme)
                .frame(height: 150)
            brightnessReadout
            BrightnessSlider(
                value: light.brightness, isEnabled: light.power, theme: theme,
                onChange: { store.setBrightness($0) },
                onEditingChanged: { store.setInteracting($0) }
            )
            ColorRow(selected: status.settings.color, isColorMode: light.power && light.mode == "color", theme: theme) {
                store.setColor($0)
            }
            controls
            messages
        }
        .padding(20)
        .background(theme.background)
        .animation(.easeInOut(duration: 0.6), value: light.color)
        .animation(.easeInOut(duration: 0.4), value: light.power)
        .animation(.easeInOut(duration: 0.4), value: light.mode)
    }

    private var header: some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: 6) {
                Text(light.name).font(.headline)
                HStack(spacing: 8) {
                    Circle()
                        .fill(store.isSending ? .yellow : (light.online ? theme.accent : .red))
                        .frame(width: 6, height: 6)
                    Text(status.automationSummary)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            Spacer()
            powerButton
        }
    }

    private var powerButton: some View {
        Button(action: store.togglePower) {
            Image(systemName: "power")
                .font(.title2.weight(.semibold))
                .frame(width: 48, height: 48)
                .background(Circle().fill(light.power ? theme.accent : .white.opacity(0.1)))
                .foregroundStyle(light.power ? .black.opacity(0.75) : .white)
                .shadow(color: light.power ? theme.accent.opacity(0.6) : .clear, radius: 12)
        }
        .buttonStyle(.plain)
        .help(light.power ? "Turn off" : "Turn on")
    }

    private var brightnessReadout: some View {
        HStack(alignment: .firstTextBaseline, spacing: 4) {
            Text("\(light.brightness)")
                .font(.system(size: 64, weight: .light, design: .rounded))
                .monospacedDigit()
                .contentTransition(.numericText(value: Double(light.brightness)))
            Text("%").font(.title3.weight(.semibold)).foregroundStyle(theme.accent)
        }
        .lineLimit(1)
        .fixedSize()
        .opacity(light.power ? 1 : 0.4)
    }

    private var controls: some View {
        HStack(spacing: 8) { modeTile; automationTile; tvTile }
    }

    private var modeTile: some View {
        let isVideo = light.mode == "video"
        let value = !light.power ? "Off" : (isVideo ? "Video" : (light.mode == "color" ? "Color" : "Other"))
        return ControlTile(
            title: "Mode", value: value, symbol: isVideo ? "sparkles.tv" : "paintpalette", theme: theme,
            options: [("color", "Color", "paintpalette"), ("video", "Video", "sparkles.tv")],
            selected: light.power ? light.mode : nil
        ) { store.setMode(video: $0 == "video") }
    }

    private var automationTile: some View {
        let automation = status.automation
        let value = !automation.enabled ? "Off" : (automation.overridden ? "Paused" : "On")
        var options: [(id: String, label: String, symbol: String)] = [
            ("on", "On", "clock.arrow.circlepath"),
            ("off", "Off", "pause.circle"),
        ]
        if automation.enabled, automation.overridden {
            options.insert(("resume", "Resume now", "play.circle"), at: 0)
        }
        return ControlTile(
            title: "Automation", value: value, symbol: automation.enabled ? "clock.arrow.circlepath" : "pause.circle",
            theme: theme, options: options, selected: automation.enabled && !automation.overridden ? "on" : (automation.enabled ? nil : "off")
        ) { choice in
            switch choice {
            case "resume": store.resumeAutomation()
            case "on": automation.enabled ? store.resumeAutomation() : store.setAutomation(enabled: true)
            default: store.setAutomation(enabled: false)
            }
        }
    }

    private var tvTile: some View {
        ControlTile(title: "TV", value: status.tv.stateTitle, theme: theme) {
            Image(systemName: status.tv.symbol)
        } picker: { dismiss in
            TVInfo(tv: status.tv, accent: theme.accent) {
                dismiss()
                store.openSettings()
            }
        }
    }

    @ViewBuilder
    private var messages: some View {
        let missingVideo = status.settings.videoSource == nil
            ? "No video scene yet. Settings explains how to create one." : nil
        let texts = [store.errorMessage, light.error, status.automation.error, status.tv.error, missingVideo].compactMap { $0 }
        ForEach(Array(Set(texts)).sorted(), id: \.self) { message in
            Label(message, systemImage: "exclamationmark.triangle.fill")
                .font(.caption)
                .foregroundStyle(.orange)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private func updateBanner(_ update: UpdateChecker.Update) -> some View {
        Button(action: updates.openReleasePage) {
            HStack(spacing: 8) {
                Image(systemName: "arrow.down.circle.fill").foregroundStyle(theme.accent)
                Text("Version \(update.version) is available").font(.callout.weight(.medium))
                Spacer()
                Text("Download").font(.callout).foregroundStyle(theme.accent)
            }
            .padding(.horizontal, 12).padding(.vertical, 8)
            .background(RoundedRectangle(cornerRadius: 12, style: .continuous).fill(.white.opacity(0.08)))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .help("Opens the release page on GitHub")
    }
}

/// Small info card behind the TV tile.
private struct TVInfo: View {
    let tv: ServerStatus.TV
    let accent: Color
    let openSettings: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(tv.stateTitle, systemImage: tv.symbol).font(.headline)
            if tv.paired {
                Text(tv.active ? "The backlight follows the picture." : "Video mode starts when the TV turns on.")
                    .foregroundStyle(.secondary)
                Text(tv.host).font(.caption.monospaced()).foregroundStyle(.secondary)
            } else {
                Text("Pair your LG TV so the backlight can switch to video mode automatically.")
                    .foregroundStyle(.secondary)
                Button("Pair in Settings…", action: openSettings)
            }
        }
        .padding(14)
        .frame(width: 240, alignment: .leading)
    }
}

private struct PlaceholderView: View {
    @Environment(LightStore.self) private var store

    var body: some View {
        VStack(spacing: 12) {
            if store.needsSetup {
                Image(systemName: "server.rack").font(.largeTitle).foregroundStyle(.secondary)
                Text("Connect to your server").font(.headline)
                Text("Enter the address of the AmbientLight server and its token.")
                    .font(.caption).foregroundStyle(.secondary).multilineTextAlignment(.center)
                Button("Open Settings…", action: store.openSettings)
                    .buttonStyle(.borderedProminent)
            } else if let message = store.errorMessage ?? store.status?.light.error {
                Image(systemName: "wifi.exclamationmark").font(.largeTitle).foregroundStyle(.secondary)
                Text(message).font(.headline).multilineTextAlignment(.center)
                Button("Try Again") { Task { await store.refresh() } }
            } else {
                ProgressView()
                Text("Connecting…").foregroundStyle(.secondary)
            }
            Divider()
            HStack(spacing: 16) {
                Button("Settings…", action: store.openSettings)
                Button("Quit") { NSApplication.shared.terminate(nil) }
            }
            .buttonStyle(.link)
        }
        .padding(28)
        .frame(maxWidth: .infinity)
        .background(Theme.offBackground)
    }
}
