import AppKit
import Foundation
import Observation
import UserNotifications

/// Looks for a newer release on GitHub once a day. It only informs the user and links to the
/// release page; nothing is downloaded or installed automatically.
@MainActor
@Observable
final class UpdateChecker: NSObject {
    struct Update: Equatable {
        let version: String
        let url: URL
    }

    static let repository = URL(string: "https://github.com/benjamin2607/ambientlightcontrol")!
    static let releasesPage = repository.appending(path: "releases")
    private static let latestReleaseAPI = URL(
        string: "https://api.github.com/repos/benjamin2607/ambientlightcontrol/releases/latest"
    )!
    private static let checkInterval: TimeInterval = 24 * 60 * 60

    private(set) var available: Update?
    private(set) var lastChecked: Date?
    private(set) var isChecking = false

    private let preferences: Preferences
    private let defaults = UserDefaults.standard
    private var timer: Timer?

    let currentVersion = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0"

    init(preferences: Preferences) {
        self.preferences = preferences
        self.lastChecked = defaults.object(forKey: "lastUpdateCheck") as? Date
        super.init()
        if Self.canNotify { UNUserNotificationCenter.current().delegate = self }
    }

    /// UNUserNotificationCenter traps outside an app bundle (e.g. `swift run`).
    private static var canNotify: Bool { Bundle.main.bundleIdentifier != nil }

    func start() {
        // Hourly tick; the actual request only happens once per day.
        timer = Timer.scheduledTimer(withTimeInterval: 60 * 60, repeats: true) { [weak self] _ in
            Task { @MainActor in await self?.checkIfDue() }
        }
        Task {
            try? await Task.sleep(for: .seconds(10)) // don't compete with the first server refresh
            await checkIfDue()
        }
    }

    func checkIfDue() async {
        guard preferences.checkForUpdates else { return }
        if let lastChecked, Date().timeIntervalSince(lastChecked) < Self.checkInterval { return }
        await check()
    }

    /// Manual check from Settings, or the daily one.
    func check() async {
        guard !isChecking else { return }
        isChecking = true
        defer { isChecking = false }

        var request = URLRequest(url: Self.latestReleaseAPI, timeoutInterval: 15)
        request.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
        request.setValue("AmbientLight/\(currentVersion)", forHTTPHeaderField: "User-Agent")
        let session = URLSession(configuration: .ephemeral)
        defer { session.finishTasksAndInvalidate() }

        // Offline, rate-limited or no release yet: try again at the next interval.
        guard let (data, response) = try? await session.data(for: request),
              (response as? HTTPURLResponse)?.statusCode == 200,
              let release = try? JSONDecoder().decode(Release.self, from: data),
              !release.draft, !release.prerelease else { return }

        lastChecked = Date()
        defaults.set(lastChecked, forKey: "lastUpdateCheck")

        let version = release.tagName.trimmingCharacters(in: CharacterSet(charactersIn: "vV"))
        guard Self.isVersion(version, newerThan: currentVersion),
              let url = URL(string: release.htmlURL), url.host == "github.com" else {
            available = nil
            return
        }
        available = Update(version: version, url: url)
        notifyOnce(about: available!)
    }

    func openReleasePage() {
        NSWorkspace.shared.open(available?.url ?? Self.releasesPage)
    }

    /// Numeric comparison of dotted versions ("1.10.0" > "1.9.2").
    nonisolated static func isVersion(_ candidate: String, newerThan current: String) -> Bool {
        let a = candidate.split(separator: ".").map { Int($0) ?? 0 }
        let b = current.split(separator: ".").map { Int($0) ?? 0 }
        for index in 0..<max(a.count, b.count) {
            let x = index < a.count ? a[index] : 0
            let y = index < b.count ? b[index] : 0
            if x != y { return x > y }
        }
        return false
    }

    // MARK: Notification

    private func notifyOnce(about update: Update) {
        guard Self.canNotify, defaults.string(forKey: "notifiedUpdateVersion") != update.version else { return }
        defaults.set(update.version, forKey: "notifiedUpdateVersion")
        let center = UNUserNotificationCenter.current()
        // Permission is only requested once an update actually exists.
        center.requestAuthorization(options: [.alert]) { granted, _ in
            guard granted else { return }
            let content = UNMutableNotificationContent()
            content.title = "AmbientLight \(update.version) is available"
            content.body = "Click to open the release page."
            content.userInfo = ["url": update.url.absoluteString]
            center.add(UNNotificationRequest(identifier: "update-\(update.version)", content: content, trigger: nil))
        }
    }

    private struct Release: Decodable {
        let tagName: String
        let htmlURL: String
        let draft: Bool
        let prerelease: Bool

        enum CodingKeys: String, CodingKey {
            case tagName = "tag_name", htmlURL = "html_url", draft, prerelease
        }
    }
}

extension UpdateChecker: UNUserNotificationCenterDelegate {
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        if let string = response.notification.request.content.userInfo["url"] as? String,
           let url = URL(string: string), url.host == "github.com" {
            DispatchQueue.main.async { NSWorkspace.shared.open(url) }
        }
        completionHandler()
    }

    /// Show the banner even though a menu bar app is technically always "in the foreground".
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner])
    }
}
