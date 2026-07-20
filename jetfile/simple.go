package jetfile

import (
	"context"
	"fmt"
	"time"
)

// The "1st communication format" is a lightweight ASCII framing for small
// writes: <0x01> 'Z' <2-digit address> <0x02> payload <0x04>. It is
// fire-and-forget; signs only answer when the frame ends with 0x03 and
// even then just with "OK". Useful for fast display updates where the
// full binary handshake isn't worth it.

// SendSimple sends one first-format frame containing payload (the bytes
// after <STX>) without waiting for a response.
func (c *Client) SendSimple(ctx context.Context, payload []byte) error {
	unit := c.dest.Unit % 100
	frame := make([]byte, 0, len(payload)+6)
	frame = append(frame, 0x01, 'Z', '0'+unit/10, '0'+unit%10, 0x02)
	frame = append(frame, payload...)
	frame = append(frame, 0x04)
	return c.writeRaw(ctx, frame)
}

// WriteTextSimple stores a text file fire-and-forget. target is either a
// 1-2 character label on the default disk, or a 4-character
// disk+folder+name path such as "ETAA" (file AA in the T folder of the
// RAM disk E — handy for frequent updates without flash wear).
func (c *Client) WriteTextSimple(ctx context.Context, target string, content *Text) error {
	payload := []byte{'A'}
	switch len(target) {
	case 1, 2:
		payload = append(payload, target...)
	case 4:
		payload = append(payload, 0x0F)
		payload = append(payload, target...)
	default:
		return fmt.Errorf("jetfile: target %q must be a 1-2 char label or 4 char path", target)
	}
	payload = append(payload, 0x06) // format marker: JetFile II, not ADP2.0
	payload = append(payload, content.Bytes()...)
	return c.SendSimple(ctx, payload)
}

// writeRaw writes bytes under the client lock with the usual deadline.
func (c *Client) writeRaw(ctx context.Context, wire []byte) error {
	c.io.mu.Lock()
	defer c.io.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.io.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}
	_, err := c.io.conn.Write(wire)
	return err
}
