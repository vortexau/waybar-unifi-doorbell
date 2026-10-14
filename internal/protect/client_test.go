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
		if r.Header.Get("X-API-KEY") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
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

func TestFindDoorbellAutoDetectFailure(t *testing.T) {
	cams := []Camera{{ID: "c", Name: "Garage"}}
	if _, err := FindDoorbell(cams, ""); err == nil {
		t.Fatal("expected error when no doorbell can be auto-detected")
	}
}
