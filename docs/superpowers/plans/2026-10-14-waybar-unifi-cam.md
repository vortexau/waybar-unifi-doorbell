# Waybar UniFi Doorbell Camera Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go app that shows a UniFi G4 Doorbell Pro live view from a Waybar icon on Hyprland, and auto-pops the live view when the doorbell rings.

**Architecture:** One static Go binary `waybar-unifi-cam` with four subcommands (`trust`, `daemon`, `toggle`, `show`). The `daemon` holds the UniFi Protect Integration API events WebSocket open and, on a ring, launches `mpv` on the camera's RTSPS stream; `toggle` is the Waybar click path; `trust` pins the UDM Pro's self-signed cert. mpv window lifecycle lives behind an injectable `Runner` interface so logic is unit-testable without spawning a window.

**Tech Stack:** Go 1.26 (stdlib for REST/JSON/TLS/process), `github.com/coder/websocket` for the WebSocket, `mpv` for RTSPS playback, Waybar `custom/` module, Hyprland window rule, systemd user service.

## Global Constraints

- Go module path: `waybar-unifi-cam`; Go directive `go 1.26`.
- Only third-party Go dependency allowed: `github.com/coder/websocket v1.8.15`. Everything else is standard library.
- `InsecureSkipVerify` is **forbidden on data connections**. The REST + WebSocket client verify via certificate pinning (custom `VerifyConnection`). `InsecureSkipVerify` is permitted in exactly one place — `tlspin.FetchLeafCert` — whose only job is trust-on-first-use cert retrieval with a user-confirmed fingerprint.
- File permissions: config and cert files `0600`; config dir `0700`. The daemon refuses to load a config whose perms are looser than `0600`.
- UniFi Protect Integration API: base path `/proxy/protect/integration/v1`, auth header `X-API-KEY`, scheme `https`/`wss` against `host` (`192.168.1.1`).
- mpv window title is the literal string `unifi-doorbell` — it MUST match the Hyprland window rule.
- TDD: write the failing test first, watch it fail, implement minimally, watch it pass, commit.
- Every commit message ends with the repository's required trailers:
  ```
  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_0124bLKhm4WuCjoqz8AWhs5M
  ```
  (Omitted from the short messages below for brevity; append them to each real commit.)

---

## Prerequisites (user/system actions, not code)

These are done once by the user and are referenced by the manual integration tests in Task 7. They do not block the build or unit tests.

1. **Fix mpv** (Manjaro partial-upgrade): `sudo pacman -Syu` so `libcdio` provides `libcdio.so.21`. Verify `mpv --version` runs cleanly.
2. **Create the Protect API key:** UniFi OS → open `https://192.168.1.1/protect/integrations/` → create a key. Copy it.
3. **Enable RTSP** on the G4 Doorbell Pro: Protect → the doorbell → Settings → Advanced → enable an RTSP stream (any quality). Without this, `rtsps-stream` returns nothing.
4. **Create config** `~/.config/waybar-unifi-cam/config.json` (then `chmod 600`):
   ```json
   {
     "host": "192.168.1.1",
     "api_key": "PASTE_KEY_HERE",
     "camera": "",
     "window": { "width": 960, "height": 540, "position": "center", "always_on_top": true },
     "ring_timeout_seconds": 30,
     "audio": true
   }
   ```

---

## Task 1: Project scaffold + config package

**Files:**
- Create: `go.mod`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `config.Config` struct with fields `Host, APIKey, Camera string`, `Window config.Window`, `RingTimeoutSeconds int`, `Audio bool`.
  - `config.Window` struct: `Width, Height int`, `Position string`, `AlwaysOnTop bool`.
  - `config.Dir() string`, `config.DefaultPath() string`, `config.CertPath() string`.
  - `config.Load(path string) (*config.Config, error)` — enforces `0600`, validates required fields, applies defaults.

- [ ] **Step 1: Create the Go module**

