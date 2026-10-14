// Package ringstate persists the time of the most recent doorbell ring so the
// daemon (which detects rings) and the `status` subcommand (which Waybar runs)
// can share it across processes.
package ringstate

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Path is the state file location: ephemeral per-session under XDG_RUNTIME_DIR,
// falling back to the system temp dir.
func Path() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "waybar-unifi-cam.ring")
}

// Write records the ring time as epoch seconds.
func Write(t time.Time) error {
	return os.WriteFile(Path(), []byte(strconv.FormatInt(t.Unix(), 10)), 0o600)
}

// Read returns the last ring time. ok is false if the file is absent,
// unreadable, or malformed.
func Read() (time.Time, bool) {
	b, err := os.ReadFile(Path())
	if err != nil {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(sec, 0), true
}
