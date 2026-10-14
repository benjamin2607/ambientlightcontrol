import AppKit
import SwiftUI

@main
struct AmbientLightApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate

    // The UI lives in a custom status item + panel (see StatusBarController); SwiftUI only needs a scene.
    var body: some Scene {
        Settings { EmptyView() }
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var controller: StatusBarController?
    private var settingsWindow: NSWindow?
    private var store: LightStore?
    private var preferences: Preferences?
    private var updates: UpdateChecker?

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Menu bar-only app: no Dock icon, no main window.
        NSApp.setActivationPolicy(.accessory)

        let preferences = Preferences()
        self.preferences = preferences
        let store = ProcessInfo.processInfo.environment["AMBIENT_DEMO"] == "1"
            ? LightStore(demo: .demo)
            : LightStore()
        store.openSettings = { [weak self] in self?.showSettings() }
        self.store = store

        let updates = UpdateChecker(preferences: preferences)
        self.updates = updates

        controller = StatusBarController(store: store, updates: updates)
        preferences.enableLaunchAtLoginOnFirstRun()
        store.start()
        if !store.isDemo { updates.start() }

        // First run: nothing to show without a server, so go straight to the setup.
        if store.needsSetup { showSettings() }
    }

    func showSettings() {
        guard let store, let preferences, let updates else { return }
        if settingsWindow == nil {
            let root = SettingsView().environment(store).environment(preferences).environment(updates)
            let window = NSWindow(contentViewController: NSHostingController(rootView: root))
            window.title = "AmbientLight Settings"
            window.styleMask = [.titled, .closable]
            window.isReleasedWhenClosed = false
            window.layoutIfNeeded() // size from SwiftUI first, otherwise center() uses the initial frame
            window.center()
            settingsWindow = window
        }
        // Accessory apps must activate explicitly, otherwise the window opens behind others without focus.
        NSApp.activate(ignoringOtherApps: true)
        settingsWindow?.makeKeyAndOrderFront(nil)
    }
}
