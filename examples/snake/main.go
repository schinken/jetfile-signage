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
// Hardware note: this library is untested on a real sign. The 1-bit RG
// packing (bit order, row padding) and the stream-frame semantics follow
// the spec's description but may need calibration — see frame().
package main

import (
	"context"
	"flag"
	"log"
	"math"
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

type game struct {
	w, h  int
	snake []point // head at index 0
	dir   point
	food  point
}

func newGame(w, h int) *game {
	g := &game{w: w, h: h}
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
// tail, and restart on any collision.
func (g *game) step() {
	g.dir = g.chooseDir()
	head := g.snake[0]
	nh := point{head.x + g.dir.x, head.y + g.dir.y}
	if nh.x < 0 || nh.x >= g.w || nh.y < 0 || nh.y >= g.h || g.hitsBody(nh) {
		g.reset()
		return
	}
	g.snake = append([]point{nh}, g.snake...)
	if nh == g.food {
		if len(g.snake) >= g.w*g.h { // board full — you win, start over
			g.reset()
			return
		}
		g.placeFood()
		return
	}
	g.snake = g.snake[:len(g.snake)-1] // didn't eat: tail follows
}

// chooseDir is a greedy heuristic: step toward the food along whichever safe
// move shrinks the distance most, never reversing into the neck.
//
// ponytail: greedy, no lookahead — the snake can wall itself in with its own
// body. It just dies and resets; a real solver would flood-fill for a safe
// path (e.g. Hamiltonian cycle).
func (g *game) chooseDir() point {
	head := g.snake[0]
	best, bestScore := g.dir, math.MaxInt
	for _, d := range []point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		if d == (point{-g.dir.x, -g.dir.y}) {
			continue // no reversing
		}
		nh := point{head.x + d.x, head.y + d.y}
		if nh.x < 0 || nh.x >= g.w || nh.y < 0 || nh.y >= g.h || g.hitsBody(nh) {
			continue
		}
		if score := abs(nh.x-g.food.x) + abs(nh.y-g.food.y); score < bestScore {
			best, bestScore = d, score
		}
	}
	return best // if nothing was safe, keep going and die next step
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

// frame renders the board to a 1-bit RG packed buffer: row-major, 4 pixels
// per byte, first pixel in the high bits, red bit above green in each 2-bit
// pair. Each row is padded to a whole byte.
//
// ponytail: the packing matches the spec's description but is untested on
// hardware. If the panel shows a sheared or color-swapped picture, the three
// knobs are all here: the row stride, the shift direction, and which bit of
// the pair is red vs green.
func (g *game) frame() []byte {
	stride := (g.w + 3) / 4
	buf := make([]byte, stride*g.h)
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
