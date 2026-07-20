package main

import "testing"

// TestFramePacking pins the RG bit layout: pixel 0 in the top two bits (red
// above green), pixel 2 two slots lower, rows padded to a byte.
func TestFramePacking(t *testing.T) {
	g := &game{panelW: 4, panelH: 1, w: 4, h: 1, snake: []point{{0, 0}}, food: point{2, 0}}
	buf := g.frame()

	if len(buf) != 1 { // (4+3)/4 = 1 byte per row, 1 row
		t.Fatalf("len = %d, want 1", len(buf))
	}
	// pixel 0 = head = amber (0b11) at bits 7-6 => 0xC0
	// pixel 2 = food = red   (0b10) at bits 3-2 => 0x08
	if got := buf[0]; got != 0xC8 {
		t.Fatalf("packed byte = %#02x, want 0xC8", got)
	}
}

// TestHamiltonianCycle verifies buildCycle produces a genuine cycle: a
// permutation of every cell, unit steps throughout, returning to the start.
func TestHamiltonianCycle(t *testing.T) {
	sizes := []struct{ w, h int }{
		{2, 2}, {4, 4}, {6, 4}, {4, 6}, {8, 6}, {10, 4}, {6, 10}, {80, 7}, {7, 80},
	}
	for _, s := range sizes {
		order, next, ok := buildCycle(s.w, s.h)
		if !ok {
			t.Fatalf("%dx%d: expected a cycle", s.w, s.h)
		}
		n := s.w * s.h

		seen := make([]bool, n)
		for _, o := range order {
			if o < 0 || o >= n || seen[o] {
				t.Fatalf("%dx%d: order is not a permutation", s.w, s.h)
			}
			seen[o] = true
		}

		visited := make([]bool, n)
		cur := 0
		for k := 0; k < n; k++ {
			if visited[cur] {
				t.Fatalf("%dx%d: revisits a cell before covering all", s.w, s.h)
			}
			visited[cur] = true
			nx := next[cur]
			if abs(cur%s.w-nx%s.w)+abs(cur/s.w-nx/s.w) != 1 {
				t.Fatalf("%dx%d: non-adjacent step %d->%d", s.w, s.h, cur, nx)
			}
			cur = nx
		}
		if cur != 0 {
			t.Fatalf("%dx%d: does not return to start", s.w, s.h)
		}
	}
}

// TestSolverNeverDies plays full games to completion on several boards: a
// correct Hamiltonian solver wins every time (fills the board) without ever
// colliding, whatever the random food sequence.
func TestSolverNeverDies(t *testing.T) {
	sizes := []struct{ w, h int }{{4, 4}, {6, 4}, {8, 6}, {6, 8}, {10, 6}}
	for _, s := range sizes {
		for run := 0; run < 40; run++ {
			g := newGame(s.w, s.h)
			n := s.w * s.h
			budget := n * n * 20
			winner := false
			for step := 0; step < budget && !winner; step++ {
				switch g.step() {
				case died:
					t.Fatalf("%dx%d run %d: snake died at step %d", s.w, s.h, run, step)
				case won:
					winner = true
				}
				if dup(g.snake) {
					t.Fatalf("%dx%d run %d: self-overlap at step %d", s.w, s.h, run, step)
				}
			}
			if !winner {
				t.Fatalf("%dx%d run %d: did not fill the board within %d steps", s.w, s.h, run, budget)
			}
		}
	}
}

func dup(ps []point) bool {
	seen := make(map[point]bool, len(ps))
	for _, p := range ps {
		if seen[p] {
			return true
		}
		seen[p] = true
	}
	return false
}
