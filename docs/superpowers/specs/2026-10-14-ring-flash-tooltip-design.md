# Ring Flash + Last-Ring Tooltip — Design

**Date:** 2026-10-14
**Status:** Approved

## Summary

Make the Waybar doorbell icon react to a ring: briefly **flash** (auto-clearing),
and show the **last-ring time** in its hover tooltip. Extends the existing
`waybar-unifi-cam` app; no new external dependencies.

Dropped from the original idea: a snapshot thumbnail. Waybar tooltips are
text-only (Pango markup, no images), and the user chose a text tooltip plus the
existing live-view pop-up over any image-display mechanism. No snapshot is
fetched.

## Behaviour

- On a doorbell ring, the icon animates (pulses) for a flash window, then returns
  to normal on its own. Nothing to dismiss.
- The flash window equals the existing `ring_timeout_seconds` (default 30s) —
  the same "event window" as the auto pop-up.
- The hover tooltip always reflects state: "Doorbell ringing now" while flashing,
  otherwise "Last ring: Mon 15:04" if a ring is known this session, otherwise
  "UniFi doorbell - click for live view".
- Clicking the icon still toggles the live view (unchanged).

## Mechanism

The daemon and the Waybar module are separate processes, so state is bridged by
a file plus a real-time signal (the standard Waybar refresh pattern):

1. Daemon on ring writes the ring timestamp to a state file, then sends
   `SIGRTMIN+<waybar_signal>` to Waybar (flash on), and schedules one more signal
   after the flash window (flash off).
2. Waybar's `custom/doorbell` module runs `waybar-unifi-cam status` as its
   `exec`; receiving the signal makes Waybar re-run it.
3. `status` reads the state file, compares the last-ring time to now, and prints
   one line of JSON: `{"class": "ringing"|"", "tooltip": "<text>"}`.
4. The glyph stays in the module's literal `format` (user-editable); the flash is
   pure CSS keyed on the `.ringing` class; the tooltip is data.

Event-driven, no polling: the icon flashes on the ring signal and clears on the
flash-window signal, with no `interval` in Waybar.

## Components

**`internal/ringstate`** (new) — persist/read the last-ring timestamp at
`$XDG_RUNTIME_DIR/waybar-unifi-cam.ring` (epoch seconds; ephemeral, clears on
reboot).
- `Path() string`
- `Write(t time.Time) error`
- `Read() (time.Time, bool)` — `ok=false` on missing/malformed/unreadable.

**`internal/status`** (new) — pure render, no files or clocks inside:
- `type Line struct { Class string `json:"class"`; Tooltip string `json:"tooltip"` }`
- `Render(lastRing, now time.Time, flash time.Duration) Line`
  - `lastRing` zero → `Class:"", Tooltip:"UniFi doorbell - click for live view"`.
  - `now - lastRing < flash` → `Class:"ringing", Tooltip:"Doorbell ringing now"`.
  - else → `Class:"", Tooltip:"Last ring: " + lastRing.Format("Mon 15:04")`.

**`main.go`**:
- `status` subcommand — reads `ringstate`, calls `status.Render(..., time.Now(),
  flash)` with `flash = ring_timeout_seconds`, marshals the `Line` to JSON on
  stdout. Always prints valid JSON; a missing state file is treated as never-rung.
  It uses `config.Load` only (for the flash duration) — **not** `loadClient`: it
  needs no pinned cert or network, and runs on every Waybar refresh. If
  `config.Load` fails, fall back to a 30s flash and still emit valid JSON.
- `signalWaybar(n int)` — thin helper running `pkill -RTMIN+<n> waybar`; errors
  ignored (Waybar may not be running). Not unit-tested, like `OSRunner`.
- `onRing` gains, before the existing resolve+show: `ringstate.Write(now)`,
  `signalWaybar(cfg.WaybarSignal)`, and a ctx-aware goroutine that sleeps the
  flash window then signals again. A `ringstate.Write` error is logged, non-fatal.

**`internal/config`** — add `WaybarSignal int` (`json:"waybar_signal"`), default
`8` when `<= 0`.

**Waybar artifacts**:
- `waybar/doorbell.jsonc` — module becomes `exec: waybar-unifi-cam status`,
  `return-type: json`, `format: "󰥹"`, `signal: 8`, `on-click: waybar-unifi-cam
  toggle`.
- `waybar/style.css` — add the `doorbell-flash` keyframes and
  `#custom-doorbell.ringing` animation.

## Configuration

`~/.config/waybar-unifi-cam/config.json` gains:
```json
"waybar_signal": 8
```
Flash duration = existing `ring_timeout_seconds`. The config template and README
are updated.

## Error handling

- `status`: missing/unreadable/malformed state → treat as never-rung; always
  emit valid JSON (never exit non-zero or print garbage — Waybar would show a
  broken module).
- `ringstate.Write` failure in the daemon: logged, non-fatal (pop-up still works,
  flash just will not show).
- `signalWaybar`: ignore errors.

## Testing

**Unit:**
- `ringstate`: write→read round-trip; missing file → `ok=false`; malformed
  contents → `ok=false`.
- `status.Render` (table-driven, injected `now`): never-rung; ring within window
  (ringing class + "ringing" tooltip); ring older than window (no class +
  "Last ring: …" tooltip); boundary at exactly `flash`.
- `config`: `waybar_signal` default applied when `<= 0`; explicit value honoured.

**Manual (documented in README):**
- `waybar-unifi-cam status` prints valid JSON in all three states.
- `waybar-unifi-cam daemon --self-test-ring` drives `onRing` → icon pulses for
  the flash window, tooltip shows the ring, then clears — no real ring needed.
- Real ring → icon flashes and the pop-up appears.

## Out of scope

- Snapshot/thumbnail display (dropped; Waybar tooltips cannot render images).
- Persistent "missed ring" badge requiring acknowledgement (chose transient
  auto-clear).
- Configurable flash duration separate from `ring_timeout_seconds`.

## File layout

```
internal/ringstate/ringstate.go   (+ ringstate_test.go)
internal/status/status.go         (+ status_test.go)
internal/config/config.go         (WaybarSignal field + default)
main.go                           (status subcommand, signalWaybar, onRing)
waybar/doorbell.jsonc             (exec/signal module)
waybar/style.css                  (flash keyframes + .ringing)
README.md                         (updated Waybar wiring + flash note)
```
