// Command life runs Conway's Game of Life on the LED panel through the
// pixel-streaming API (0x08). It asks the sign for its size, enters stream
// mode once, then pushes one full RG frame per generation. The grid wraps
// around (a torus), and reseeds itself when it dies out or settles down so
// the display keeps moving.
//
// The panel is red/green LEDs: cells born this generation glow amber,
// survivors green.
//
//	go run . -addr 10.0.0.42
//	go run . -addr 10.0.0.42 -w 96 -h 32 -fps 12   # override panel size
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
	fps := flag.Int("fps", 10, "generations per second")
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
		w, h = 64, 16 // the sign didn't say; pick something to run on
	}
	log.Printf("running on %dx%d, %d gen/s (Ctrl-C to stop)", w, h, *fps)

	if err := c.StartStream(ctx, jetfile.MoveLeft, 0, true); err != nil {
		log.Fatalf("start stream: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c.StopStream(ctx)
	}()

	g := newGrid(w, h)
	tick := time.NewTicker(time.Second / time.Duration(max(*fps, 1)))
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("bye")
			return
		case <-tick.C:
			if err := c.StreamData(ctx, g.frame()); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("stream: %v", err)
			}
			g.advance()
		}
	}
}

type grid struct {
	w, h   int
	cells  []bool // current generation, row-major
	prev   []bool // previous generation (to color newborns)
	prev2  []bool // two generations back (to spot period-2 oscillation)
	stable int    // consecutive generations with no real change
}

const (
	density     = 0.30 // fraction of cells alive after a (re)seed
	stableLimit = 8     // reseed after this many stalled generations
)

func newGrid(w, h int) *grid {
	g := &grid{w: w, h: h}
	g.seed()
	return g
}

func (g *grid) seed() {
	n := g.w * g.h
	g.cells = make([]bool, n)
	g.prev = make([]bool, n)
	g.prev2 = nil
	g.stable = 0
	for i := range g.cells {
		g.cells[i] = rand.Float64() < density
	}
}

// advance computes the next generation, then reseeds if the board went
// extinct or settled into a still life / period-2 blinker soup.
func (g *grid) advance() {
	g.step()
	if g.population() == 0 || equal(g.cells, g.prev) || equal(g.cells, g.prev2) {
		g.stable++
	} else {
		g.stable = 0
	}
	if g.stable > stableLimit {
		g.seed()
	}
}

// step applies Conway's rules on the toroidal grid.
func (g *grid) step() {
	next := make([]bool, len(g.cells))
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			i := y*g.w + x
			n := g.liveNeighbors(x, y)
			next[i] = n == 3 || (g.cells[i] && n == 2)
		}
	}
	g.prev2, g.prev, g.cells = g.prev, g.cells, next
}

func (g *grid) liveNeighbors(x, y int) int {
	n := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx := (x + dx + g.w) % g.w
			ny := (y + dy + g.h) % g.h
			if g.cells[ny*g.w+nx] {
				n++
			}
		}
	}
	return n
}

func (g *grid) population() int {
	n := 0
	for _, c := range g.cells {
		if c {
			n++
		}
	}
	return n
}

// frame packs the live cells into a 1-bit RG buffer: row-major, 4 pixels per
// byte, first pixel in the high bits, red bit above green in each 2-bit pair.
// Rows are padded to a whole byte. Cells born this generation are amber,
// survivors green.
//
// ponytail: the packing matches the spec's description but is untested on
// hardware. If the panel shows a sheared or color-swapped picture, the three
// knobs are all here: the row stride, the shift direction, and which bit of
// the pair is red vs green.
func (g *grid) frame() []byte {
	stride := (g.w + 3) / 4
	buf := make([]byte, stride*g.h)
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			i := y*g.w + x
			if !g.cells[i] {
				continue
			}
			v := green
			if !g.prev[i] { // wasn't alive last generation
				v = amber
			}
			buf[y*stride+x/4] |= byte(v) << uint(6-2*(x%4))
		}
	}
	return buf
}

func equal(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
