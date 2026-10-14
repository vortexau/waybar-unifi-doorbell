# Ring Flash + Last-Ring Tooltip Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Waybar doorbell icon flash briefly (auto-clearing) on a ring and show the last-ring time in its hover tooltip.

**Architecture:** The daemon writes a last-ring timestamp to a state file and sends `SIGRTMIN+N` to Waybar on ring (and again when the flash window ends). A new `status` subcommand renders `{class, tooltip}` JSON from that state file, which the Waybar `custom/doorbell` module runs as its `exec`. The flash is pure CSS keyed on the `.ringing` class; the glyph stays in the module's literal `format`.

**Tech Stack:** Go 1.26 (stdlib only for this feature), existing `github.com/coder/websocket` dep unchanged, Waybar custom module (`exec` + `signal`), `pkill` for signalling.

## Global Constraints

- Go module `waybar-unifi-cam`, Go directive `go 1.26`. No new third-party dependencies (stdlib only).
- `status` must ALWAYS print valid JSON on stdout and exit 0, even when the state file is missing/unreadable (a custom module that errors shows a broken/blank entry in Waybar).
- `status` uses `config.Load` only — never `loadClient`; it needs no pinned cert or network.
- Flash window = the existing `ring_timeout_seconds` (default 30s). No separate flash config.
- New config key `waybar_signal` (JSON `waybar_signal`), default `8` when `<= 0`.
- State file: `$XDG_RUNTIME_DIR/waybar-unifi-cam.ring` (epoch seconds), mode `0600`; fall back to `os.TempDir()` when `XDG_RUNTIME_DIR` is unset.
- mpv window title literal `unifi-doorbell` (unchanged); click still runs `waybar-unifi-cam toggle`.
- TDD: failing test first, watch it fail, implement minimally, watch it pass, commit.
- Every commit message ends with:
  ```
  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_0124bLKhm4WuCjoqz8AWhs5M
  ```

---

## Task 1: Config — `waybar_signal` field + default

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: existing `config.Config`, `config.Load`.
- Produces: `Config.WaybarSignal int` (`json:"waybar_signal"`), defaulted to `8` by `Load` when `<= 0`.

- [ ] **Step 1: Write the failing test**

Add to `internal/config/config_test.go`:

```go
func TestLoadWaybarSignalDefault(t *testing.T) {
	p := writeConfig(t, `{"host":"h","api_key":"k"}`, 0o600)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.WaybarSignal != 8 {
		t.Fatalf("expected default waybar_signal 8, got %d", c.WaybarSignal)
	}
}

func TestLoadWaybarSignalExplicit(t *testing.T) {
	p := writeConfig(t, `{"host":"h","api_key":"k","waybar_signal":5}`, 0o600)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.WaybarSignal != 5 {
		t.Fatalf("expected waybar_signal 5, got %d", c.WaybarSignal)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadWaybarSignal`
Expected: FAIL — `c.WaybarSignal undefined`.

- [ ] **Step 3: Add the field and default**

In `internal/config/config.go`, add the field to the `Config` struct (after `Audio`):

```go
	Audio              bool   `json:"audio"`
	WaybarSignal       int    `json:"waybar_signal"`
```

And in `Load`, alongside the other defaults (after the `RingTimeoutSeconds` default), add:

```go
	if c.WaybarSignal <= 0 {
		c.WaybarSignal = 8
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/`
Expected: PASS (existing tests plus the two new ones).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add waybar_signal with default 8"
```

---

## Task 2: `ringstate` package

**Files:**
- Create: `internal/ringstate/ringstate.go`
- Test: `internal/ringstate/ringstate_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `ringstate.Path() string`
  - `ringstate.Write(t time.Time) error`
  - `ringstate.Read() (time.Time, bool)` — `ok=false` on missing/malformed/unreadable.

- [ ] **Step 1: Write the failing test**

Create `internal/ringstate/ringstate_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ringstate/`
Expected: FAIL — `undefined: Write` (build error).

- [ ] **Step 3: Write the implementation**

Create `internal/ringstate/ringstate.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ringstate/`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/ringstate/
git commit -m "feat(ringstate): persist last-ring timestamp in XDG_RUNTIME_DIR"
```

---

## Task 3: `status` package — pure render

**Files:**
- Create: `internal/status/status.go`
- Test: `internal/status/status_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `type status.Line struct { Class string `json:"class"`; Tooltip string `json:"tooltip"` }`
  - `status.Render(lastRing, now time.Time, flash time.Duration) Line`

- [ ] **Step 1: Write the failing test**

Create `internal/status/status_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/status/`
Expected: FAIL — `undefined: Render` (build error).

- [ ] **Step 3: Write the implementation**

Create `internal/status/status.go`:

