package status

import (
	"testing"
	"time"
)

func TestRenderNeverRung(t *testing.T) {
	got := Render(time.Time{}, time.Unix(1000, 0), 30*time.Second)
	if got.Class != "" {
		t.Fatalf("never-rung should have no class, got %q", got.Class)
	}
	if got.Tooltip != "UniFi doorbell - click for live view" {
		t.Fatalf("unexpected tooltip: %q", got.Tooltip)
	}
}

func TestRenderRingingWithinWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	got := Render(now.Add(-5*time.Second), now, 30*time.Second)
	if got.Class != "ringing" {
		t.Fatalf("expected ringing class, got %q", got.Class)
	}
	if got.Tooltip != "Doorbell ringing now" {
		t.Fatalf("unexpected tooltip: %q", got.Tooltip)
	}
}

func TestRenderStaleShowsLastRing(t *testing.T) {
	last := time.Date(2026, 10, 12, 15, 4, 0, 0, time.UTC) // a Monday
	now := last.Add(2 * time.Hour)
	got := Render(last, now, 30*time.Second)
	if got.Class != "" {
		t.Fatalf("stale ring should have no class, got %q", got.Class)
	}
	if got.Tooltip != "Last ring: Mon 15:04" {
		t.Fatalf("unexpected tooltip: %q", got.Tooltip)
	}
}

func TestRenderBoundaryIsNotRinging(t *testing.T) {
	now := time.Unix(1000, 0)
	// exactly at the flash window -> no longer ringing
	got := Render(now.Add(-30*time.Second), now, 30*time.Second)
	if got.Class != "" {
		t.Fatalf("at exactly flash window, expected no class, got %q", got.Class)
	}
}
