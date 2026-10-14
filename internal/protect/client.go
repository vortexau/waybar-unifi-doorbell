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
