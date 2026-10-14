import SwiftUI

struct SettingsView: View {
    @Environment(LightStore.self) private var store

    var body: some View {
        Form {
            ServerSection()
            if let status = store.status {
                ScheduleSection(status: status)
                TVSection(status: status)
            }
            GeneralSection()
            AboutSection()
        }
        .formStyle(.grouped)
        .frame(width: 460)
        .fixedSize(horizontal: false, vertical: true)
    }
}

private struct ServerSection: View {
    @Environment(LightStore.self) private var store
    @State private var address = ""
    @State private var token = ""
    @State private var isWorking = false
    @State private var error: String?

    var body: some View {
        Section {
            if store.needsSetup {
                TextField("Address", text: $address, prompt: Text("http://192.168.1.10:8787"))
                    .textContentType(.URL)
                SecureField("Token", text: $token)
                    .onSubmit(connect)
                if let error {
                    Label(error, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                }
                HStack {
                    Spacer()
                    if isWorking { ProgressView().controlSize(.small) }
                    Button("Connect", action: connect)
                        .keyboardShortcut(.defaultAction)
                        .disabled(address.isEmpty || token.isEmpty || isWorking)
                }
            } else {
                LabeledContent("Connected to", value: store.serverURL?.absoluteString ?? "–")
                if let version = store.status?.version {
                    LabeledContent("Server version", value: version)
                }
                HStack {
                    Spacer()
                    Button("Disconnect", role: .destructive, action: store.disconnect)
                }
            }
        } header: {
            Text("Server")
        } footer: {
            if store.needsSetup {
                Text("The token is created on the server's first start, in the file api-token of its config folder (on Unraid: /mnt/user/appdata/ambientlightcontrol/api-token). It is stored in your Keychain.")
                    .foregroundStyle(.secondary)
            }
        }
    }

    private func connect() {
        guard !address.isEmpty, !token.isEmpty else { return }
        isWorking = true
        error = nil
        Task {
            do {
                try await store.connect(urlString: address, token: token)
                token = ""
            } catch {
                self.error = (error as? LocalizedError)?.errorDescription ?? "Connection failed."
            }
            isWorking = false
        }
    }
}

private struct ScheduleSection: View {
    @Environment(LightStore.self) private var store
    let status: ServerStatus

    var body: some View {
        Section {
            Toggle("Automation", isOn: Binding(
                get: { status.settings.automationEnabled },
                set: { store.setAutomation(enabled: $0) }
            ))
            timePicker("Light on at", value: status.settings.onTime) { store.updateSettings(SettingsChange(onTime: $0)) }
            timePicker("Light off at", value: status.settings.offTime) { store.updateSettings(SettingsChange(offTime: $0)) }
            BrightnessSetting(title: "Daytime brightness", value: status.settings.brightness) {
                store.updateSettings(SettingsChange(brightness: $0))
            }
            BrightnessSetting(title: "Video brightness", value: status.settings.videoBrightness) {
                store.updateSettings(SettingsChange(videoBrightness: $0))
            }
            videoSourcePicker
        } header: {
            Text("Automation")
        } footer: {
            Text("Between these times the backlight shows the daytime color. When the TV is on, it follows the picture, also after the off time. Choose the daytime color in the menu bar popover. Changing the brightness there also updates the level for the current mode.")
                .foregroundStyle(.secondary)
        }
    }

    private func timePicker(_ title: String, value: String, onChange: @escaping (String) -> Void) -> some View {
        DatePicker(title, selection: Binding(
            get: { TimeOfDay.date(from: value) },
            set: { onChange(TimeOfDay.string(from: $0)) }
        ), displayedComponents: .hourAndMinute)
    }

    @ViewBuilder
    private var videoSourcePicker: some View {
        if status.videoSources.isEmpty {
            LabeledContent("Video scene") {
                Text("None found").foregroundStyle(.secondary)
            }
            Text("The Govee API can't start Video mode directly. In the Govee app, switch the backlight to Video mode and save it as a snapshot called \"Video\". It is picked up within a few minutes.")
                .font(.caption).foregroundStyle(.secondary)
        } else {
            Picker("Video scene", selection: Binding(
                get: { status.settings.videoSource },
                set: { if let ref = $0 { store.updateSettings(SettingsChange(videoSource: ref)) } }
            )) {
                if status.settings.videoSource == nil { Text("Choose…").tag(ServerStatus.SceneRef?.none) }
                ForEach(status.videoSources) { ref in
                    Text("\(ref.name) (\(ref.kindTitle))").tag(Optional(ref))
                }
            }
        }
    }
}

/// Slider that only sends its value when released.
private struct BrightnessSetting: View {
    let title: String
    let value: Int
    let onCommit: (Int) -> Void
    @State private var draft: Double?

