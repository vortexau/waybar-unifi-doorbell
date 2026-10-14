# Waybar UniFi Doorbell Camera — Design

**Date:** 2026-10-14
**Status:** Approved

## Summary

A small Go application that puts a UniFi G4 Doorbell Pro live view behind a Waybar
icon on Hyprland, and automatically pops up the live view when the doorbell rings.

- **On-demand:** clicking the Waybar icon toggles a floating live-view window.
- **Automatic:** when the doorbell is pressed, the live view pops up on its own.
- **Live:** a true RTSPS stream (video + audio), not a 1-second refreshing image.

## Environment

- Compositor: Hyprland (Wayland) on Manjaro Linux.
- UniFi console: UDM Pro at `192.168.1.1`, running UniFi Protect.
- Camera: UniFi G4 Doorbell Pro.
- Player: `mpv` (installed from pacman; not currently present).
- Toolchain: Go (already installed); `ffmpeg` present.

## Access method: official Integration API (API key)

Authentication uses a UniFi Protect **API key** (`X-API-KEY` header), created at
`https://192.168.1.1/protect/integrations/`. No user account is required. The key is
currently full-access within Protect and is treated as a secret, but is independently
revocable.

Two endpoints/feeds are used:

- **RTSPS stream URL:** `GET /proxy/protect/integration/v1/cameras/{id}/rtsps-stream`
  returns an `rtsps://192.168.1.1:7441/<alias>` URL for live view.
- **Ring event:** the WebSocket `/proxy/protect/integration/v1/subscribe/events`
  emits an event with `event_type: "ring"` when the doorbell is pressed.
- **Discovery:** `GET /proxy/protect/integration/v1/cameras` lists cameras; the
  doorbell is auto-detected (overridable by name/id in config).

The UDM Pro uses a self-signed certificate. Rather than disabling TLS verification
(which would accept any cert and allow a LAN man-in-the-middle to capture the
full-access API key), the app **pins the UDM Pro's certificate**:

- At setup, a `waybar-unifi-cam trust` subcommand fetches the device's leaf cert from
  `host:443`, prints its SHA-256 fingerprint for the user to confirm, and stores the
  PEM at `~/.config/waybar-unifi-cam/udm-cert.pem` (`0600`).
- The shared `http.Client` uses a `tls.Config` whose `RootCAs` pool contains only that
  pinned cert (with `ServerName` set to the host), reused for both REST and the
  WebSocket dial. Any cert mismatch (device reset, impersonation) then fails loudly in
  the journal instead of silently trusting the wrong endpoint.
- If the pinned cert file is absent, the daemon refuses to start with a message telling
  the user to run `waybar-unifi-cam trust` first. `InsecureSkipVerify` is never used.

## Architecture

A single static Go binary `waybar-unifi-cam` with four subcommands (`trust`, `daemon`,
`toggle`, `show`). Waybar and the daemon both invoke the same binary, so exactly one
code path owns the mpv window.

0. **`waybar-unifi-cam trust`** — one-time setup. Fetches the UDM Pro's leaf cert,
   prints its SHA-256 fingerprint for confirmation, and pins it to
   `~/.config/waybar-unifi-cam/udm-cert.pem` (`0600`). See the access-method section.

1. **`waybar-unifi-cam daemon`** — always-on, runs as a systemd user service with
   auto-restart.
   - On start: REST discovery of the doorbell + fetch of its RTSPS URL.
   - Holds the events WebSocket open with auto-reconnect (exponential backoff).
   - On `event_type: "ring"` → runs the `show` path.

2. **`waybar-unifi-cam toggle`** — Waybar click. If the live-view window is up, kill
   it; else launch it. Stays open until toggled again or Esc. No auto-close.

3. **`waybar-unifi-cam show`** — ring popup. Launch (or keep) the window and arm a
   30s auto-close timer; Esc still closes early.

**Window-state tracking:** a pidfile in `$XDG_RUNTIME_DIR` identifies the running mpv
process, so `toggle`/`show` can tell whether a window already exists. A stale pidfile
(from a crash) is ignored and overwritten. mpv runs with a fixed app-id, low-latency
RTSPS flags, and audio enabled.

**Waybar `custom/doorbell`** — an icon (doorbell glyph) with `on-click` =
`waybar-unifi-cam toggle`. Static icon in v1.

