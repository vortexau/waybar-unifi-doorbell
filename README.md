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

Create the config directory:

```bash
mkdir -p ~/.config/waybar-unifi-cam
```

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

Waybar needs **two** edits — one in the config and one in the stylesheet — and
they must be done by hand (do not pipe the snippet files into your config; they
are fragments, not drop-in replacements).

### 1. Add the module to `~/.config/waybar/config.jsonc`

A custom module needs **both** an entry in a `modules-*` list *and* a matching
definition object. Do both:

- Add `"custom/doorbell"` to one of your `modules-left` / `modules-center` /
  `modules-right` arrays, wherever you want the icon to sit.
- Add the definition object (from `waybar/doorbell.jsonc`) alongside your other
  module definitions. The key name must match exactly:

  ```jsonc
  "custom/doorbell": {
      "format": "󰥹",
      "tooltip": true,
      "tooltip-format": "UniFi doorbell - click to toggle live view",
      "on-click": "waybar-unifi-cam toggle"
  },
  ```

  Mind the commas: every object in the config is comma-separated, so the block
  needs a trailing `,` unless it is the last entry. The `format` value is a
  Nerd Font glyph (`nf-md-doorbell`); if you see a blank box or tofu, change it
  to any icon your bar font provides, or plain text like `"DOOR"`.

### 2. Add the style to `~/.config/waybar/style.css`

Append a rule for `#custom-doorbell` (Waybar maps the module `custom/doorbell`
to the CSS id `#custom-doorbell`). The repo's `waybar/style.css` uses a generic
blue; adapt the colour to your theme. For example, a theme that defines colours
with `@define-color`:

```css
#custom-doorbell {
    color: @green;      /* or a literal like #89b4fa */
    padding: 2px 14px;
}

#custom-doorbell:hover {
    color: @bright;
}
```

**How to append correctly.** Paste the rule into the file with an editor, or
append the *contents* of the snippet file — not its name:

```bash
# correct: appends the file's contents
cat /path/to/repo/waybar/style.css >> ~/.config/waybar/style.css

# WRONG: this writes the literal text "waybar/style.css" into your stylesheet,
# which is invalid CSS and makes Waybar fail to load its styles
echo waybar/style.css >> ~/.config/waybar/style.css
```

### 3. Reload and verify

```bash
killall -SIGUSR2 waybar      # SIGUSR2 tells Waybar to reload config + style
```

If the icon does not appear or styling looks wrong, **run Waybar from a
terminal** — a signal reload hides parse errors, but a foreground run prints
them with file and line numbers:

```bash
killall waybar; waybar        # Ctrl-C to stop; read the stderr output
```

Common messages: a CSS parse error points at a stray or malformed line in
`style.css`; `Unable to parse config` points at a JSON syntax error (usually a
missing or extra comma) in `config.jsonc`.

### 4. Hyprland window rule

Append the rules in `hyprland/windowrule.conf` to your Hyprland config, then:

```bash
hyprctl reload
```

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
