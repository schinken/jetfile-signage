package jetfile

import (
	"bytes"
	"context"
	"net"
	"testing"
)

// rawClient captures raw wire bytes instead of parsing packets.
func rawClient(t *testing.T, opts ...Option) (*Client, <-chan []byte) {
	t.Helper()
	srv, cli := net.Pipe()
	t.Cleanup(func() { srv.Close(); cli.Close() })
	got := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := srv.Read(buf)
		got <- buf[:n]
	}()
	return NewClient(cli, opts...), got
}

func TestWriteTextSimpleRAMPath(t *testing.T) {
	c, got := rawClient(t)
	err := c.WriteTextSimple(context.Background(), "ETAA", NewText().Str("Hi"))
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("\x01Z00\x02A\x0FETAA\x06Hi\x04")
	if g := <-got; !bytes.Equal(g, want) {
		t.Errorf("got  % X\nwant % X", g, want)
	}
}

func TestWriteTextSimpleLabelAndAddress(t *testing.T) {
	c, got := rawClient(t, WithAddress(0, 7))
	if err := c.WriteTextSimple(context.Background(), "0", NewText().Str("x")); err != nil {
		t.Fatal(err)
	}
	want := []byte("\x01Z07\x02A0\x06x\x04")
	if g := <-got; !bytes.Equal(g, want) {
		t.Errorf("got  % X\nwant % X", g, want)
	}
}

func TestWriteTextSimpleBadTarget(t *testing.T) {
	c, _ := rawClient(t)
	if err := c.WriteTextSimple(context.Background(), "TOOLONG", NewText()); err == nil {
		t.Error("want error for bad target length")
	}
}
