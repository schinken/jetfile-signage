package jetfile

import (
	"bytes"
	"testing"
	"time"
)

func TestTimezoneCodes(t *testing.T) {
	cases := []struct {
		offset int
		want   byte
	}{
		{0, 0x0C},
		{2 * 3600, 0x0E},
		{-12 * 3600, 0x00},
		{13 * 3600, 0x19},
		{5*3600 + 1800, 0x1B}, // +5:30 special
		{9*3600 + 1800, 0x1E}, // +9:30 special
		{4*3600 + 1800, 0x11}, // +4:30 has no code: round to +5
		{14 * 3600, 0x19},     // clamped to +13
		{-13 * 3600, 0x00},    // clamped to -12
	}
	for _, tc := range cases {
		if got := timezoneCode(tc.offset); got != tc.want {
			t.Errorf("timezoneCode(%d) = 0x%02X, want 0x%02X", tc.offset, got, tc.want)
		}
	}
	// round trips for the exact codes
	for _, off := range []int{-12 * 3600, 0, 3600, 5*3600 + 2700, 13 * 3600} {
		if got := timezoneOffset(timezoneCode(off)); got != off {
			t.Errorf("round trip %d -> %d", off, got)
		}
	}
}

func TestSetClockGolden(t *testing.T) {
	var req *Packet
	c := newTestClient(t, func(r *Packet) []*Packet { req = r; return ok() })

	when := time.Date(2026, 7, 20, 15, 4, 5, 0, time.FixedZone("CEST", 2*3600)) // a Monday
	if err := c.SetClock(ctx, when); err != nil {
		t.Fatal(err)
	}
	if req.Cmd != CmdSetClockExt {
		t.Errorf("cmd = %v", req.Cmd)
	}
	want := []byte{0x26, 0x20, 0x07, 0x20, 0x15, 0x04, 0x05, 0x02, 0x0E, 0, 0, 0}
	if !bytes.Equal(req.Arg, want) {
		t.Errorf("arg  = % X\nwant   % X", req.Arg, want)
	}
}

func TestSetClockFallsBackToLegacy(t *testing.T) {
	var cmds []Command
	c := newTestClient(t, func(req *Packet) []*Packet {
		cmds = append(cmds, req.Cmd)
		if req.Cmd == CmdSetClockExt {
			return []*Packet{{Flag: 1, Arg: []byte{0x05, 0x90, 0, 0}}} // bad sub command
		}
		if len(req.Arg) != 8 {
			t.Errorf("legacy arg length = %d, want 8", len(req.Arg))
		}
		return ok()
	})
	if err := c.SetClock(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 || cmds[0] != CmdSetClockExt || cmds[1] != CmdSetClock {
		t.Errorf("cmds = %v", cmds)
	}
}

func TestClockRead(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		return []*Packet{{Flag: 0, Arg: []byte{0x26, 0x20, 0x07, 0x20, 0x15, 0x04, 0x01, 0x0D}}}
	})
	got, err := c.Clock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 20, 15, 4, 0, 0, time.FixedZone("UTC+1", 3600))
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
