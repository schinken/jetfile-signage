package main

import (
	"testing"

	"github.com/schinken/jetfile-signage/examples/internal/ledframe"
)

// TestRenderThreshold: pixels at or above the threshold light up.
func TestRenderThreshold(t *testing.T) {
	gray := []byte{200, 10, 200, 10} // 4x1
	buf := render(gray, 4, 1, 128, false, ledframe.Green)
	// pixels 0 and 2 lit green: 0x40 | 0x04 = 0x44
	if buf[0] != 0x44 {
		t.Fatalf("byte = %#02x, want 0x44", buf[0])
	}
}

// TestRenderInvert: with -invert, the dark pixels light instead.
func TestRenderInvert(t *testing.T) {
	gray := []byte{200, 10, 200, 10}
	buf := render(gray, 4, 1, 128, true, ledframe.Green)
	// pixels 1 and 3 lit green: 0x10 | 0x01 = 0x11
	if buf[0] != 0x11 {
		t.Fatalf("byte = %#02x, want 0x11", buf[0])
	}
}