```bash
cd /home/vortex/Projects/waybar-unifi-cam
cat > go.mod <<'EOF'
module waybar-unifi-cam

go 1.26
EOF
```

- [ ] **Step 2: Write the failing test**

Create `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/config/`
Expected: FAIL — `undefined: Load` (build error).

- [ ] **Step 4: Write minimal implementation**

Create `internal/config/config.go`:

```go
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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/config/`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add go.mod internal/config/
git commit -m "feat(config): typed config load with perms enforcement"
```

---

## Task 2: TLS certificate pinning + fingerprint

**Files:**
- Create: `internal/tlspin/tlspin.go`
- Test: `internal/tlspin/tlspin_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `tlspin.FetchLeafCert(host string) (*x509.Certificate, error)` — dials `host:443`, returns the leaf cert (TOFU; the only sanctioned `InsecureSkipVerify`).
  - `tlspin.Fingerprint(cert *x509.Certificate) string` — SHA-256, colon-separated hex.
  - `tlspin.EncodePEM(cert *x509.Certificate) []byte`.
  - `tlspin.ConfigFromPEM(pemBytes []byte, serverName string) (*tls.Config, error)` — a `*tls.Config` that trusts ONLY the exact pinned cert via `VerifyConnection`.

- [ ] **Step 1: Write the failing test**

Create `internal/tlspin/tlspin_test.go`:

```go
package tlspin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func makeCert(t *testing.T, cn string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestConfigFromPEMAcceptsPinned(t *testing.T) {
	cert := makeCert(t, "udm")
	cfg, err := ConfigFromPEM(EncodePEM(cert), "192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.VerifyConnection(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert},
	}); err != nil {
		t.Fatalf("pinned cert should verify: %v", err)
	}
}

func TestConfigFromPEMRejectsOther(t *testing.T) {
	cfg, err := ConfigFromPEM(EncodePEM(makeCert(t, "udm")), "192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.VerifyConnection(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{makeCert(t, "evil")},
	}); err == nil {
		t.Fatal("different cert should be rejected")
	}
}

func TestFingerprintStable(t *testing.T) {
	cert := makeCert(t, "udm")
	if Fingerprint(cert) != Fingerprint(cert) {
		t.Fatal("fingerprint must be deterministic")
	}
	if len(Fingerprint(cert)) != 95 { // 32 bytes -> 64 hex + 31 colons
		t.Fatalf("unexpected fingerprint length: %q", Fingerprint(cert))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tlspin/`
Expected: FAIL — `undefined: ConfigFromPEM` etc.

- [ ] **Step 3: Write minimal implementation**

Create `internal/tlspin/tlspin.go`:

