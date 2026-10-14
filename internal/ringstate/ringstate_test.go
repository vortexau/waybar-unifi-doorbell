package ringstate

import (
	"os"
	"testing"
	"time"
)

func TestWriteReadRoundTrip(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	want := time.Unix(1_700_000_123, 0)
	if err := Write(want); err != nil {
		t.Fatal(err)
	}
	got, ok := Read()
	if !ok {
		t.Fatal("expected ok=true after Write")
	}
	if !got.Equal(want) {
		t.Fatalf("round-trip mismatch: got %v want %v", got, want)
	}
}

func TestReadMissing(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if _, ok := Read(); ok {
		t.Fatal("expected ok=false when no state file exists")
	}
}

func TestReadMalformed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	if err := os.WriteFile(Path(), []byte("not-a-number"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := Read(); ok {
		t.Fatal("expected ok=false for malformed state file")
	}
}

func TestWritePerms(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := Write(time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", info.Mode().Perm())
	}
}
