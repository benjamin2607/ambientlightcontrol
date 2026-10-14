import Foundation
import Testing
@testable import AmbientLight

@Suite struct ServerURLValidation {
    @Test(arguments: [
        "192.168.1.10:8787",
        "http://192.168.1.10:8787",
        "http://10.0.0.5:8787/",
        "http://tower.local:8787",
        "http://tower:8787",
        "http://172.20.0.2:8787",
        "http://100.101.102.103:8787",
        "https://light.example.com",
    ])
    func accepts(_ input: String) throws {
        _ = try ServerAPI.validatedURL(input)
    }

    @Test(arguments: [
        "http://light.example.com",  // token would travel unencrypted over the internet
        "http://8.8.8.8:8787",
        "http://172.32.0.1:8787",
        "ftp://192.168.1.10",
        "http://user:pass@192.168.1.10",
        "",
    ])
    func rejects(_ input: String) {
        #expect(throws: ServerAPI.APIError.self) { try ServerAPI.validatedURL(input) }
    }

    @Test func addsSchemeAndDropsTrailingSlash() throws {
        let url = try ServerAPI.validatedURL(" 192.168.1.10:8787/ ")
        #expect(url.absoluteString == "http://192.168.1.10:8787")
    }
}

@Suite struct StatusDecoding {
    static let json = """
    {
      "light": {"ready": true, "name": "TV Backlight 3 Lite", "sku": "H6099", "online": true, "power": true,
                "brightness": 70, "color": 16756303, "mode": "video"},
      "tv": {"host": "192.168.1.20", "paired": true, "state": "on", "active": true},
      "automation": {"enabled": true, "overridden": false, "target": "video", "reason": "TV is on",
                     "nextChange": "2026-10-14T22:00:00+02:00"},
      "settings": {"automationEnabled": true, "onTime": "07:00", "offTime": "22:00", "color": 16756303,
                   "brightness": 70, "videoBrightness": 100, "videoSource": {"kind": "snapshot", "name": "Video"}, "tvHost": "192.168.1.20"},
      "videoSources": [{"kind": "snapshot", "name": "Video"}],
      "version": "1.0.0"
    }
    """

    @Test func decodesServerResponse() throws {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let status = try decoder.decode(ServerStatus.self, from: Data(Self.json.utf8))
        #expect(status.light.isVideo)
        #expect(status.tv.stateTitle == "On")
        #expect(status.automation.nextChange != nil)
        #expect(status.settings.videoSource?.name == "Video")
        #expect(status.automationSummary == "Auto · TV is on")
    }
}

@Suite struct LightChanges {
    @Test func laterValuesWin() {
        let merged = LightChange(brightness: 20).merged(with: LightChange(brightness: 60))
        #expect(merged == LightChange(brightness: 60))
    }

    @Test func colorAndBrightnessCombine() {
        let merged = LightChange(color: 0xFF0000).merged(with: LightChange(brightness: 40))
        #expect(merged == LightChange(brightness: 40, color: 0xFF0000))
    }

    @Test func powerChangeReplacesEverything() {
        let merged = LightChange(color: 0xFF0000).merged(with: LightChange(power: false))
        #expect(merged == LightChange(power: false))
    }

    @Test func videoDropsPendingColor() {
        let merged = LightChange(color: 0xFF0000).merged(with: LightChange(mode: "video"))
        #expect(merged == LightChange(mode: "video"))
    }

    @Test func colorAfterVideoMeansColorMode() {
        let merged = LightChange(mode: "video").merged(with: LightChange(color: 0x00FF00))
        #expect(merged == LightChange(color: 0x00FF00))
    }

    @MainActor @Test func optimisticColorBecomesDaytimeColor() {
        var status = ServerStatus.demo
        status.light.power = false
        LightStore.applyOptimistically(LightChange(color: 0x123456), to: &status)
        #expect(status.light.power)
        #expect(status.light.mode == "color")
        #expect(status.settings.color == 0x123456)
    }
}

@Suite struct Brightness {
    @MainActor @Test func eachModeKeepsItsOwnLevel() {
        var status = ServerStatus.demo
        status.settings.brightness = 50
        status.settings.videoBrightness = 100
        LightStore.applyOptimistically(LightChange(mode: "video"), to: &status)
        #expect(status.light.brightness == 100)
        LightStore.applyOptimistically(LightChange(brightness: 80), to: &status)
        #expect(status.settings.videoBrightness == 80)
        #expect(status.settings.brightness == 50)
        LightStore.applyOptimistically(LightChange(mode: "color"), to: &status)
        #expect(status.light.brightness == 50)
    }
}

@Suite struct Colors {
    @Test func hexRoundTrip() {
        for hex in [0x000000, 0xFFFFFF, 0xFF9A3C, 0x123456] {
            #expect(RGB(hex: hex).hex == hex)
        }
    }

    @Test func vividKeepsHue() {
        let vivid = RGB(hex: 0x402010).vivid
        #expect(vivid.r == 1)
        #expect(abs(vivid.g - 0.5) < 0.01)
    }

    @Test func timeOfDayRoundTrip() {
        #expect(TimeOfDay.string(from: TimeOfDay.date(from: "07:05")) == "07:05")
        #expect(TimeOfDay.string(from: TimeOfDay.date(from: "22:00")) == "22:00")
    }
}
