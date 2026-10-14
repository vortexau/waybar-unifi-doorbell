# waybar-unifi-cam

View a UniFi G4 Doorbell Pro from a Waybar icon on Hyprland, with an automatic
live-view pop-up when the doorbell rings. Uses the official UniFi Protect
Integration API (API key) for both the RTSPS stream URL and the ring event.

## Prerequisites

1. **mpv** must run: `mpv --version`. On Manjaro, if it complains about a
   missing `libcdio.so.*`, finish the system upgrade: `sudo pacman -Syu`.
2. **Protect API key:** open `https://192.168.1.1/protect/integrations/` and
   create a key.
3. **Enable RTSP** on the doorbell: Protect -> doorbell -> Settings -> Advanced
   -> enable an RTSP stream.

## Build & install

```bash
go build -o ~/.local/bin/waybar-unifi-cam .
```

## Configure

Create `~/.config/waybar-unifi-cam/config.json` (then lock it down):

```json
{
  "host": "192.168.1.1",
  "api_key": "PASTE_KEY_HERE",
  "camera": "",
  "window": { "width": 960, "height": 540, "position": "center", "always_on_top": true },
  "ring_timeout_seconds": 30,
  "audio": true
}
```

```bash
chmod 600 ~/.config/waybar-unifi-cam/config.json
```

Pin the UDM Pro certificate (one time; confirm the printed fingerprint):

```bash
waybar-unifi-cam trust
```

## Wire up Waybar & Hyprland

- Add `"custom/doorbell"` to `modules-right` and merge `waybar/doorbell.jsonc`
  into `~/.config/waybar/config.jsonc`; append `waybar/style.css` to your
  Waybar `style.css`. Reload Waybar.
- Append `hyprland/windowrule.conf` rules to your Hyprland config and reload
  (`hyprctl reload`).

## Run the daemon

The daemon spawns mpv, so it needs the Wayland environment. Two options:

**A. Hyprland exec-once (simplest, inherits the full env):**

```conf
exec-once = ~/.local/bin/waybar-unifi-cam daemon
```

**B. systemd user service:**

```bash
cp systemd/waybar-unifi-cam.service ~/.config/systemd/user/
# Make the Wayland env visible to user services (needed once per session setup):
systemctl --user import-environment WAYLAND_DISPLAY XDG_CURRENT_DESKTOP HYPRLAND_INSTANCE_SIGNATURE
systemctl --user enable --now waybar-unifi-cam.service
journalctl --user -u waybar-unifi-cam -f
```

## Verify

- Click the Waybar icon -> live view opens (video + audio); click again -> closes.
- `waybar-unifi-cam show` -> opens and auto-closes after 30s; Esc closes early.
- `waybar-unifi-cam daemon --self-test-ring` -> window pops once immediately.
- `waybar-unifi-cam daemon --dump-events` -> press the doorbell and confirm a
  frame with a `ring` type is logged (this validates the real event shape).
- Real ring -> press the doorbell -> live view pops centered.

## Troubleshooting

- **`pinned cert missing`** -> run `waybar-unifi-cam trust`.
- **401 / unauthorized** -> bad or revoked API key.
- **`no RTSPS stream`** -> enable RTSP on the camera in Protect.
- **No pop-up on ring, daemon healthy** -> run with `--dump-events` to inspect
  the real frame; if the ring field differs, update `IsRing` and the
  `testdata/ring.json` fixture.
- **Window not floating/centered** -> confirm the mpv title is `unifi-doorbell`
  and the Hyprland rules are loaded (`hyprctl clients | grep -i unifi`).