**Hyprland window rule** — matches the mpv app-id → float, centered, 960×540,
always-on-top.

### Data flow

```
            REST (once)                WebSocket (persistent)
daemon ───────────────────► UDM Pro Protect Integration API
  │   get doorbell id + RTSPS URL   │   ring event ("ring")
  │                                 ▼
  │                      waybar-unifi-cam show ──► mpv (floating, 960x540, 30s auto-close)
  ▼
Waybar icon ──on-click──► waybar-unifi-cam toggle ──► mpv (floating, stays until toggled/Esc)
```

## Behaviour decisions

- Ring popup: auto-close after **30s**, and manual close (Esc/click) allowed.
- Icon click: **toggles** the same live view on/off.
- Audio: **video + audio** (listen).
- Window: **centered, 960×540**, always-on-top.
- Two-way talk: **out of scope** for v1.

## Configuration

`~/.config/waybar-unifi-cam/config.json` (JSON via stdlib). Mode `0600`, enforced at
startup — the daemon refuses to start if perms are looser. Secrets live only here,
never in Waybar or Hyprland config. The pinned cert sits beside it at
`~/.config/waybar-unifi-cam/udm-cert.pem` (`0600`), written by `trust`.

```json
{
  "host": "192.168.1.1",
  "api_key": "<from Protect → Integrations>",
  "camera": "",
  "window": { "width": 960, "height": 540, "position": "center", "always_on_top": true },
  "ring_timeout_seconds": 30,
  "audio": true
}
```

## Error handling

- **Startup:** missing/loose-perm config, missing pinned cert (`run trust first`), bad
  API key (HTTP 401), cert mismatch (pin verification failure), or no doorbell found
  → log a clear message and exit non-zero (visible in `systemctl --user status`).
- **RTSPS URL** is re-fetched on each `show`, so key rotation or camera-id changes
  recover without a daemon restart.
- **WebSocket drop:** reconnect with exponential backoff (1s → max 30s), logging each
  attempt. Rings during a disconnect are missed by design (no event replay) in v1.
- **mpv:** if the stream fails to open, mpv exits and the pidfile is cleaned up so the
  next `toggle`/`show` starts fresh. Launch failures are logged.
- **Toggle race:** pidfile checked against a live process; stale pidfile overwritten.

## Testing strategy

**Unit (Go, table-driven):**
- Config load/validation, including the `0600` perms check.
- Cert pinning: a `tls.Config` built from the pinned PEM accepts the matching cert and
  rejects a different one (fed test certs).
- Ring-event JSON parsing, fed captured/synthetic frames.
- Toggle state logic for pidfile present / stale / absent.
- RTSPS URL parsing.

**Integration (manual, documented in README):**
1. `waybar-unifi-cam toggle` → window opens with live audio+video; again → closes.
2. `waybar-unifi-cam show` → opens, auto-closes after 30s; Esc closes early.
3. `waybar-unifi-cam daemon --self-test-ring` injects a synthetic ring event to prove
   the daemon→show path without a real visitor.
4. Real ring → press the doorbell → window pops centered.

**Resilience:**
- `systemctl --user restart waybar-unifi-cam` mid-session.
- Drop/restore network to watch reconnect in `journalctl --user -u waybar-unifi-cam`.

## File layout

```
waybar-unifi-cam/
├── go.mod / go.sum
├── main.go                  // subcommand dispatch
├── internal/
│   ├── config/              // load + perms enforcement
│   ├── protect/             // REST client + events WebSocket (typed ring event)
│   └── view/                // mpv launcher, pidfile, toggle/show logic
├── systemd/waybar-unifi-cam.service   // user unit
├── waybar/                  // custom/doorbell snippet + CSS
├── hyprland/                // windowrule snippet
└── README.md                // setup: API key, install, Waybar/Hyprland wiring, tests
```

## Dependencies

- Go standard library for REST, JSON, TLS, process spawning, config.
- `github.com/coder/websocket` — the only third-party Go dependency.
- `mpv` (pacman) — RTSPS player.

## Out of scope (v1)

- Two-way talk (talkback).
- Event replay for rings missed during a WebSocket disconnect.
- Dynamic Waybar icon state (flash/badge on ring) — static icon only.
- Multiple cameras — single doorbell only.
