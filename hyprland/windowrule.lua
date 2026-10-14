-- For the Hyprland Lua config (~/.config/hypr/hyprland.lua).
-- Paste this block into hyprland.lua (anywhere at top level), then restart
-- Hyprland or run `hyprctl reload`.
--
-- The title must match view.DefaultTitle ("unifi-doorbell").
-- The window SIZE comes from mpv itself (window.width/height in config.json):
-- Hyprland 0.56.2 ignores size/move/center for floated windows set via rules
-- (hyprwm/Hyprland#16446), so we only float + pin here and let mpv size and
-- centre the window via its own --autofit/--geometry.
hl.window_rule({
    name  = "unifi-doorbell",
    match = { title = "^(unifi-doorbell)$" },

    float = true,
    pin   = true,
    center = true, -- honoured on Hyprland versions without #16446; harmless otherwise
})
