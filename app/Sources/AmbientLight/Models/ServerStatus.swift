import Foundation

/// Snapshot of the server state (`GET /api/v1/status`).
struct ServerStatus: Codable, Equatable {
    var light: Light
    var tv: TV
    var automation: Automation
    var settings: Settings
    var videoSources: [SceneRef]
    var version: String

    struct Light: Codable, Equatable {
        var ready: Bool
        var name: String
        var sku: String
        var online: Bool
        var power: Bool
        var brightness: Int
        var color: Int
        /// off, color, video or unknown (changed in the Govee app).
        var mode: String
        var error: String?

        var isVideo: Bool { power && mode == "video" }
    }

    struct TV: Codable, Equatable {
        var host: String
        var paired: Bool
        var state: String
        var active: Bool
        var error: String?

        var stateTitle: String {
            switch state {
            case "on": "On"
            case "idle": "Screen saver"
            case "screen_off": "Screen off"
            case "standby": "Standby"
            case "off": "Off"
            default: paired ? "Not connected" : "Not paired"
            }
        }

        var symbol: String {
            switch state {
            case "on": "tv.inset.filled"
            case "unpaired": "tv.slash"
            default: "tv"
            }
        }
    }

    struct Automation: Codable, Equatable {
        var enabled: Bool
        var overridden: Bool
        var target: String
        var reason: String
        var nextChange: Date?
        var error: String?
    }

    struct Settings: Codable, Equatable {
        var automationEnabled: Bool
        var onTime: String
        var offTime: String
        var color: Int
        /// Brightness with the daytime colour and in video mode; the popover slider updates the current one.
        var brightness: Int
        var videoBrightness: Int
        var videoSource: SceneRef?
        var tvHost: String
    }

    struct SceneRef: Codable, Hashable, Identifiable {
        var kind: String
        var name: String

        var id: String { kind + "/" + name }
        var kindTitle: String { kind == "snapshot" ? "Snapshot" : "DIY scene" }
    }
}

extension ServerStatus {
    /// One line for the popover header.
    var automationSummary: String {
        if !automation.enabled { return "Automation off" }
        if automation.overridden { return "Manual · auto resumes on next change" }
        return "Auto · " + automation.reason
    }

    static let demo = ServerStatus(
        light: Light(ready: true, name: "TV Backlight 3 Lite", sku: "H6099", online: true, power: true,
                     brightness: 70, color: 0xFF9A3C, mode: "color"),
        tv: TV(host: "192.168.1.20", paired: true, state: "standby", active: false),
        automation: Automation(enabled: true, overridden: false, target: "color", reason: "Daytime 07:00–22:00",
                               nextChange: Calendar.current.date(bySettingHour: 22, minute: 0, second: 0, of: .now)),
        settings: Settings(automationEnabled: true, onTime: "07:00", offTime: "22:00", color: 0xFF9A3C,
                           brightness: 70, videoBrightness: 100, videoSource: SceneRef(kind: "snapshot", name: "Video"), tvHost: "192.168.1.20"),
        videoSources: [SceneRef(kind: "snapshot", name: "Video"), SceneRef(kind: "diyScene", name: "Cozy")],
        version: "demo"
    )
}

/// Body of `POST /api/v1/light`. Nil fields stay unchanged.
struct LightChange: Encodable, Equatable {
    var power: Bool?
    var brightness: Int?
    var color: Int?
    var mode: String?

    var isEmpty: Bool { power == nil && brightness == nil && color == nil && mode == nil }

    /// Later values win; a power change resets the others (on/off is its own action).
    func merged(with other: LightChange) -> LightChange {
        if other.power != nil { return other }
        var result = self
        if result.power == false { result.power = nil }
        result.brightness = other.brightness ?? result.brightness
        result.color = other.color ?? result.color
        result.mode = other.mode ?? result.mode
        if other.color != nil, other.mode == nil { result.mode = nil }
        if other.mode == "video" { result.color = nil }
        return result
    }
}

/// Body of `PATCH /api/v1/settings`.
struct SettingsChange: Encodable, Equatable {
    var automationEnabled: Bool?
    var onTime: String?
    var offTime: String?
    var brightness: Int?
    var videoBrightness: Int?
    var videoSource: ServerStatus.SceneRef?
    var tvHost: String?
}
