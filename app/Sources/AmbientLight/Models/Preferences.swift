import Foundation
import Observation
import ServiceManagement

/// User preferences stored in UserDefaults (nothing secret lives here).
@MainActor
@Observable
final class Preferences {
    private let defaults = UserDefaults.standard

    /// Daily check of the GitHub releases for a newer version.
    var checkForUpdates: Bool {
        didSet { defaults.set(checkForUpdates, forKey: "checkForUpdates") }
    }

    private(set) var launchAtLogin: Bool = false
    private(set) var launchAtLoginNeedsApproval = false

    init() {
        // Defaults only apply to keys never set before, so choices survive app updates.
        checkForUpdates = defaults.object(forKey: "checkForUpdates") as? Bool ?? true
        refreshLoginItemStatus()
    }

    // MARK: Launch at login

    /// Login items only work for a real .app bundle (not `swift run`).
    var canManageLoginItem: Bool { Bundle.main.bundleIdentifier != nil && Bundle.main.bundlePath.hasSuffix(".app") }

    func setLaunchAtLogin(_ enabled: Bool) {
        guard canManageLoginItem else { return }
        do {
            if enabled { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
        } catch {
            // Status below reflects what actually happened; nothing else to do.
        }
        refreshLoginItemStatus()
    }

    /// First launch of the bundled app: start with the Mac by default (can be turned off in Settings).
    /// Only from an Applications folder, so test runs from build/ don't register a login item for that path.
    func enableLaunchAtLoginOnFirstRun() {
        let isInstalled = Bundle.main.bundlePath.contains("/Applications/")
        guard canManageLoginItem, isInstalled, !defaults.bool(forKey: "didConfigureLoginItem") else { return }
        defaults.set(true, forKey: "didConfigureLoginItem")
        setLaunchAtLogin(true)
    }

    func refreshLoginItemStatus() {
        let status = SMAppService.mainApp.status
        launchAtLogin = status == .enabled
        launchAtLoginNeedsApproval = status == .requiresApproval
    }
}
