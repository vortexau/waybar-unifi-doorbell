package view

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

type fakeRunner struct {
	alive   map[int]bool
	started int
	killed  []int
}

func newFake() *fakeRunner { return &fakeRunner{alive: map[int]bool{}} }

func (f *fakeRunner) Start(name string, args []string) (int, error) {
	f.started++
	pid := 1000 + f.started
	f.alive[pid] = true
	return pid, nil
}
func (f *fakeRunner) Alive(pid int) bool { return f.alive[pid] }
func (f *fakeRunner) Kill(pid int) error {
	f.alive[pid] = false
	f.killed = append(f.killed, pid)
	return nil
}

func pidFile(t *testing.T) string { return filepath.Join(t.TempDir(), "p.pid") }

func TestToggleOpensWhenClosed(t *testing.T) {
	f := newFake()
	pp := pidFile(t)
	if err := Toggle(pp, Options{RTSPSURL: "rtsps://x", Title: DefaultTitle}, f); err != nil {
		t.Fatal(err)
	}
	if f.started != 1 {
		t.Fatalf("expected 1 start, got %d", f.started)
	}
	if _, err := os.Stat(pp); err != nil {
		t.Fatalf("pidfile should exist: %v", err)
	}
}

func TestToggleClosesWhenOpen(t *testing.T) {
	f := newFake()
	pp := pidFile(t)
	_ = Toggle(pp, Options{}, f) // open (pid 1001)
	if err := Toggle(pp, Options{}, f); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.killed, 1001) {
		t.Fatalf("expected pid 1001 killed, killed=%v", f.killed)
	}
	if _, err := os.Stat(pp); !os.IsNotExist(err) {
		t.Fatal("pidfile should be removed after close")
	}
}

func TestToggleIgnoresStalePid(t *testing.T) {
	f := newFake()
	pp := pidFile(t)
	if err := os.WriteFile(pp, []byte("99999"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 99999 is not alive in the fake, so Toggle must start a fresh window.
	if err := Toggle(pp, Options{}, f); err != nil {
		t.Fatal(err)
	}
	if f.started != 1 {
		t.Fatalf("expected a fresh start over stale pid, got %d", f.started)
	}
}

func TestShowAutoCloses(t *testing.T) {
	old := PollInterval
	PollInterval = time.Millisecond
	defer func() { PollInterval = old }()

	f := newFake()
	pp := pidFile(t)
	err := Show(pp, Options{RingTimeout: 10 * time.Millisecond}, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.killed) != 1 {
		t.Fatalf("expected auto-close kill, killed=%v", f.killed)
	}
}

// dyingRunner reports its process alive on the first poll, then dead -
// simulating the user closing the window (Esc) before the timeout.
type dyingRunner struct {
	*fakeRunner
	checks int
}

func (d *dyingRunner) Alive(pid int) bool {
	d.checks++
	return d.checks <= 1
}

func TestShowStopsEarlyWhenUserCloses(t *testing.T) {
	old := PollInterval
	PollInterval = time.Millisecond
	defer func() { PollInterval = old }()

	d := &dyingRunner{fakeRunner: newFake()}
	pp := pidFile(t)
	if err := Show(pp, Options{RingTimeout: time.Minute}, d); err != nil {
		t.Fatal(err)
	}
	if len(d.killed) != 0 {
		t.Fatalf("user-closed window must not be killed, killed=%v", d.killed)
	}
	if _, err := os.Stat(pp); !os.IsNotExist(err) {
		t.Fatal("pidfile should be removed after early close")
	}
}

func TestMpvArgsAudio(t *testing.T) {
	on := MpvArgs(Options{RTSPSURL: "rtsps://x", Audio: true, Title: "t"})
	if !slices.Contains(on, "--mute=no") || on[len(on)-1] != "rtsps://x" {
		t.Fatalf("audio-on args wrong: %v", on)
	}
	off := MpvArgs(Options{RTSPSURL: "rtsps://x", Audio: false, Title: "t"})
	if !slices.Contains(off, "--mute=yes") {
		t.Fatalf("audio-off args wrong: %v", off)
	}
}
