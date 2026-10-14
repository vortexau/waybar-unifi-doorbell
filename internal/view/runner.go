package view

import (
	"os"
	"os/exec"
	"syscall"
)

// OSRunner runs mpv as a detached process in its own session so it survives
// the short-lived launching process (e.g. the Waybar click handler).
type OSRunner struct{}

func (OSRunner) Start(name string, args []string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }() // reap if we outlive it
	return pid, nil
}

func (OSRunner) Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil // signal 0 = existence probe
}

func (OSRunner) Kill(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}
