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
