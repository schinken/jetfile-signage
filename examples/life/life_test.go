package main

import "testing"

// emptyGrid builds a grid with the given live cells and no random seed.
func emptyGrid(w, h int, alive ...[2]int) *grid {
	g := &grid{w: w, h: h, cells: make([]bool, w*h), prev: make([]bool, w*h)}
	for _, p := range alive {
		g.cells[p[1]*w+p[0]] = true
	}
	return g
}

func aliveEq(t *testing.T, g *grid, want ...[2]int) {
	t.Helper()
	exp := make([]bool, g.w*g.h)
	for _, p := range want {
		exp[p[1]*g.w+p[0]] = true
	}
	if !equal(g.cells, exp) {
		t.Fatalf("cells = %v, want alive at %v", g.cells, want)
	}
}

// TestBlinker: a horizontal 3-cell bar oscillates to vertical and back.
func TestBlinker(t *testing.T) {
	g := emptyGrid(5, 5, [2]int{1, 2}, [2]int{2, 2}, [2]int{3, 2})
	g.step()
	aliveEq(t, g, [2]int{2, 1}, [2]int{2, 2}, [2]int{2, 3})
	g.step()
	aliveEq(t, g, [2]int{1, 2}, [2]int{2, 2}, [2]int{3, 2})
}

// TestBlock: the 2x2 block is a still life — unchanged after a generation.
func TestBlock(t *testing.T) {
	g := emptyGrid(4, 4, [2]int{1, 1}, [2]int{2, 1}, [2]int{1, 2}, [2]int{2, 2})
	g.step()
	aliveEq(t, g, [2]int{1, 1}, [2]int{2, 1}, [2]int{1, 2}, [2]int{2, 2})
}

// TestGlider: a glider translates by (1,1) every four generations.
func TestGlider(t *testing.T) {
	start := [][2]int{{1, 0}, {2, 1}, {0, 2}, {1, 2}, {2, 2}}
	g := emptyGrid(8, 8, start...)
	for i := 0; i < 4; i++ {
		g.step()
	}
	want := make([][2]int, len(start))
	for i, p := range start {
		want[i] = [2]int{p[0] + 1, p[1] + 1}
	}
	aliveEq(t, g, want...)
}

// TestFramePacking pins the RG bit layout: pixel 0 amber (born) in the top
// two bits, pixel 2 green (survivor) two slots lower.
func TestFramePacking(t *testing.T) {
	g := &grid{
		w: 4, h: 1,
		cells: []bool{true, false, true, false},
		prev:  []bool{false, false, true, false}, // pixel 0 just born, pixel 2 survived
	}
	buf := g.frame()
	if len(buf) != 1 {
		t.Fatalf("len = %d, want 1", len(buf))
	}
	// pixel 0 = amber (0b11) at bits 7-6 => 0xC0
	// pixel 2 = green (0b01) at bits 3-2 => 0x04
	if got := buf[0]; got != 0xC4 {
		t.Fatalf("packed byte = %#02x, want 0xC4", got)
	}
}
