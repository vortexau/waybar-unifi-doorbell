package view

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const DefaultTitle = "unifi-doorbell"

// PollInterval is how often Show checks whether the window is still open.
var PollInterval = 200 * time.Millisecond

// Runner abstracts process control so logic is testable without spawning mpv.
type Runner interface {
	Start(name string, args []string) (int, error)
	Alive(pid int) bool
	Kill(pid int) error
}

type Options struct {
	RTSPSURL    string
	Audio       bool
	Title       string
	RingTimeout time.Duration
}

// MpvArgs builds the mpv command line for the low-latency live view.
func MpvArgs(o Options) []string {
	args := []string{
		"--no-terminal",
		"--force-window=yes",
		"--ontop",
		"--title=" + o.Title,
		"--profile=low-latency",
		"--demuxer-lavf-o=rtsp_transport=tcp",
		"--network-timeout=10",
	}
	if o.Audio {
		args = append(args, "--mute=no")
	} else {
		args = append(args, "--mute=yes")
	}
	return append(args, o.RTSPSURL)
}

func readPid(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

func writePid(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o600)
}

// IsOpen reports whether a live-view window is currently running.
func IsOpen(pidPath string, r Runner) bool {
	pid, ok := readPid(pidPath)
	return ok && r.Alive(pid)
}

func launch(pidPath string, o Options, r Runner) (int, error) {
	pid, err := r.Start("mpv", MpvArgs(o))
	if err != nil {
		return 0, err
	}
	if err := writePid(pidPath, pid); err != nil {
		return 0, err
	}
	return pid, nil
}

// Toggle closes the live view if open, otherwise opens it (detached).
func Toggle(pidPath string, o Options, r Runner) error {
	if pid, ok := readPid(pidPath); ok && r.Alive(pid) {
		_ = r.Kill(pid)
		return os.Remove(pidPath)
	}
	_, err := launch(pidPath, o, r)
	return err
}

// Show ensures the live view is open and closes it after o.RingTimeout unless
// the user closed it first. It blocks until close/timeout; the daemon runs it
// in a goroutine.
func Show(pidPath string, o Options, r Runner) error {
	pid, ok := readPid(pidPath)
	if !ok || !r.Alive(pid) {
		var err error
		if pid, err = launch(pidPath, o, r); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(o.RingTimeout)
	for time.Now().Before(deadline) {
		if !r.Alive(pid) {
			if cur, ok := readPid(pidPath); ok && cur == pid {
				_ = os.Remove(pidPath)
			}
			return nil // user closed early
		}
		time.Sleep(PollInterval)
	}
	if r.Alive(pid) {
		_ = r.Kill(pid)
	}
	if cur, ok := readPid(pidPath); ok && cur == pid {
		_ = os.Remove(pidPath)
	}
	return nil
}
