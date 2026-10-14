package view

import (
	"os"
	"testing"
)

func TestOSRunnerAlive(t *testing.T) {
	r := OSRunner{}
	if !r.Alive(os.Getpid()) {
		t.Fatal("current process should be reported alive")
	}
	if r.Alive(-1) {
		t.Fatal("pid -1 should not be alive")
	}
}