```go
// Package status renders the Waybar custom-module JSON for the doorbell icon
// from the last-ring time. It is pure: no files, no clock, no I/O — the caller
// supplies lastRing and now so the logic is fully testable.
package status

import "time"

// Line is the JSON a Waybar custom module consumes. The glyph lives in the
// module's `format`, so only class (for the flash CSS) and tooltip are set here.
type Line struct {
	Class   string `json:"class"`
	Tooltip string `json:"tooltip"`
}

// Render decides the icon state. A zero lastRing means "never rung this
// session". A ring newer than flash is "ringing"; otherwise the tooltip shows
// when it last rang.
func Render(lastRing, now time.Time, flash time.Duration) Line {
	switch {
	case lastRing.IsZero():
		return Line{Tooltip: "UniFi doorbell - click for live view"}
	case now.Sub(lastRing) < flash:
		return Line{Class: "ringing", Tooltip: "Doorbell ringing now"}
	default:
		return Line{Tooltip: "Last ring: " + lastRing.Format("Mon 15:04")}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/status/`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/status/
git commit -m "feat(status): pure Render for doorbell module JSON"
```

---

## Task 4: Wire into `main` — `status` subcommand, signalling, onRing flash

**Files:**
- Modify: `main.go`

**Interfaces:**
- Consumes: `config.Load`, `config.DefaultPath`, `config.Config.WaybarSignal`, `config.Config.RingTimeoutSeconds`, `ringstate.Write`, `ringstate.Read`, `status.Render`, `status.Line`.
- Produces: a `status` subcommand; `signalWaybar(n int)`; flash behaviour inside `cmdDaemon`'s `onRing`.

- [ ] **Step 1: Add imports**

In `main.go`, add to the import block: `"encoding/json"`, `"os/exec"`, and the two internal packages. The import block should include (alongside the existing entries):

```go
	"encoding/json"
	"os/exec"

	"waybar-unifi-cam/internal/ringstate"
	"waybar-unifi-cam/internal/status"
```

- [ ] **Step 2: Add the `status` case to the dispatch switch**

In `main()`, add a case alongside `trust|daemon|toggle|show`:

```go
	case "status":
		err = cmdStatus()
```

Also update the usage string to include `status`:

```go
		fmt.Fprintln(os.Stderr, "usage: waybar-unifi-cam {trust|daemon|toggle|show|status}")
```

- [ ] **Step 3: Add `signalWaybar` and `cmdStatus`**

Add these functions to `main.go` (e.g. near `pidPath`):

```go
// signalWaybar tells Waybar to re-run the doorbell module's exec by sending
// SIGRTMIN+n. Errors are ignored: Waybar may not be running, which is fine.
func signalWaybar(n int) {
	if n <= 0 {
		n = 8
	}
	_ = exec.Command("pkill", fmt.Sprintf("-RTMIN+%d", n), "waybar").Run()
}

// cmdStatus prints the Waybar custom-module JSON for the doorbell icon. It never
// errors out: a missing config or state file still yields valid JSON so Waybar
// never shows a broken module.
func cmdStatus() error {
	flash := 30 * time.Second
	if cfg, err := config.Load(config.DefaultPath()); err == nil {
		flash = time.Duration(cfg.RingTimeoutSeconds) * time.Second
	}
	last, _ := ringstate.Read()
	b, err := json.Marshal(status.Render(last, time.Now(), flash))
	if err != nil {
		// Should never happen for this struct; emit a safe fallback.
		fmt.Println(`{"class":"","tooltip":"UniFi doorbell"}`)
		return nil
	}
	fmt.Println(string(b))
	return nil
}
```

- [ ] **Step 4: Add the flash to `onRing`**

In `cmdDaemon`, replace the existing `onRing` definition:

```go
	onRing := func() {
		go func() {
			rctx, cancel := context.WithTimeout(ctx, resolveTimeout)
			defer cancel()
			url, err := resolveStream(rctx, cfg, c)
			if err != nil {
				log.Println("ring: cannot resolve stream:", err)
				return
			}
			if err := view.Show(pidPath(), options(cfg, url), view.OSRunner{}); err != nil {
				log.Println("ring: show failed:", err)
			}
		}()
	}
```

with (records the ring + flashes immediately, independent of stream resolution):

```go
	flash := time.Duration(cfg.RingTimeoutSeconds) * time.Second
	onRing := func() {
		if err := ringstate.Write(time.Now()); err != nil {
			log.Println("ring: cannot write ring state:", err)
		}
		signalWaybar(cfg.WaybarSignal) // flash on
		go func() {                    // flash off after the window
			select {
			case <-ctx.Done():
			case <-time.After(flash):
				signalWaybar(cfg.WaybarSignal)
			}
		}()
		go func() {
			rctx, cancel := context.WithTimeout(ctx, resolveTimeout)
			defer cancel()
			url, err := resolveStream(rctx, cfg, c)
			if err != nil {
				log.Println("ring: cannot resolve stream:", err)
				return
			}
			if err := view.Show(pidPath(), options(cfg, url), view.OSRunner{}); err != nil {
				log.Println("ring: show failed:", err)
			}
		}()
	}
