package main

import "testing"

// TestFramePacking pins the RG bit layout: pixel 0 in the top two bits (red
// above green), pixel 2 two slots lower, rows padded to a byte.
func TestFramePacking(t *testing.T) {
	g := &game{w: 4, h: 1, snake: []point{{0, 0}}, food: point{2, 0}}
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

func TestStepEatsAndGrows(t *testing.T) {
	g := &game{w: 5, h: 1, snake: []point{{1, 0}}, dir: point{1, 0}, food: point{2, 0}}
	g.step()

	if len(g.snake) != 2 {
		t.Fatalf("length = %d, want 2 after eating", len(g.snake))
	}
	if g.snake[0] != (point{2, 0}) {
		t.Fatalf("head = %+v, want {2 0}", g.snake[0])
	}
}

func TestStepMovesWithoutGrowing(t *testing.T) {
	g := &game{w: 5, h: 2, snake: []point{{1, 0}}, dir: point{1, 0}, food: point{4, 1}}
	g.step()

	if len(g.snake) != 1 {
		t.Fatalf("length = %d, want 1 (no food eaten)", len(g.snake))
	}
	if g.snake[0] != (point{2, 0}) {
		t.Fatalf("head = %+v, want {2 0}", g.snake[0])
	}
}

// TestWallCollisionResets: heading into a wall restarts the game.
func TestWallCollisionResets(t *testing.T) {
	g := &game{w: 3, h: 1, snake: []point{{2, 0}}, dir: point{1, 0}, food: point{0, 0}}
	g.step() // greedy would turn back, but reversing is banned -> hits the wall

	if len(g.snake) != 1 || g.snake[0] != (point{1, 0}) {
		t.Fatalf("after reset: %+v, want single snake at center {1 0}", g.snake)
	}
}
