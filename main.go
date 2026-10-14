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
