package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Window struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Position    string `json:"position"`
	AlwaysOnTop bool   `json:"always_on_top"`
}

type Config struct {
	Host               string `json:"host"`
	APIKey             string `json:"api_key"`
	Camera             string `json:"camera"`
	Window             Window `json:"window"`
	RingTimeoutSeconds int    `json:"ring_timeout_seconds"`
	Audio              bool   `json:"audio"`
}

// Dir returns the config directory, honouring XDG_CONFIG_HOME.
func Dir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "waybar-unifi-cam")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "waybar-unifi-cam")
}

func DefaultPath() string { return filepath.Join(Dir(), "config.json") }
func CertPath() string    { return filepath.Join(Dir(), "udm-cert.pem") }

// Load reads and validates the config. It refuses files whose permissions are
// looser than 0600 (any group/other bit set), applies defaults, and checks
// required fields.
func Load(path string) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("config %s has insecure permissions %o; run: chmod 600 %s",
			path, info.Mode().Perm(), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Host == "" {
		return nil, fmt.Errorf("config: host is required")
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("config: api_key is required")
	}
	if c.RingTimeoutSeconds <= 0 {
		c.RingTimeoutSeconds = 30
	}
	if c.Window.Width == 0 {
		c.Window.Width = 960
	}
	if c.Window.Height == 0 {
		c.Window.Height = 540
	}
	return &c, nil
}
