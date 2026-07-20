package jetfile

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// newTestClient starts an in-memory fake sign. For every request the
// handler returns zero or more response packets (serial is stamped
// automatically unless already set).
func newTestClient(t *testing.T, handler func(req *Packet) []*Packet, opts ...Option) *Client {
	t.Helper()
	srv, cli := net.Pipe()
	t.Cleanup(func() { srv.Close(); cli.Close() })
	go func() {
		for {
			req, err := ReadPacket(srv)
			if err != nil {
				return
			}
			for _, resp := range handler(req) {
				resp.Response = true
				if resp.Serial == 0 {
					resp.Serial = req.Serial
				}
				if resp.Cmd == 0 {
					resp.Cmd = req.Cmd
				}
				wire, err := resp.Marshal()
				if err != nil {
					panic(err)
				}
				if _, err := srv.Write(wire); err != nil {
					return
				}
			}
		}
	}()
	return NewClient(cli, opts...)
}

// ok is a plain success status reply.
func ok() []*Packet {
	return []*Packet{{Flag: 1, Arg: []byte{0x00, 0x90, 0, 0}}}
}

func TestDoStampsAndReturnsResponse(t *testing.T) {
	var seen *Packet
	c := newTestClient(t, func(req *Packet) []*Packet {
		seen = req
		return []*Packet{{Flag: 0, Data: []byte("payload")}}
	}, WithAddress(3, 7), WithSource(0x0102))

	resp, err := c.Do(context.Background(), &Packet{Cmd: CmdConnectionTest})
	if err != nil {
		t.Fatal(err)
	}
	if seen.Dest != (Address{3, 7}) || seen.Source != 0x0102 || seen.Serial == 0 {
		t.Errorf("request not stamped: %+v", seen)
	}
	if string(resp.Data) != "payload" {
		t.Errorf("resp data = %q", resp.Data)
	}
}

func TestDoDeviceError(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		return []*Packet{{Flag: 1, Arg: []byte{0x08, 0x90, 0, 0}}}
	})
	_, err := c.Do(context.Background(), &Packet{Cmd: CmdRemove})
	var derr *DeviceError
	if !errors.As(err, &derr) || derr.Code != StatusFileNotFound || derr.Cmd != CmdRemove {
		t.Fatalf("got %v", err)
	}
}

func TestDoOKStatusIsNotAnError(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet { return ok() })
	if _, err := c.Do(context.Background(), &Packet{Cmd: CmdPause}); err != nil {
		t.Fatal(err)
	}
}

func TestDoSkipsStaleSerials(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		return []*Packet{
			{Serial: req.Serial + 100, Flag: 0, Data: []byte("stale")},
			{Flag: 0, Data: []byte("fresh")},
		}
	})
	resp, err := c.Do(context.Background(), &Packet{Cmd: CmdReadClock})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Data) != "fresh" {
		t.Errorf("got %q, want fresh", resp.Data)
	}
}

func TestDoContextCancel(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet { return nil }) // never answers
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := c.Do(ctx, &Packet{Cmd: CmdPause})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if time.Since(start) > time.Second {
		t.Error("cancellation did not interrupt the read")
	}
}

func TestDoTimeout(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet { return nil },
		WithTimeout(30*time.Millisecond))
	_, err := c.Do(context.Background(), &Packet{Cmd: CmdPause})
	var nerr net.Error
	if !errors.As(err, &nerr) || !nerr.Timeout() {
		t.Fatalf("got %v, want timeout", err)
	}
}

func TestDialAddsDefaultPort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	c, err := Dial(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	// no listener on port 9520 expected; just verify the port is appended
	// by checking the dial error mentions it.
	if _, err := Dial("127.0.0.1", WithTimeout(50*time.Millisecond)); err == nil {
		t.Skip("something actually listens on :9520")
	} else if !strings.Contains(err.Error(), "9520") {
		t.Errorf("dial error should mention default port: %v", err)
	}
}
