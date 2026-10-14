# Architecture

```
┌──────────────────────────── AmbientLight.app (macOS) ────────────────────────────┐
│ StatusBarController ── NSStatusItem + borderless NSPanel (SwiftUI PopoverView)   │
│        │                                                                         │
│ LightStore (@Observable, @MainActor) ── optimistic UI, debounce, polling         │
│        │                                                                         │
│ ServerAPI ── URLSession (ephemeral) ── CredentialStore (Keychain: token)         │
└────────┼─────────────────────────────────────────────────────────────────────────┘
         ▼  HTTP + Bearer token (LAN)
┌──────────────────────────── ambientlightd (Docker) ──────────────────────────────┐
│ api ── routes, token check, limits                                               │
│  │                                                                               │
│ controller ── loop every 5 s: TV state + clock → automation.Decide → apply       │
│  │         │                │                                                    │
│ govee      webos            store (/config/state.json)                           │
└──┼─────────┼─────────────────────────────────────────────────────────────────────┘
   ▼         ▼
 Govee API   LG TV :3001 (TLS WebSocket, SSAP)
```

## Server (`server/`)

| Package | Responsibility |
|---|---|
| `cmd/ambientlightd` | Configuration from the environment, API token, startup, `healthcheck` subcommand |
| `internal/automation` | Pure decision logic: schedule window (also across midnight), target state, next change |
| `internal/controller` | Device discovery, polling, applying targets, manual overrides, video scene lookup, TV watcher lifecycle |
| `internal/govee` | Govee Developer API v2 client (`/user/devices`, `/device/state`, `/device/control`, `/device/diy-scenes`) |
| `internal/webos` | LG pairing and power-state subscription, minimal RFC 6455 WebSocket client, certificate pinning |
| `internal/store` | Settings and TV pairing, written atomically with mode `0600` |
| `internal/api` | HTTP handlers, bearer auth, failure limiter, security headers |

### Automation rules

`automation.Decide` gets two inputs, *inside the daily window* and *TV active*:

| TV active | In window | Target |
|---|---|---|
| yes | – | Video |
| no | yes | Daytime colour |
| no | no | Off |

- The controller sends commands only when the target changes (edge-triggered). This keeps API usage low and
  doesn't fight changes made in the Govee app.
- **Manual changes:** if a change from the app leads to a state other than the target (e.g. switching off at
  noon), the context (`InWindow`, `TVActive`) is stored as an override. The automation stays paused until the
  context changes, or until *Resume* is used. A colour change while the target is the daytime colour is not an
  override; it becomes the new daytime colour.
- **TV off grace period:** 20 s, so input switches and restarts don't flip the light.
- **Brightness:** the daytime colour uses `brightness` (default 50 %), video mode uses `videoBrightness`
  (default 100 %), set after the snapshot because a snapshot restores its own brightness. Dimming in the
  popover updates the level of the current mode.
- Failed commands are retried after 30 s. If recalling the video snapshot fails, the scene list is reloaded
  once (a snapshot saved again under the same name gets a new id).

### Govee specifics

- The H6099 exposes `powerSwitch`, `brightness`, `colorRgb`, segment colours, scenes, music modes, `diyScene`
  and `snapshot`. There is **no capability for Video mode**. The server recalls a snapshot (or DIY scene) that
  was saved in the Govee app while Video mode was active. It is referenced by name, because the option values
  are opaque and may change.
- The device state does not report the current mode. The server remembers the last mode it set (`mode` in
  the status). After changes in the Govee app it may show `unknown`.
- Rate limits: the Developer API allows about 10,000 requests per day per account. The server needs about
  1,500 (state once a minute, scenes every 30 minutes, plus commands).

### LG webOS specifics

- `wss://<tv>:3001`, SSAP protocol. Registration with `pairingType: PROMPT` and a manifest that only asks for
  `READ_POWER_STATE`. The TV returns a `client-key` that is used for later sessions.
- Older webOS versions (tested: LG GX, 2020) accept an unsigned manifest for pairing, but then reject the
  power-state request with `401 insufficient permissions`. The manifest therefore carries LG's signed block
  (`manifest.go`). A 401 is reported as "pair again".
- Subscription to `ssap://com.webos.service.tvpower/power/getPowerState`. `Active` is "on". `processing`
  values like *Request Power Off* announce a transition and count as standby.
- Without Quick Start+, the TV leaves the network when switched off. A failed connection then means "off".
- Ping every 20 s, 50 s read timeout, reconnect with back-off up to 30 s.

## API

All endpoints except `/healthz` need `Authorization: Bearer <token>`. Every successful call returns the full status.

| Method | Path | Body |
|---|---|---|
| `GET` | `/healthz` | – (no auth; for the Docker health check) |
| `GET` | `/api/v1/status` | – |
| `POST` | `/api/v1/light` | `{"power": bool, "brightness": 1–100, "color": 0–16777215, "mode": "color" \| "video"}` (any subset) |
| `PATCH` | `/api/v1/settings` | `{"automationEnabled", "onTime": "HH:MM", "offTime", "color", "brightness", "videoBrightness", "videoSource": {"kind": "snapshot" \| "diyScene", "name"}, "tvHost"}` (any subset) |
| `POST` | `/api/v1/automation/resume` | – |
| `POST` | `/api/v1/tv/pair` | `{"host": "192.168.1.20"}` (optional; waits up to 60 s for the prompt on the TV) |

Example:

```bash
curl -H "Authorization: Bearer $TOKEN" http://nas:8787/api/v1/status
```

## App (`app/`)

| File | Responsibility |
|---|---|
| `App/AmbientLightApp.swift` | App entry, settings window, first-run setup |
| `App/StatusBarController.swift` | Menu bar item, popover panel (own `NSPanel` because `MenuBarExtra` always draws a square system frame behind the content), context menu |
| `Models/LightStore.swift` | UI state. Optimistic updates, one write in flight at a time, no writes while the slider is held |
| `Models/ServerStatus.swift` | API types, merging of pending changes |
| `Services/ServerAPI.swift` | HTTP client, URL validation (HTTP only for local addresses) |
| `Services/CredentialStore.swift` | Token in the Keychain, server address in UserDefaults |
| `Services/UpdateChecker.swift` | Daily GitHub release check, notification, link to the release page |
| `Views/` | Popover, backlight illustration, brightness slider, colour row, tiles, settings |

The theme tints the popover with the current light colour, scaled to full brightness so dark colours still
give a visible accent. Video mode shows a slowly rotating spectrum behind the TV.