    var body: some View {
        LabeledContent(title) {
            HStack {
                Slider(value: Binding(get: { draft ?? Double(value) }, set: { draft = $0 }), in: 1...100, step: 1) { editing in
                    if !editing, let draft {
                        onCommit(Int(draft))
                        self.draft = nil
                    }
                }
                .frame(width: 180)
                Text("\(Int(draft ?? Double(value))) %").monospacedDigit().frame(width: 44, alignment: .trailing)
            }
        }
    }
}

private struct TVSection: View {
    @Environment(LightStore.self) private var store
    let status: ServerStatus
    @State private var host = ""
    @State private var error: String?

    var body: some View {
        Section {
            TextField("TV address", text: $host, prompt: Text("192.168.1.20"))
            LabeledContent("Status") {
                Label(status.tv.stateTitle, systemImage: status.tv.symbol)
            }
            if let error {
                Label(error, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
            }
            HStack {
                if store.isPairing {
                    ProgressView().controlSize(.small)
                    Text("Accept the prompt on your TV…").foregroundStyle(.secondary)
                }
                Spacer()
                Button(status.tv.paired ? "Pair Again" : "Pair TV", action: pair)
                    .disabled(host.trimmingCharacters(in: .whitespaces).isEmpty || store.isPairing)
            }
        } header: {
            Text("LG TV")
        } footer: {
            Text("The TV must be switched on. It asks once to allow the connection. Only the power state is read. To keep detection fast, turn on Settings › General › Devices › TV Management › Quick Start+ on the TV.")
                .foregroundStyle(.secondary)
        }
        .onAppear { host = status.tv.host }
    }

    private func pair() {
        error = nil
        Task {
            do {
                try await store.pairTV(host: host)
            } catch {
                self.error = (error as? LocalizedError)?.errorDescription ?? "Pairing failed."
            }
        }
    }
}

private struct GeneralSection: View {
    @Environment(LightStore.self) private var store
    @Environment(Preferences.self) private var preferences

    var body: some View {
        Section("General") {
            Toggle("Launch at login", isOn: Binding(
                get: { preferences.launchAtLogin },
                set: { preferences.setLaunchAtLogin($0) }
            ))
            .disabled(!preferences.canManageLoginItem)
            if !preferences.canManageLoginItem {
                Text("Available when running the installed AmbientLight.app.")
                    .font(.caption).foregroundStyle(.secondary)
            } else if preferences.launchAtLoginNeedsApproval {
                Text("Approve AmbientLight in System Settings › General › Login Items.")
                    .font(.caption).foregroundStyle(.orange)
            }
        }
        .onAppear { preferences.refreshLoginItemStatus() }
    }
}

private struct AboutSection: View {
    @Environment(Preferences.self) private var preferences
    @Environment(UpdateChecker.self) private var updates

    var body: some View {
        Section("About") {
            LabeledContent("Version", value: updates.currentVersion)
            Toggle("Check for updates daily", isOn: Binding(
                get: { preferences.checkForUpdates },
                set: { preferences.checkForUpdates = $0 }
            ))
            HStack {
                if let update = updates.available {
                    Label("Version \(update.version) is available", systemImage: "arrow.down.circle.fill")
                        .foregroundStyle(.orange)
                    Spacer()
                    Button("Download…", action: updates.openReleasePage)
                } else {
                    Text(updates.lastChecked.map { "Up to date · checked \($0.formatted(.relative(presentation: .named)))" }
                         ?? "Not checked yet")
                        .foregroundStyle(.secondary)
                    Spacer()
                    if updates.isChecking { ProgressView().controlSize(.small) }
                    Button("Check Now") { Task { await updates.check() } }
                        .disabled(updates.isChecking)
                }
            }
            Link("Source code on GitHub", destination: UpdateChecker.repository)
            Text("Update checks contact api.github.com once a day. Nothing is downloaded or installed automatically.")
                .font(.caption).foregroundStyle(.secondary)
            Text("Not affiliated with or endorsed by Govee or LG Electronics. Govee is a trademark of Shenzhen Intellirocks Tech. Co., Ltd.; LG and webOS are trademarks of LG Electronics Inc.")
                .font(.caption).foregroundStyle(.secondary)
        }
    }
}

/// "HH:MM" strings (as the server stores them) ⇄ Date for the time pickers.
enum TimeOfDay {
    static func date(from string: String) -> Date {
        let parts = string.split(separator: ":").compactMap { Int($0) }
        guard parts.count == 2 else { return Calendar.current.startOfDay(for: .now) }
        return Calendar.current.date(bySettingHour: parts[0], minute: parts[1], second: 0, of: .now) ?? .now
    }

    static func string(from date: Date) -> String {
        let c = Calendar.current.dateComponents([.hour, .minute], from: date)
        return String(format: "%02d:%02d", c.hour ?? 0, c.minute ?? 0)
    }
}
