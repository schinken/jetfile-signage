// Command snake plays Snake on the LED panel through the pixel-streaming
// API (0x08). It asks the sign for its panel size, enters stream mode once,
// then pushes one full RG frame per tick. The snake plays itself and
// restarts when it dies.
//
// The panel is red/green LEDs, so every pixel is off, red, green or amber.
//
//	go run . -addr 10.0.0.42
//	go run . -addr 10.0.0.42 -w 96 -h 32 -fps 12   # override panel size
//
// The solver follows a Hamiltonian cycle over the panel, so it never traps
// itself — it fills the board and starts over. See chooseDir.
//
// Hardware note: this library is untested on a real sign. The 1-bit RG
// packing (bit order, row padding) follows the spec's description but may
// need calibration — see frame().
package main

import (
	"context"
	"flag"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"time"

	"github.com/schinken/jetfile-signage/jetfile"
)

// 2-bit pixel codes: bit1 = red LED, bit0 = green LED.
const (
	off   = 0
	green = 1
	red   = 2
	amber = 3
)

func main() {
	addr := flag.String("addr", "", "sign address, host or host:port")
	wFlag := flag.Int("w", 0, "panel width in pixels (0 = ask the sign)")
	hFlag := flag.Int("h", 0, "panel height in pixels (0 = ask the sign)")
	fps := flag.Int("fps", 8, "game speed in frames per second")
	flag.Parse()
	if *addr == "" {
		log.Fatal("-addr is required")
	}

	c, err := jetfile.Dial(*addr)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	w, h := *wFlag, *hFlag
	if w == 0 || h == 0 {
		if p, err := c.SystemParams(ctx); err == nil && p.Width > 0 && p.Height > 0 {
			w, h = int(p.Width), int(p.Height)
		}
	}
	if w <= 0 || h <= 0 {
		w, h = 64, 16 // the sign didn't say; pick something playable
	}
	log.Printf("playing on %dx%d, %d fps (Ctrl-C to stop)", w, h, *fps)

	// One RG frame per push; speed 0 = display as fast as the sign accepts.
	if err := c.StartStream(ctx, jetfile.MoveLeft, 0, true); err != nil {
		log.Fatalf("start stream: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c.StopStream(ctx)
	}()

	g := newGame(w, h)
	tick := time.NewTicker(time.Second / time.Duration(max(*fps, 1)))
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("bye")
			return
		case <-tick.C:
			g.step()
			if err := c.StreamData(ctx, g.frame()); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("stream: %v", err)
			}
		}
	}
}

type point struct{ x, y int }

var dirs4 = []point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

type outcome int

const (
	ongoing outcome = iota
	died
	won
)

type game struct {
	panelW, panelH int      // physical panel, what frame() renders into
	w, h           int      // play field == the Hamiltonian-cycle area
	snake          []point  // head at index 0
	dir            point
	food           point
	order, next    []int    // cycle: order[cell] = position, next[cell] = successor
	ok             bool     // a cycle exists; otherwise fall back to greedy
}

func newGame(panelW, panelH int) *game {
	g := &game{panelW: panelW, panelH: panelH, w: panelW, h: panelH}
	g.order, g.next, g.ok = buildCycle(g.w, g.h)
	if !g.ok { // both dimensions odd: play the largest even sub-board, dark edge
		if g.w%2 == 1 && g.w > 2 {
			g.w--
		} else if g.h%2 == 1 && g.h > 2 {
			g.h--
		}
		g.order, g.next, g.ok = buildCycle(g.w, g.h)
	}
	g.reset()
	return g
}

func (g *game) reset() {
	g.snake = []point{{g.w / 2, g.h / 2}}
	g.dir = point{1, 0}
	g.placeFood()
}

func (g *game) placeFood() {
	for {
		f := point{rand.IntN(g.w), rand.IntN(g.h)}
		if !g.occupied(f) {
			g.food = f
			return
		}
	}
}

// step advances one tick: pick a heading, move the head, eat or shuffle the
// tail. Collisions restart the game (died); filling the board is a win.
func (g *game) step() outcome {
	g.dir = g.chooseDir()
	head := g.snake[0]
	nh := point{head.x + g.dir.x, head.y + g.dir.y}
	if nh.x < 0 || nh.x >= g.w || nh.y < 0 || nh.y >= g.h || g.hitsBody(nh) {
		g.reset()
		return died
	}
	g.snake = append([]point{nh}, g.snake...)
	if nh == g.food {
		if len(g.snake) >= g.w*g.h {
			g.reset()
			return won
		}
		g.placeFood()
		return ongoing
	}
	g.snake = g.snake[:len(g.snake)-1] // didn't eat: tail follows
	return ongoing
}

