package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil { // defeat umask
		t.Fatal(err)
	}
	return p
}

func TestLoadValidAppliesDefaults(t *testing.T) {
	p := writeConfig(t, `{"host":"192.168.1.1","api_key":"k"}`, 0o600)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Host != "192.168.1.1" || c.APIKey != "k" {
		t.Fatalf("unexpected: %+v", c)
	}
	if c.RingTimeoutSeconds != 30 || c.Window.Width != 960 || c.Window.Height != 540 {
		t.Fatalf("defaults not applied: %+v", c)
	}
}

func TestLoadRejectsLoosePerms(t *testing.T) {
	p := writeConfig(t, `{"host":"h","api_key":"k"}`, 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for 0644 perms")
	}
}

func TestLoadRequiresHostAndKey(t *testing.T) {
	p := writeConfig(t, `{"api_key":"k"}`, 0o600)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for missing host")
	}
}
