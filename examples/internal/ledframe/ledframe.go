// Package ledframe packs pixels into the 1-bit RG layout JetFile II signs
// expect for pixel streaming (StartStream with oneBitRG). It is shared by the
// streaming examples so the packing lives in exactly one place.
package ledframe

// Pixel codes: bit1 = red LED, bit0 = green LED. A red/green panel can show
// these four states.
const (
	Off   = 0
	Green = 1
	Red   = 2
	Amber = 3
)

// Buf is a panel-sized frame in the 1-bit RG packed layout: row-major, 4
// pixels per byte, the first pixel in the high bits, the red bit above green
// in each 2-bit pair. Rows are padded to a whole byte.
//
// ponytail: the packing matches the spec's description but is untested on
// hardware. If a real panel shows a sheared or color-swapped image, the three
// knobs are all here: the row stride, the shift direction, and which bit of
// the pair is red vs green. Fix it once and every streaming example follows.
type Buf struct {
	W, H   int
	stride int
	b      []byte
}

// New returns a cleared frame for a w×h panel.
func New(w, h int) *Buf {
	stride := (w + 3) / 4
	return &Buf{W: w, H: h, stride: stride, b: make([]byte, stride*h)}
}

// Set lights pixel (x, y) with code v (Off/Green/Red/Amber). Coordinates
// outside the panel are ignored.
func (f *Buf) Set(x, y, v int) {
	if x < 0 || x >= f.W || y < 0 || y >= f.H {
		return
	}
	f.b[y*f.stride+x/4] |= byte(v) << uint(6-2*(x%4))
}

// Bytes returns the packed frame, ready for Client.StreamData.
func (f *Buf) Bytes() []byte { return f.b }