// chooseDir steers along a Hamiltonian cycle of the board. Following the
// cycle blindly never collides but is slow, so it takes the furthest-ahead
// shortcut the safety budget allows: advance at most maxJump steps along the
// cycle, keeping a margin behind the tail so a shortcut can never seal the
// snake in. The d==1 move (the plain cycle successor) is always available and
// always free, so a legal move always exists — the snake never dies.
func (g *game) chooseDir() point {
	if !g.ok {
		return g.greedyDir() // tiny/odd board with no cycle
	}
	head := g.snake[0]
	hi := g.idx(head)
	n := g.w * g.h
	dist := func(from, to int) int { // steps from `from` to `to` along the cycle
		if d := g.order[to] - g.order[from]; d >= 0 {
			return d
		} else {
			return d + n
		}
	}
	distTail := n // room before we'd reach the tail
	if len(g.snake) > 1 {
		distTail = dist(hi, g.idx(g.snake[len(g.snake)-1]))
	}
	distFood := dist(hi, g.idx(g.food))

	cut := distTail - 3 // how far past the plain cycle step we may jump
	if n-len(g.snake) < n/2 {
		cut = 0 // crowded: hug the cycle, no shortcuts
	}
	if cut < 0 {
		cut = 0
	}
	maxJump := 1 + cut
	if maxJump > distFood {
		maxJump = distFood // never jump past the food, or we'd never eat it
	}

	target, best := -1, 0
	for _, nb := range g.neighbors(head) {
		if g.hitsBody(nb) { // the tail cell is fair game — it vacates as we move
			continue
		}
		if d := dist(hi, g.idx(nb)); d >= 1 && d <= maxJump && d > best {
			best, target = d, g.idx(nb)
		}
	}
	if target == -1 {
		return g.dir // only reachable on a cycle-less board; move collides, resets
	}
	t := g.cell(target)
	return point{t.x - head.x, t.y - head.y}
}

// greedyDir is the fallback for boards too small or oddly-shaped for a cycle:
// step toward the food along the safe move that shrinks the distance most.
func (g *game) greedyDir() point {
	head := g.snake[0]
	best, bestScore := g.dir, 1<<30
	for _, d := range dirs4 {
		if d == (point{-g.dir.x, -g.dir.y}) {
			continue
		}
		nh := point{head.x + d.x, head.y + d.y}
		if nh.x < 0 || nh.x >= g.w || nh.y < 0 || nh.y >= g.h || g.hitsBody(nh) {
			continue
		}
		if score := abs(nh.x-g.food.x) + abs(nh.y-g.food.y); score < bestScore {
			best, bestScore = d, score
		}
	}
	return best
}

func (g *game) idx(p point) int  { return p.y*g.w + p.x }
func (g *game) cell(i int) point { return point{i % g.w, i / g.w} }

func (g *game) neighbors(p point) []point {
	out := make([]point, 0, 4)
	for _, d := range dirs4 {
		n := point{p.x + d.x, p.y + d.y}
		if n.x >= 0 && n.x < g.w && n.y >= 0 && n.y < g.h {
			out = append(out, n)
		}
	}
	return out
}

func (g *game) occupied(p point) bool {
	for _, s := range g.snake {
		if s == p {
			return true
		}
	}
	return false
}

// hitsBody reports whether p collides with the snake, ignoring the tail cell
// (which vacates as the head advances).
func (g *game) hitsBody(p point) bool {
	for i := 0; i < len(g.snake)-1; i++ {
		if g.snake[i] == p {
			return true
		}
	}
	return false
}

// buildCycle returns a Hamiltonian cycle over a w×h grid as order[cell] (its
// position in the cycle) and next[cell] (its successor). It works whenever
// one side is even and both are ≥2; otherwise ok is false.
func buildCycle(w, h int) (order, next []int, ok bool) {
	if w < 2 || h < 2 {
		return nil, nil, false
	}
	var seq []point
	switch {
	case h%2 == 0:
		seq = serpentine(w, h)
	case w%2 == 0: // transpose: build with even height, then swap coordinates
		for _, p := range serpentine(h, w) {
			seq = append(seq, point{p.y, p.x})
		}
	default:
		return nil, nil, false // both odd: no cycle exists
	}
	n := w * h
	order = make([]int, n)
	next = make([]int, n)
	for k, p := range seq {
		order[p.y*w+p.x] = k
	}
	for k, p := range seq {
		np := seq[(k+1)%n]
		next[p.y*w+p.x] = np.y*w + np.x
	}
	return order, next, true
}

// serpentine lays out a Hamiltonian cycle for an even height h: right across
// the top row, snake down through columns 1..w-1, then up column 0 back to
// the start.
func serpentine(w, h int) []point {
	seq := make([]point, 0, w*h)
	for x := 0; x < w; x++ { // top row, left to right
		seq = append(seq, point{x, 0})
	}
	for y := 1; y < h; y++ { // rows below zig-zag over columns 1..w-1
		if y%2 == 1 {
			for x := w - 1; x >= 1; x-- {
				seq = append(seq, point{x, y})
			}
		} else {
			for x := 1; x < w; x++ {
				seq = append(seq, point{x, y})
			}
		}
	}
	for y := h - 1; y >= 1; y-- { // column 0, bottom to top, closes the loop
		seq = append(seq, point{0, y})
	}
	return seq
}

// frame renders the board to a 1-bit RG packed buffer: row-major, 4 pixels
// per byte, first pixel in the high bits, red bit above green in each 2-bit
// pair. Each row is padded to a whole byte.
//
// ponytail: the packing matches the spec's description but is untested on
// hardware. If the panel shows a sheared or color-swapped picture, the three
// knobs are all here: the row stride, the shift direction, and which bit of
// the pair is red vs green.
func (g *game) frame() []byte {
	stride := (g.panelW + 3) / 4
	buf := make([]byte, stride*g.panelH)
	set := func(x, y, v int) {
		buf[y*stride+x/4] |= byte(v) << uint(6-2*(x%4))
	}
	for _, s := range g.snake {
		set(s.x, s.y, green)
	}
	set(g.snake[0].x, g.snake[0].y, amber) // head stands out
	set(g.food.x, g.food.y, red)
	return buf
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
