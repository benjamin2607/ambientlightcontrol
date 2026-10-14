# AmbientLight

A native macOS menu bar app and a small automation server for the **Govee TV Backlight 3 Lite** (H6099)
and other Govee lights. Pick a colour and brightness from a compact popover. The server takes care of the
daily routine:

- **07:00:** the backlight turns on in your daytime colour (50 % brightness by default)
- **TV on:** the backlight switches to **Video mode** and follows the picture (LG webOS TVs, 100 % by default)
- **TV off:** back to the daytime colour
- **22:00:** off, unless the TV is still on; then the light turns off when the TV does

Times are adjustable. A manual change from the popover pauses the automation until the next event (TV on/off
or the next scheduled time).

- **Server:** a single Go binary in a ~10 MB Docker image with no dependencies, so it runs on a NAS (Unraid template
  included) and keeps working while your Mac sleeps
- **App:** native SwiftUI with no dependencies. The popover is tinted with the colour your backlight shows
- **Keys stay where they belong:** the Govee API key lives only on the server. The Mac keeps just the server
  token in its Keychain

> **Unofficial.** Not affiliated with or endorsed by Govee or LG Electronics. The app uses the public Govee
> Developer API and the local webOS API of the TV, both of which may change.

```
macOS app ──HTTP + token (LAN)──▶ server (Docker) ──HTTPS──▶ Govee Developer API ──▶ backlight
                                        │
                                        └──TLS WebSocket (LAN)──▶ LG TV (power state only)
```

## Requirements

- A Govee light and a **Govee API key**: Govee Home app › Profile › Settings › Apply for API Key
- An **LG webOS TV** (2018 or newer) for the TV detection. Without one, the schedule still works
- A Docker host that is always on (Unraid, Synology, Raspberry Pi …)
- macOS 14 Sonoma or later for the app. To build it: Swift 5.10+ (Xcode Command Line Tools are enough)

## Setup

### 1. Video mode snapshot (once, in the Govee app)

The Govee API can't start Video mode directly, but it can recall a **snapshot**. In the Govee Home app,
switch the backlight to **Video** mode with the settings you like and save it as a snapshot called **Video**.
The server picks it up automatically. A snapshot with another name can be chosen in the app's Settings.

### 2. Server on Unraid

Give the TV a fixed IP address in your router first. On the TV, turn on **Quick Start+** (Settings › General ›
Devices › TV Management). The TV then stays reachable in standby, which makes detection faster and more reliable.

1. Open the Unraid terminal (**>_** icon at the top right) and run these two commands. They create the config
   folder and add the container template:
   ```bash
   mkdir -p /mnt/user/appdata/ambientlightcontrol && chown 99:100 /mnt/user/appdata/ambientlightcontrol
   ```
   ```bash
   wget -qO /boot/config/plugins/dockerMan/templates-user/my-ambientlightcontrol.xml https://raw.githubusercontent.com/benjamin2607/ambientlightcontrol/main/deploy/unraid-template.xml
   ```
2. Go to **Docker › Add Container**, choose **ambientlightcontrol** under *Template*.
3. Enter your **Govee API key**, the **IP address of your TV** and your time zone, then click **Apply**.
4. The server creates an access token on its first start. Show it in the Unraid terminal with:
   ```bash
   cat /mnt/user/appdata/ambientlightcontrol/api-token
   ```

The image is `ghcr.io/benjamin2607/ambientlightcontrol` (amd64 and arm64). Updates: **Docker › Check for
Updates** in Unraid. Other Docker hosts: see [deploy/docker-compose.yml](deploy/docker-compose.yml).

### 3. macOS app

1. Download **AmbientLight.zip** from the [latest release](https://github.com/benjamin2607/ambientlightcontrol/releases/latest),
   unzip it and move **AmbientLight.app** to *Applications*.
2. The app is ad-hoc signed but **not notarized** by Apple, so macOS blocks the first launch. Open it once,
   then go to **System Settings › Privacy & Security** and click **Open Anyway**. Optionally compare the download
   with the published SHA-256 checksum first:
   ```bash
   shasum -a 256 ~/Downloads/AmbientLight.zip
   ```
3. Settings open on the first start:
   1. **Server:** enter the address of your NAS with port 8787 (e.g. `http://192.168.1.10:8787`) and the token.
   2. **LG TV:** switch the TV on, click **Pair TV** and accept the prompt on the TV with the remote.
   3. **Automation:** check the times, brightness levels and the video scene.

The app starts with your Mac and shows a notice when a new version is released.

To build it yourself instead (Xcode Command Line Tools are enough):

```bash
git clone https://github.com/benjamin2607/ambientlightcontrol.git
cd ambientlightcontrol
scripts/install.sh
```

Afterwards:

| Action | How |
|---|---|
| Open the popover | Click the menu bar icon |
| Daytime colour | Pick a swatch or the rainbow button in the popover. It is applied now and used by the automation |
| Video mode now | Mode tile › Video |
| Pause or resume the automation | Automation tile |
| Settings, Quit | Right-click the menu bar icon, or <kbd>⌘</kbd><kbd>,</kbd> while the popover is open |
| Close the popover | Click elsewhere or press <kbd>Esc</kbd> |

## How it works

- **The server** checks every 5 seconds what the light should do. It sends a command to Govee only when that
  changes, and reads the light state once a minute. This stays far below Govee's daily request limit.
- **TV detection** keeps a TLS WebSocket open to the TV and subscribes to its power state. Only *Active*
  (picture on) counts as "TV in use". Screen saver, screen off and standby don't count. After the TV turns
  off, the server waits 20 seconds before switching back, so input changes don't cause flicker.
- **The app** polls the server every 3 seconds while the popover is open and every 30 seconds otherwise.
  Slider and colour changes are bundled into one request.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for details and the API.

## Development

```bash
scripts/run-server.sh         # server on 127.0.0.1:8787, reads GOVEE_API_KEY from .env, state in .data/
scripts/run-app.sh --demo     # app with sample data, no server needed
scripts/run-app.sh            # app against a real server
scripts/test.sh               # Go and Swift tests
scripts/build-app.sh          # build/AmbientLight.app
docker build -t ambientlightcontrol server
```

- Create `.env` from [.env.example](.env.example). It is ignored by git.
- `AMBIENT_OPEN_ON_LAUNCH=1` opens the popover right after launch, which is handy for layout work.
- App icon: `scripts/make-icon.sh` regenerates `app/Resources/AppIcon.icns`.

## Security

See [SECURITY.md](SECURITY.md) for how keys and tokens are handled and how to report a vulnerability.

## License

[MIT](LICENSE). See [NOTICE](NOTICE) for trademarks.
