import Foundation
import Observation

/// Single source of truth for the UI: polls the server and sends debounced, optimistic updates.
@MainActor
@Observable
final class LightStore {
    private(set) var status: ServerStatus?
    private(set) var errorMessage: String?
    private(set) var isSending = false
    private(set) var needsSetup = false
    private(set) var isPairing = false
    private(set) var serverURL: URL?

    /// Set by the app shell; opens the Settings window (server setup lives there).
    var openSettings: () -> Void = {}

    private let credentials = CredentialStore()
    private var api: ServerAPI?
    private let isDemoMode: Bool
    private let debounce: Duration
    private var pollTask: Task<Void, Never>?
    private var debounceTask: Task<Void, Never>?
    private var pending = LightChange()
    private var isInteracting = false
    /// Faster polling while the popover is open, so TV and automation changes show up quickly.
    private var isVisible = false

    init(debounce: Duration = .milliseconds(400)) {
        self.isDemoMode = false
        self.debounce = debounce
        if let url = credentials.serverURL, let token = credentials.token() {
            api = ServerAPI(baseURL: url, token: token)
            serverURL = url
        } else {
            needsSetup = true
        }
    }

    /// Offline mode for design work: `AMBIENT_DEMO=1`.
    init(demo: ServerStatus) {
        self.isDemoMode = true
        self.debounce = .zero
        self.status = demo
        self.serverURL = URL(string: "http://192.168.1.10:8787")
    }

    var isDemo: Bool { isDemoMode }

    func start() {
        guard pollTask == nil, !isDemo else { return }
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                await self?.refresh()
                let interval: Duration = self?.isVisible == true ? .seconds(3) : .seconds(30)
                try? await Task.sleep(for: interval)
            }
        }
    }

    func setVisible(_ visible: Bool) {
        isVisible = visible
        if visible { Task { await refresh() } }
    }

    func refresh() async {
        guard let api, !hasPendingWork else { return }
        do {
            let fresh = try await api.status()
            if !hasPendingWork { status = fresh }
            errorMessage = nil
        } catch {
            handle(error)
        }
    }

    // MARK: Server connection

    /// Checks the address and token against the server before storing them.
    func connect(urlString: String, token: String) async throws {
        let url = try ServerAPI.validatedURL(urlString)
        let token = token.trimmingCharacters(in: .whitespacesAndNewlines)
        let candidate = ServerAPI(baseURL: url, token: token)
        let fresh = try await candidate.status()
        guard credentials.save(url: url, token: token) else {
            throw ServerAPI.APIError.server("The token could not be saved in the Keychain.")
        }
        api = candidate
        serverURL = url
        status = fresh
        needsSetup = false
        errorMessage = nil
    }

    func disconnect() {
        credentials.clear()
        debounceTask?.cancel()
        debounceTask = nil
        pending = LightChange()
        api = nil
        serverURL = nil
        status = nil
        needsSetup = true
    }

    // MARK: Light

    func togglePower() { send(LightChange(power: !(status?.light.power ?? false))) }
    func setBrightness(_ value: Int) { send(LightChange(brightness: value)) }
    func setColor(_ rgb: Int) { send(LightChange(color: rgb)) }
    func setMode(video: Bool) { send(LightChange(mode: video ? "video" : "color")) }

    /// While the slider is held, values only change locally; one request goes out on release.
    func setInteracting(_ active: Bool) {
        isInteracting = active
        if active {
            debounceTask?.cancel()
            debounceTask = nil
        } else {
            scheduleFlush()
        }
    }

    private func send(_ change: LightChange) {
        guard var current = status else { return }
        Self.applyOptimistically(change, to: &current)
        status = current
        guard !isDemo else { return }
        pending = pending.merged(with: change)
        scheduleFlush()
    }

    /// What the UI shows before the server confirms.
    static func applyOptimistically(_ change: LightChange, to status: inout ServerStatus) {
        if let power = change.power {
            status.light.power = power
            if !power { status.light.mode = "off" }
        }
        if let brightness = change.brightness {
            status.light.brightness = brightness
            status.light.power = true
            switch status.light.mode {
            case "color": status.settings.brightness = brightness
            case "video": status.settings.videoBrightness = brightness
            default: break
            }
        }
        if let color = change.color {
            if status.light.mode != "color", change.brightness == nil { status.light.brightness = status.settings.brightness }
            status.light.color = color
            status.light.power = true
            status.light.mode = "color"
            status.settings.color = color
        }
        if let mode = change.mode {
            status.light.power = true
            status.light.mode = mode
            if mode == "color" {
                status.light.color = status.settings.color
                status.light.brightness = change.brightness ?? status.settings.brightness
            } else if mode == "video" {
                status.light.brightness = change.brightness ?? status.settings.videoBrightness
            }
        }
    }

    private var hasPendingWork: Bool {
        isInteracting || isSending || debounceTask != nil || !pending.isEmpty
    }

    private func scheduleFlush() {
        debounceTask?.cancel()
        guard !isInteracting, !pending.isEmpty else {
            debounceTask = nil
            return
        }
        debounceTask = Task { [weak self, debounce] in
            try? await Task.sleep(for: debounce)
            guard !Task.isCancelled, let self else { return }
            self.debounceTask = nil
            await self.flush()
        }
    }

    /// One request at a time; changes made meanwhile follow afterwards.
    private func flush() async {
        guard let api, !isSending, !pending.isEmpty else { return }
        let change = pending
        pending = LightChange()
        isSending = true
        do {
            let fresh = try await api.change(change)
            if pending.isEmpty, !isInteracting, debounceTask == nil { status = fresh }
            errorMessage = nil
        } catch {
            handle(error)
            isSending = false
            await refresh()
            return
        }
        isSending = false
        if !pending.isEmpty { scheduleFlush() }
    }

    // MARK: Automation and settings

    func setAutomation(enabled: Bool) {
        status?.automation.enabled = enabled
        status?.settings.automationEnabled = enabled
        perform { try await $0.update(SettingsChange(automationEnabled: enabled)) }
    }

    func resumeAutomation() {
        status?.automation.overridden = false
        perform { try await $0.resumeAutomation() }
    }

    func updateSettings(_ change: SettingsChange) {
        perform { try await $0.update(change) }
    }

    /// Asks the server to pair with the TV; the user has to accept the prompt on the TV.
    func pairTV(host: String) async throws {
        guard let api else { return }
        isPairing = true
        defer { isPairing = false }
        status = try await api.pairTV(host: host.trimmingCharacters(in: .whitespaces))
        errorMessage = nil
    }

    private func perform(_ call: @escaping (ServerAPI) async throws -> ServerStatus) {
        guard let api, !isDemo else { return }
        Task {
            do {
                status = try await call(api)
                errorMessage = nil
            } catch {
                handle(error)
                await refresh()
            }
        }
    }

    private func handle(_ error: Error) {
        errorMessage = (error as? LocalizedError)?.errorDescription ?? "Something went wrong."
    }
}
