package handlers

import (
	"testing"
)

func TestMsToNs(t *testing.T) {
	if msToNs(0) != 0 || msToNs(-5) != 0 {
		t.Fatal("non-positive ms must map to 0")
	}
	if msToNs(1737504000000) != 1737504000000000000 {
		t.Fatalf("got %d", msToNs(1737504000000))
	}
}
