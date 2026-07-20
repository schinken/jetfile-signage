package ledframe

import "testing"

// TestPacking pins the RG bit layout: pixel 0 in the top two bits (red above
// green), pixel 2 two slots lower, rows padded to a byte.
func TestPacking(t *testing.T) {
	f := New(4, 1)
	f.Set(0, 0, Amber) // 0b11 at bits 7-6 => 0xC0
	f.Set(2, 0, Red)   // 0b10 at bits 3-2 => 0x08
	buf := f.Bytes()
	if len(buf) != 1 {
		t.Fatalf("len = %d, want 1", len(buf))
	}
	if buf[0] != 0xC8 {
		t.Fatalf("packed byte = %#02x, want 0xC8", buf[0])
	}
}

func TestStridePadsRows(t *testing.T) {
	f := New(5, 2) // (5+3)/4 = 2 bytes per row, 2 rows
	if len(f.Bytes()) != 4 {
		t.Fatalf("len = %d, want 4", len(f.Bytes()))
	}
	f.Set(4, 1, Green) // last pixel, second row => byte index 2*1+4/4 = 3
	if f.Bytes()[3] != 0x40 {
		t.Fatalf("byte 3 = %#02x, want 0x40", f.Bytes()[3])
	}
}

func TestSetIgnoresOutOfRange(t *testing.T) {
	f := New(4, 1)
	f.Set(9, 9, Amber)
	f.Set(-1, 0, Amber)
	for _, b := range f.Bytes() {
		if b != 0 {
			t.Fatalf("out-of-range Set wrote %#02x", b)
		}
	}
}