```go
package tlspin

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"time"
)

// FetchLeafCert connects to host:443 and returns its leaf certificate. It
// deliberately skips verification: its ONLY purpose is retrieving a self-signed
// cert for trust-on-first-use pinning. The caller must show the fingerprint to
// the user for out-of-band confirmation, and no secrets are sent here.
func FetchLeafCert(host string) (*x509.Certificate, error) {
	d := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(d, "tcp", net.JoinHostPort(host, "443"),
		&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // TOFU retrieval only
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no peer certificate from %s", host)
	}
	return certs[0], nil
}

// Fingerprint returns the SHA-256 of the cert as colon-separated hex.
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	h := hex.EncodeToString(sum[:])
	out := make([]byte, 0, len(h)+len(h)/2-1)
	for i := 0; i < len(h); i += 2 {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, h[i], h[i+1])
	}
	return string(out)
}

// EncodePEM returns the PEM encoding of a certificate.
func EncodePEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

// ConfigFromPEM builds a *tls.Config trusting ONLY the exact certificate in
// pemBytes. It pins by comparing the server leaf's SHA-256 to the pinned cert
// in VerifyConnection. The default CA chain check is disabled because the pin
// is strictly stronger for a known device and is robust to self-signed certs
// whose SAN omits the IP address.
func ConfigFromPEM(pemBytes []byte, serverName string) (*tls.Config, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("no certificate in pinned PEM")
	}
	pinned, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pinned cert: %w", err)
	}
	want := sha256.Sum256(pinned.Raw)
	return &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // replaced by exact pin below
		ServerName:         serverName,
		MinVersion:         tls.VersionTLS12,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("no peer certificate")
			}
			if got := sha256.Sum256(cs.PeerCertificates[0].Raw); got != want {
				return fmt.Errorf("certificate pin mismatch for %s", serverName)
			}
			return nil
		},
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tlspin/`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/tlspin/
git commit -m "feat(tlspin): certificate pinning and fingerprint helpers"
```

---

## Task 3: Protect REST client (discovery + RTSPS URL)

**Files:**
- Create: `internal/protect/client.go`
- Test: `internal/protect/client_test.go`

**Interfaces:**
- Consumes: `*tls.Config` from `tlspin.ConfigFromPEM`.
- Produces:
  - `protect.New(host, apiKey string, tlsConf *tls.Config) *protect.Client`.
  - `protect.Camera` struct: `ID, Name, ModelKey, Type string`, `FeatureFlags struct{ IsDoorbell bool }`.
  - `(*Client).Cameras(ctx context.Context) ([]Camera, error)`.
  - `protect.FindDoorbell(cams []Camera, sel string) (Camera, error)` — pure selector.
  - `(*Client).RTSPSURL(ctx context.Context, cameraID string) (string, error)`.
  - Unexported `newRaw(base, apiKey string, hc *http.Client) *Client` for tests, and const `integrationBase = "/proxy/protect/integration/v1"`.

- [ ] **Step 1: Write the failing test**

Create `internal/protect/client_test.go`:

```go
package protect

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCamerasAndFindDoorbell(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(integrationBase+"/cameras", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		io.WriteString(w, `[{"id":"a","name":"Front Door","featureFlags":{"isDoorbell":true}},
		                   {"id":"b","name":"Yard","featureFlags":{"isDoorbell":false}}]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newRaw(srv.URL, "k", srv.Client())
	cams, err := c.Cameras(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cams) != 2 {
		t.Fatalf("got %d cameras", len(cams))
	}
	db, err := FindDoorbell(cams, "")
	if err != nil {
		t.Fatal(err)
	}
	if db.ID != "a" {
		t.Fatalf("auto-detect wrong doorbell: %q", db.ID)
	}
	if _, err := FindDoorbell(cams, "nope"); err == nil {
		t.Fatal("expected error selecting missing camera")
	}
	byName, err := FindDoorbell(cams, "yard")
	if err != nil || byName.ID != "b" {
		t.Fatalf("name selection failed: %v %+v", err, byName)
	}
}

func TestRTSPSURLPrefersHigh(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(integrationBase+"/cameras/a/rtsps-stream", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"low":"rtsps://x/low","high":"rtsps://x/high"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newRaw(srv.URL, "k", srv.Client())
	u, err := c.RTSPSURL(context.Background(), "a")
	if err != nil {
		t.Fatal(err)
	}
	if u != "rtsps://x/high" {
		t.Fatalf("expected high quality, got %q", u)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/protect/`
Expected: FAIL — `undefined: newRaw` etc.

- [ ] **Step 3: Write minimal implementation**

Create `internal/protect/client.go`:

```go
package protect

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const integrationBase = "/proxy/protect/integration/v1"

type Client struct {
	base   string // e.g. https://192.168.1.1
	apiKey string
	tls    *tls.Config
	http   *http.Client
}

func New(host, apiKey string, tlsConf *tls.Config) *Client {
	return &Client{
		base:   "https://" + host,
		apiKey: apiKey,
		tls:    tlsConf,
		http: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsConf},
		},
	}
}

// newRaw builds a client against an explicit base URL and http.Client (tests).
func newRaw(base, apiKey string, hc *http.Client) *Client {
	return &Client{base: base, apiKey: apiKey, http: hc}
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

type Camera struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ModelKey     string `json:"modelKey"`
	Type         string `json:"type"`
	FeatureFlags struct {
		IsDoorbell bool `json:"isDoorbell"`
	} `json:"featureFlags"`
}

func (c *Client) Cameras(ctx context.Context) ([]Camera, error) {
	body, err := c.get(ctx, integrationBase+"/cameras")
	if err != nil {
		return nil, err
	}
	var cams []Camera
	if err := json.Unmarshal(body, &cams); err != nil {
		return nil, fmt.Errorf("parse cameras: %w", err)
	}
	return cams, nil
}

// FindDoorbell returns the camera matching sel (by id or case-insensitive name).
// If sel is empty, it returns the first camera flagged isDoorbell, falling back
// to a name containing "doorbell".
func FindDoorbell(cams []Camera, sel string) (Camera, error) {
	if sel != "" {
		for _, c := range cams {
			if c.ID == sel || strings.EqualFold(c.Name, sel) {
				return c, nil
			}
		}
		return Camera{}, fmt.Errorf("no camera matching %q", sel)
	}
	for _, c := range cams {
		if c.FeatureFlags.IsDoorbell {
			return c, nil
		}
	}
	for _, c := range cams {
		if strings.Contains(strings.ToLower(c.Name), "doorbell") {
			return c, nil
		}
	}
	return Camera{}, fmt.Errorf(`no doorbell found; set "camera" in config`)
}

// RTSPSURL returns the highest-quality RTSPS URL for a camera. The endpoint
// returns a JSON object mapping quality name -> rtsps URL.
func (c *Client) RTSPSURL(ctx context.Context, cameraID string) (string, error) {
	body, err := c.get(ctx, fmt.Sprintf("%s/cameras/%s/rtsps-stream", integrationBase, cameraID))
	if err != nil {
		return "", err
	}
	var streams map[string]string
	if err := json.Unmarshal(body, &streams); err != nil {
		return "", fmt.Errorf("parse rtsps-stream: %w", err)
	}
	for _, q := range []string{"high", "medium", "low"} {
		if u := streams[q]; u != "" {
			return u, nil
		}
	}
	for _, u := range streams {
		if u != "" {
			return u, nil
		}
	}
	return "", fmt.Errorf("no RTSPS stream; enable RTSP on the camera in Protect")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/protect/`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/protect/client.go internal/protect/client_test.go
git commit -m "feat(protect): REST client for camera discovery and RTSPS URL"
```

---

## Task 4: Events WebSocket + ring detection

**Files:**
- Create: `internal/protect/events.go`
- Create: `internal/protect/testdata/ring.json`
- Create: `internal/protect/testdata/motion.json`
- Test: `internal/protect/events_test.go`
- Modify: `go.mod` / `go.sum` (add `github.com/coder/websocket`)

**Interfaces:**
- Consumes: `*Client` (with `base`, `apiKey`, `tls` set by `New`).
- Produces:
  - `protect.IsRing(frame []byte) bool` — pure detector.
  - `(*Client).Events(ctx context.Context, onFrame func([]byte)) error` — one WebSocket connection; calls `onFrame` per frame; returns on ctx cancel or drop.

> **Note on frame shape:** the exact JSON of the integration events feed is confirmed only to carry a `ring`-typed event. `IsRing` matches the documented spellings. Task 6 adds `daemon --dump-events`; the FIRST real integration run (Task 7) should dump a genuine ring frame and, if its shape differs, the testdata fixtures and `IsRing` are tightened then. The pure test below locks in the documented shapes.

- [ ] **Step 1: Add the WebSocket dependency**

```bash
cd /home/vortex/Projects/waybar-unifi-cam
go get github.com/coder/websocket@v1.8.15
```

- [ ] **Step 2: Create test fixtures**

Create `internal/protect/testdata/ring.json`:

```json
{"type":"ring","item":{"camera":"a","eventId":"e1","start":1700000000000}}
```

Create `internal/protect/testdata/motion.json`:

```json
{"type":"motion","item":{"camera":"a"}}
```

- [ ] **Step 3: Write the failing test**

Create `internal/protect/events_test.go`:

```go
package protect

import (
	"os"
	"testing"
)

func TestIsRing(t *testing.T) {
	ring, err := os.ReadFile("testdata/ring.json")
	if err != nil {
		t.Fatal(err)
	}
	motion, err := os.ReadFile("testdata/motion.json")
	if err != nil {
		t.Fatal(err)
	}
	if !IsRing(ring) {
		t.Error("ring.json should be detected as a ring")
	}
	if IsRing(motion) {
		t.Error("motion.json should not be a ring")
	}
	if IsRing([]byte("not json")) {
		t.Error("garbage should not be a ring")
	}
	// camelCase spelling also accepted
	if !IsRing([]byte(`{"eventType":"ring"}`)) {
		t.Error("eventType spelling should be accepted")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/protect/ -run TestIsRing`
Expected: FAIL — `undefined: IsRing`.

- [ ] **Step 5: Write minimal implementation**

Create `internal/protect/events.go`:

```go
package protect

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/coder/websocket"
)

// IsRing reports whether a raw events-websocket frame is a doorbell ring. The
// integration events feed delivers JSON objects; a ring is identified by a
// type field equal to "ring". Snake_case, camelCase, and a nested item.type
// are all accepted to tolerate schema spelling differences.
func IsRing(frame []byte) bool {
	var e struct {
		Type      string `json:"type"`
		EventType string `json:"eventType"`
		Item      struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	if json.Unmarshal(frame, &e) != nil {
		return false
	}
	return strings.EqualFold(e.Type, "ring") ||
		strings.EqualFold(e.EventType, "ring") ||
		strings.EqualFold(e.Item.Type, "ring")
}

// Events opens the integration events websocket and calls onFrame for every
// frame until ctx is cancelled or the connection errors (returned).
func (c *Client) Events(ctx context.Context, onFrame func([]byte)) error {
	wsURL := strings.Replace(c.base, "https://", "wss://", 1) +
		integrationBase + "/subscribe/events"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"X-API-KEY": {c.apiKey}},
		HTTPClient: &http.Client{Transport: &http.Transport{TLSClientConfig: c.tls}},
	})
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(1 << 20)
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		onFrame(data)
	}
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/protect/`
Expected: PASS (all protect tests, including TestIsRing).

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/protect/
git commit -m "feat(protect): events websocket and ring detection"
```

---

## Task 5: View package — pidfile, mpv args, toggle/show logic

**Files:**
- Create: `internal/view/view.go`
- Test: `internal/view/view_test.go`

**Interfaces:**
- Consumes: nothing (pure logic + injected `Runner`).
- Produces:
  - `view.Runner` interface: `Start(name string, args []string) (int, error)`, `Alive(pid int) bool`, `Kill(pid int) error`.
  - `view.Options` struct: `RTSPSURL string`, `Audio bool`, `Title string`, `RingTimeout time.Duration`.
  - `const view.DefaultTitle = "unifi-doorbell"`.
  - `view.MpvArgs(o Options) []string`.
  - `view.IsOpen(pidPath string, r Runner) bool`.
  - `view.Toggle(pidPath string, o Options, r Runner) error`.
  - `view.Show(pidPath string, o Options, r Runner) error`.
  - `var view.PollInterval = 200 * time.Millisecond` (lowered in tests).

- [ ] **Step 1: Write the failing test**

Create `internal/view/view_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/view/`
Expected: FAIL — `undefined: Toggle` etc.

- [ ] **Step 3: Write minimal implementation**

Create `internal/view/view.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/view/`
Expected: PASS (all view tests).

- [ ] **Step 5: Commit**

```bash
git add internal/view/
git commit -m "feat(view): pidfile-tracked toggle/show with injectable runner"
```

---

## Task 6: OS runner + subcommand wiring (main)

**Files:**
- Create: `internal/view/runner.go`
- Test: `internal/view/runner_test.go`
- Create: `main.go`

**Interfaces:**
- Consumes: `config`, `tlspin`, `protect`, `view` from Tasks 1–5.
- Produces:
  - `view.OSRunner` implementing `view.Runner` (real mpv via `exec` + `setsid`).
  - `main` dispatching `trust | daemon | toggle | show`, with `daemon` flags `--self-test-ring` and `--dump-events`.

- [ ] **Step 1: Write the failing test**

Create `internal/view/runner_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/view/ -run TestOSRunnerAlive`
Expected: FAIL — `undefined: OSRunner`.

- [ ] **Step 3: Write the OS runner**

Create `internal/view/runner.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/view/ -run TestOSRunnerAlive`
Expected: PASS.

- [ ] **Step 5: Write main.go**

Create `main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"waybar-unifi-cam/internal/config"
	"waybar-unifi-cam/internal/protect"
	"waybar-unifi-cam/internal/tlspin"
	"waybar-unifi-cam/internal/view"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: waybar-unifi-cam {trust|daemon|toggle|show}")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "trust":
		err = cmdTrust(os.Args[2:])
	case "daemon":
		err = cmdDaemon(os.Args[2:])
	case "toggle":
		err = cmdView(false)
	case "show":
		err = cmdView(true)
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func pidPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "waybar-unifi-cam.pid")
}

func cmdTrust(args []string) error {
	host := ""
	if len(args) > 0 {
		host = args[0]
	} else {
		c, err := config.Load(config.DefaultPath())
		if err != nil {
			return fmt.Errorf("need host: pass as arg or create %s first (%w)",
				config.DefaultPath(), err)
		}
		host = c.Host
	}
	cert, err := tlspin.FetchLeafCert(host)
	if err != nil {
		return err
	}
	fmt.Printf("Certificate for %s\n  SHA-256: %s\n", host, tlspin.Fingerprint(cert))
	fmt.Println("Confirm this matches the fingerprint in the UniFi console, then it is pinned.")
	if err := os.MkdirAll(config.Dir(), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(config.CertPath(), tlspin.EncodePEM(cert), 0o600); err != nil {
		return err
	}
	fmt.Println("Pinned to", config.CertPath())
	return nil
}

func loadClient() (*config.Config, *protect.Client, error) {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return nil, nil, err
	}
	pemBytes, err := os.ReadFile(config.CertPath())
	if err != nil {
		return nil, nil, fmt.Errorf("pinned cert missing (%v); run: waybar-unifi-cam trust", err)
	}
	tlsConf, err := tlspin.ConfigFromPEM(pemBytes, cfg.Host)
	if err != nil {
		return nil, nil, err
	}
	return cfg, protect.New(cfg.Host, cfg.APIKey, tlsConf), nil
}

func resolveStream(ctx context.Context, cfg *config.Config, c *protect.Client) (string, error) {
	cams, err := c.Cameras(ctx)
	if err != nil {
		return "", err
	}
	db, err := protect.FindDoorbell(cams, cfg.Camera)
	if err != nil {
		return "", err
	}
	return c.RTSPSURL(ctx, db.ID)
}

func options(cfg *config.Config, url string) view.Options {
	return view.Options{
		RTSPSURL:    url,
		Audio:       cfg.Audio,
		Title:       view.DefaultTitle,
		RingTimeout: time.Duration(cfg.RingTimeoutSeconds) * time.Second,
	}
}

func cmdView(show bool) error {
	r := view.OSRunner{}
	pp := pidPath()
	// Fast path: closing a toggle needs no network.
	if !show && view.IsOpen(pp, r) {
		return view.Toggle(pp, view.Options{}, r)
	}
	cfg, c, err := loadClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	url, err := resolveStream(ctx, cfg, c)
	if err != nil {
		return err
	}
	o := options(cfg, url)
	if show {
		return view.Show(pp, o, r)
	}
	return view.Toggle(pp, o, r)
}

func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	selfTest := fs.Bool("self-test-ring", false, "inject one synthetic ring, then run normally")
	dump := fs.Bool("dump-events", false, "log every raw event frame")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, c, err := loadClient()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	onRing := func() {
		rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		url, err := resolveStream(rctx, cfg, c)
		if err != nil {
			log.Println("ring: cannot resolve stream:", err)
			return
		}
		go view.Show(pidPath(), options(cfg, url), view.OSRunner{})
	}

	if *selfTest {
		log.Println("self-test: injecting synthetic ring")
		onRing()
	}

	backoff := time.Second
	for ctx.Err() == nil {
		log.Println("connecting events websocket")
		err := c.Events(ctx, func(frame []byte) {
			if *dump {
				log.Printf("event: %s", frame)
			}
			if protect.IsRing(frame) {
				log.Println("doorbell ring detected")
				onRing()
			}
		})
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("events websocket closed (%v); reconnecting in %s", err, backoff)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
	return nil
}
```

- [ ] **Step 6: Verify the whole module builds, vets, and tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build succeeds; vet clean; all unit tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/view/runner.go internal/view/runner_test.go main.go
git commit -m "feat: OS runner and trust/daemon/toggle/show subcommands"
```

---

## Task 7: Integration artifacts + install + manual verification

**Files:**
- Create: `systemd/waybar-unifi-cam.service`
- Create: `waybar/doorbell.jsonc`
- Create: `waybar/style.css`
- Create: `hyprland/windowrule.conf`
- Create: `README.md`

**Interfaces:**
- Consumes: the built binary from Task 6.
- Produces: deployment snippets + docs. No new Go code.

- [ ] **Step 1: systemd user unit**

Create `systemd/waybar-unifi-cam.service`:

```ini
[Unit]
Description=Waybar UniFi doorbell camera daemon
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=simple
ExecStart=%h/.local/bin/waybar-unifi-cam daemon
Restart=always
RestartSec=5

[Install]
WantedBy=graphical-session.target
```

- [ ] **Step 2: Waybar module snippet**

Create `waybar/doorbell.jsonc`:

```jsonc
// Merge "custom/doorbell" into "modules-right" in ~/.config/waybar/config.jsonc,
// then add this block:
"custom/doorbell": {
    "format": "󰥹",            // nf-md-doorbell (Nerd Font); change if yours differs
    "tooltip": true,
    "tooltip-format": "UniFi doorbell - click to toggle live view",
    "on-click": "waybar-unifi-cam toggle"
}
```

Create `waybar/style.css`:

```css
/* Append to ~/.config/waybar/style.css */
#custom-doorbell {
    padding: 0 10px;
    color: #89b4fa;
}
```

- [ ] **Step 3: Hyprland window rule**

Create `hyprland/windowrule.conf`:

```conf
# Source this from hyprland.conf, or paste these lines in.
# The title must match view.DefaultTitle ("unifi-doorbell").
windowrulev2 = float, title:^(unifi-doorbell)$
windowrulev2 = size 960 540, title:^(unifi-doorbell)$
windowrulev2 = center, title:^(unifi-doorbell)$
windowrulev2 = pin, title:^(unifi-doorbell)$
```

- [ ] **Step 4: README**

Create `README.md`:

````markdown
# waybar-unifi-cam

View a UniFi G4 Doorbell Pro from a Waybar icon on Hyprland, with an automatic
live-view pop-up when the doorbell rings. Uses the official UniFi Protect
Integration API (API key) for both the RTSPS stream URL and the ring event.

## Prerequisites

1. **mpv** must run: `mpv --version`. On Manjaro, if it complains about a
   missing `libcdio.so.*`, finish the system upgrade: `sudo pacman -Syu`.
2. **Protect API key:** open `https://192.168.1.1/protect/integrations/` and
   create a key.
3. **Enable RTSP** on the doorbell: Protect -> doorbell -> Settings -> Advanced
   -> enable an RTSP stream.

## Build & install

```bash
go build -o ~/.local/bin/waybar-unifi-cam .
```

## Configure

Create `~/.config/waybar-unifi-cam/config.json` (then lock it down):

```json
{
  "host": "192.168.1.1",
  "api_key": "PASTE_KEY_HERE",
  "camera": "",
  "window": { "width": 960, "height": 540, "position": "center", "always_on_top": true },
  "ring_timeout_seconds": 30,
  "audio": true
}
```

```bash
chmod 600 ~/.config/waybar-unifi-cam/config.json
```

Pin the UDM Pro certificate (one time; confirm the printed fingerprint):

```bash
waybar-unifi-cam trust
```

## Wire up Waybar & Hyprland

- Add `"custom/doorbell"` to `modules-right` and merge `waybar/doorbell.jsonc`
  into `~/.config/waybar/config.jsonc`; append `waybar/style.css` to your
  Waybar `style.css`. Reload Waybar.
- Append `hyprland/windowrule.conf` rules to your Hyprland config and reload
  (`hyprctl reload`).

## Run the daemon

The daemon spawns mpv, so it needs the Wayland environment. Two options:

**A. Hyprland exec-once (simplest, inherits the full env):**

```conf
exec-once = ~/.local/bin/waybar-unifi-cam daemon
```

**B. systemd user service:**

```bash
cp systemd/waybar-unifi-cam.service ~/.config/systemd/user/
# Make the Wayland env visible to user services (needed once per session setup):
systemctl --user import-environment WAYLAND_DISPLAY XDG_CURRENT_DESKTOP HYPRLAND_INSTANCE_SIGNATURE
systemctl --user enable --now waybar-unifi-cam.service
journalctl --user -u waybar-unifi-cam -f
```

## Verify

- Click the Waybar icon -> live view opens (video + audio); click again -> closes.
- `waybar-unifi-cam show` -> opens and auto-closes after 30s; Esc closes early.
- `waybar-unifi-cam daemon --self-test-ring` -> window pops once immediately.
- `waybar-unifi-cam daemon --dump-events` -> press the doorbell and confirm a
  frame with a `ring` type is logged (this validates the real event shape).
- Real ring -> press the doorbell -> live view pops centered.

## Troubleshooting

- **`pinned cert missing`** -> run `waybar-unifi-cam trust`.
- **401 / unauthorized** -> bad or revoked API key.
- **`no RTSPS stream`** -> enable RTSP on the camera in Protect.
- **No pop-up on ring, daemon healthy** -> run with `--dump-events` to inspect
  the real frame; if the ring field differs, update `IsRing` and the
  `testdata/ring.json` fixture.
- **Window not floating/centered** -> confirm the mpv title is `unifi-doorbell`
  and the Hyprland rules are loaded (`hyprctl clients | grep -i unifi`).
````

- [ ] **Step 5: Final build check and commit**

Run: `go build ./... && go test ./...`
Expected: PASS.

```bash
git add systemd/ waybar/ hyprland/ README.md
git commit -m "docs: deployment artifacts, install guide, and manual tests"
```

- [ ] **Step 6: Live integration verification (requires mpv fixed + config + trust done)**

Perform the checks from the README "Verify" section against the real UDM Pro.
If `--dump-events` shows a ring frame whose shape differs from
`testdata/ring.json`, update the fixture and `IsRing`, re-run
`go test ./internal/protect/`, and commit.

---

## Notes for the implementer

- Run `go test ./...` after every task; it must stay green.
- Do not add dependencies beyond `github.com/coder/websocket`.
- The `main.go` wiring is not unit-tested by design (it is thin glue); it is
  covered by the Task 7 manual integration checks and by `go build`/`go vet`.
- Missed rings during a WebSocket disconnect are accepted for v1 (no replay).
