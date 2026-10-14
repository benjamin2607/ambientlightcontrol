# Security

## How AmbientLight handles your data

| Data | Where it is kept |
|---|---|
| Govee API key | **Server only**, as the environment variable `GOVEE_API_KEY` or a file via `GOVEE_API_KEY_FILE` (Docker secret). Sent only to `openapi.api.govee.com` over HTTPS. Never logged, never sent to the app. |
| Server API token | Server: `/config/api-token` (mode `0600`), generated with 256 bits of randomness on first start, or set via `API_TOKEN`/`API_TOKEN_FILE`. Mac: Keychain item `io.github.benjamin2607.ambientlight.server`, *after first unlock, this device only* (not synced via iCloud). |
| TV pairing key and TLS certificate fingerprint | Server: `/config/state.json` (mode `0600`). |
| Settings (times, colour, TV address) | Server: `/config/state.json`. Mac: only the server address in UserDefaults. |

### Server

- Every API endpoint except `GET /healthz` requires `Authorization: Bearer <token>`. Tokens are compared
  as SHA-256 digests in constant time. After 10 wrong tokens an address is blocked for 5 minutes.
- Request bodies are limited to 16 KB, unknown JSON fields are rejected and all values are range-checked.
  Server timeouts guard against slow clients.
- The API is plain HTTP and meant for your **local network only**. Don't forward the port to the internet.
  For remote access use a VPN (WireGuard, Tailscale), or put a reverse proxy with HTTPS in front.
- **LG TV:** the server requests only the `READ_POWER_STATE` permission. Older webOS versions grant it only
  to a manifest with LG's signed block, so the publicly known signed block of LG's test remote app is sent
  along (as Home Assistant and PyWebOSTV do). The TV uses a self-signed
  certificate, so its fingerprint is pinned at pairing (trust on first use). If it changes, for example after a
  firmware update or because another device answers on that address, the server stops and asks you to pair again.
- **Container:** a `scratch` image without shell or package manager, running as `99:100` (non-root). The
  Unraid template and compose file add `--read-only`, `--cap-drop=ALL` and `no-new-privileges`.
- **No third-party Go modules.** The WebSocket client for the TV is part of this repository.
- Logs contain no keys or tokens. Failed authentication attempts are logged with the client address.
- **Unraid:** template values, including the Govee API key, are stored in plain text on the USB flash drive.
  If that concerns you, put the key in a file in the appdata folder (`chmod 600`) and set
  `GOVEE_API_KEY_FILE=/config/govee-api-key` instead of `GOVEE_API_KEY`.

### macOS app

- Talks only to your server and, once a day for the update check, to `https://api.github.com`. This can be
  turned off in Settings › About.
- Plain HTTP is only accepted for local and VPN addresses (private IPv4 ranges, Tailscale's 100.64.0.0/10
  and `.ts.net`, `.local`, host names without a domain). Any other address requires HTTPS. The app enforces
  this itself (`ServerAPI.validatedURL`), because macOS's local-network exception doesn't cover VPN addresses. The URL session is ephemeral, with no cookies and no cache.
- The app opens **no listening ports** and has **no third-party dependencies**.
- The update check only shows a notice and links to the GitHub release page. It never downloads or runs
  code.
- Release builds are ad-hoc signed with the hardened runtime. They are **not notarized**. Verify the
  SHA-256 checksum published with each release, or build from source.

To remove everything: in the app click Settings › Disconnect (deletes the Keychain item), delete the app, run
`defaults delete io.github.benjamin2607.ambientlight`, remove the container and its config folder, and
revoke the Govee API key in the Govee app if you no longer need it.

## Reporting a vulnerability

Please do **not** open a public issue. Use GitHub's
[private vulnerability reporting](https://github.com/benjamin2607/ambientlightcontrol/security/advisories/new)
instead. You will get an answer within a few days.
