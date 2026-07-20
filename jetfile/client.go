package jetfile

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// DefaultPort is the TCP/UDP port JetFile II signs listen on.
const DefaultPort = 9520

// Client talks to one LED sign over a net.Conn. It is safe for concurrent
// use; requests are serialized because the protocol is strict
// request/response. Address a specific sign (or broadcast) with To.
type Client struct {
	io *ioConn // shared across To variants so they serialize on one conn

	source    uint16
	dest      Address
	timeout   time.Duration
	partition byte
}

// ioConn is the connection state shared by a Client and its To variants.
type ioConn struct {
	conn   net.Conn
	r      *bufio.Reader
	mu     sync.Mutex
	serial uint16
}

// Option configures a Client.
type Option func(*Client)

// WithAddress sets the destination sign address (default: broadcast 0/0).
func WithAddress(group, unit byte) Option {
	return func(c *Client) { c.dest = Address{Group: group, Unit: unit} }
}

// WithTimeout sets the per-request I/O timeout (default 5s).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithPartition sets the default disk partition for file commands.
// 'D' (flash, default) survives power cycles; 'E' (RAM) avoids flash wear
// for frequently updated content.
func WithPartition(p byte) Option {
	return func(c *Client) { c.partition = p }
}

// WithSource sets the source address field (default 0, rarely needed).
func WithSource(s uint16) Option {
	return func(c *Client) { c.source = s }
}

// NewClient wraps an existing connection (e.g. a *net.UDPConn for signs
// that only speak UDP).
func NewClient(conn net.Conn, opts ...Option) *Client {
	c := &Client{
		io:        &ioConn{conn: conn, r: bufio.NewReader(conn)},
		timeout:   5 * time.Second,
		partition: 'D',
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Dial connects to a sign over TCP. addr without a port defaults to :9520.
func Dial(addr string, opts ...Option) (*Client, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, fmt.Sprint(DefaultPort))
	}
	var probe Client
	probe.timeout = 5 * time.Second
	for _, o := range opts {
		o(&probe)
	}
	conn, err := net.DialTimeout("tcp", addr, probe.timeout)
	if err != nil {
		return nil, err
	}
	return NewClient(conn, opts...), nil
}

// Close closes the underlying connection. Closing any To variant closes the
// shared connection for all of them.
func (c *Client) Close() error { return c.io.conn.Close() }

// To returns a Client that sends to the given group/unit address while
// sharing this client's connection and request serialization. Use it to
// address one sign on a shared bus, or To(0, 0) to broadcast, without
// disturbing the original client:
//
//	c.To(1, 4).WriteTextFile(ctx, "0", msg) // this sign
//	c.To(0, 0).SetClock(ctx, time.Now())    // every sign on the bus
func (c *Client) To(group, unit byte) *Client {
	nc := *c // shares io (pointer); dest/source/timeout/partition are copies
	nc.dest = Address{Group: group, Unit: unit}
	return &nc
}

// Do sends p and returns the sign's response.
//
// It stamps Serial, Source and Dest from the client — use To to change the
// destination (To(0, 0) broadcasts). If p.Flag is FlagNoReply, Do writes the
// packet and returns (nil, nil) without waiting; otherwise it reads the reply
// and, if that reply carries a non-success status, returns both the packet
// and a *DeviceError.
//
// Use Do for protocol commands without a typed wrapper:
//
//	resp, err := c.Do(ctx, &jetfile.Packet{Cmd: 0x0902})
func (c *Client) Do(ctx context.Context, p *Packet) (*Packet, error) {
	c.io.mu.Lock()
	defer c.io.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.io.conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { c.io.conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()

	c.io.serial++
	p.Response = false
	p.Serial = c.io.serial
	p.Source = c.source
	p.Dest = c.dest

	wire, err := p.Marshal()
	if err != nil {
		return nil, err
	}
	if _, err := c.io.conn.Write(wire); err != nil {
		return nil, ctxErr(ctx, err)
	}

	if p.Flag == FlagNoReply {
		return nil, nil // caller asked for no reply; nothing to read
	}

	// Skip stale frames (wrong serial), e.g. late answers to a timed-out
	// request still sitting in the stream.
	for skipped := 0; ; skipped++ {
		if skipped > 16 {
			return nil, fmt.Errorf("jetfile: no response for serial %d", p.Serial)
		}
		resp, err := ReadPacket(c.io.r)
		if err != nil {
			return nil, ctxErr(ctx, err)
		}
		if resp.Serial != p.Serial {
			continue
		}
		if code, ok := resp.Status(); ok && !code.OK() {
			return resp, &DeviceError{Cmd: p.Cmd, Code: code}
		}
		return resp, nil
	}
}

// exec runs a command built from cmd, arg and data and expects a plain
// status reply.
func (c *Client) exec(ctx context.Context, cmd Command, arg, data []byte) error {
	_, err := c.Do(ctx, &Packet{Cmd: cmd, Arg: arg, Data: data})
	return err
}

// query runs a command and returns the response packet.
func (c *Client) query(ctx context.Context, cmd Command, arg []byte) (*Packet, error) {
	return c.Do(ctx, &Packet{Cmd: cmd, Arg: arg})
}

// ctxErr prefers the context's error over a net timeout caused by
// cancellation.
func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// nulString cuts b at the first NUL byte and trims trailing spaces.
func nulString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimRight(string(b), " ")
}
