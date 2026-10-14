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
