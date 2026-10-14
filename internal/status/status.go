// Package status renders the Waybar custom-module JSON for the doorbell icon
// from the last-ring time. It is pure: no files, no clock, no I/O — the caller
// supplies lastRing and now so the logic is fully testable.
package status

import "time"

// Line is the JSON a Waybar custom module consumes. The glyph lives in the
// module's `format`, so only class (for the flash CSS) and tooltip are set here.
type Line struct {
	Class   string `json:"class"`
	Tooltip string `json:"tooltip"`
}

// Render decides the icon state. A zero lastRing means "never rung this
// session". A ring newer than flash is "ringing"; otherwise the tooltip shows
// when it last rang.
func Render(lastRing, now time.Time, flash time.Duration) Line {
	switch {
	case lastRing.IsZero():
		return Line{Tooltip: "UniFi doorbell - click for live view"}
	case now.Sub(lastRing) < flash:
		return Line{Class: "ringing", Tooltip: "Doorbell ringing now"}
	default:
		return Line{Tooltip: "Last ring: " + lastRing.Format("Mon 15:04")}
	}
}