```

- [ ] **Step 5: Build, vet, and sanity-check `status` output**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build + vet clean; all unit tests pass.

Then verify `status` prints valid JSON in the never-rung case (no state file in a throwaway runtime dir):

Run:
```bash
XDG_RUNTIME_DIR=$(mktemp -d) go run . status
```
Expected: exactly `{"class":"","tooltip":"UniFi doorbell - click for live view"}`

- [ ] **Step 6: Commit**

```bash
git add main.go
git commit -m "feat: status subcommand and ring flash via SIGRTMIN signal"
```

---

## Task 5: Waybar artifacts, Makefile template, and README

**Files:**
- Modify: `waybar/doorbell.jsonc`
- Modify: `waybar/style.css`
- Modify: `Makefile` (config template)
- Modify: `README.md`

**Interfaces:**
- Consumes: the `status`/`toggle` subcommands and the `waybar_signal` config key.
- Produces: deployment snippets and docs. No Go code.

- [ ] **Step 1: Update the Waybar module snippet**

Replace the contents of `waybar/doorbell.jsonc` with:

```jsonc
// Merge "custom/doorbell" into a modules-* array in ~/.config/waybar/config.jsonc,
// then add this definition block. The icon flashes on a ring (CSS .ringing class)
// and the tooltip shows the last-ring time; both are driven by `status`, which
// Waybar re-runs when the daemon sends SIGRTMIN+<signal>.
"custom/doorbell": {
    "exec": "waybar-unifi-cam status",
    "return-type": "json",
    "format": "󰥹",
    "signal": 8,
    "on-click": "waybar-unifi-cam toggle"
}
```

- [ ] **Step 2: Add the flash CSS to the style snippet**

Append to `waybar/style.css`:

```css

/* Flash while the doorbell is ringing (the `status` subcommand sets the
   .ringing class). Adjust colours to your theme; @define-color users might
   use @surface/@green/@bg-deep as below. */
@keyframes doorbell-flash {
    0%, 100% { background: @surface; color: @green; }
    50%      { background: @green;   color: @bg-deep; }
}
#custom-doorbell.ringing {
    animation: doorbell-flash 0.8s ease-in-out infinite;
}
```

- [ ] **Step 3: Add `waybar_signal` to the Makefile config template**

In `Makefile`, inside the `define CONFIG_TEMPLATE` block, add the key before the closing brace so it reads:

```make
  "ring_timeout_seconds": 30,
  "audio": true,
  "waybar_signal": 8
}
```

(Note the comma after `"audio": true`.)

- [ ] **Step 4: Update the README**

In `README.md`, update the config JSON example (the one under "Configure") to include `"waybar_signal": 8` (add a comma after `"audio": true`). Then, in the Waybar wiring section where the module block is shown, replace the static block with the `exec`/`signal` block from Step 1 and add this sentence after it:

```markdown
The icon flashes while the doorbell is ringing and its tooltip shows the last
ring time. This is driven by the `status` subcommand, which Waybar re-runs when
the daemon sends `SIGRTMIN+<waybar_signal>` (default 8 — change `waybar_signal`
in config.json and `"signal"` in the module together if 8 collides). Append the
`doorbell-flash` rule from `waybar/style.css` to your stylesheet.
```

- [ ] **Step 5: Verify nothing Go broke and commit**

Run: `go build ./... && go test ./...`
Expected: PASS (artifacts are not Go code; this just confirms no accidental breakage).

```bash
git add waybar/doorbell.jsonc waybar/style.css Makefile README.md
git commit -m "docs: ring-flash Waybar module, CSS, config template, README"
```

---

## Deploy & verify on the live system (controller/operator step)

Not a code task — apply the change to the running desktop after the tasks above are merged. The user has authorised editing their live `~/.config/waybar/config.jsonc`.

- [ ] Reinstall the binary: `make install-bin`.
- [ ] Update live `~/.config/waybar/config.jsonc`: replace the static `"custom/doorbell"` block with the `exec`/`signal` block (keep the glyph the user already has).
- [ ] Append the `doorbell-flash` keyframes + `#custom-doorbell.ringing` rule to `~/.config/waybar/style.css` (idempotent: skip if already present).
- [ ] Add `"waybar_signal": 8` to `~/.config/waybar-unifi-cam/config.json` (optional; default applies without it).
- [ ] Reload Waybar (`killall -SIGUSR2 waybar` / `make reload`) and restart the daemon (`systemctl --user restart waybar-unifi-cam`).
- [ ] Verify with `waybar-unifi-cam daemon --self-test-ring` (or press the doorbell): the icon pulses for ~30s then clears; hovering shows "Last ring: …".

---

## Notes for the implementer

- Run `go test ./...` after every task; keep it green.
- No new dependencies — stdlib only.
- `main.go` additions are thin glue, covered by `go build`/`go vet` plus the Step-5 `status` sanity check and the manual deploy verification; the `onRing` flash path is exercised by `daemon --self-test-ring`.
