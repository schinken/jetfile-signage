package jetfile

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"
)

// chunkSize is the payload per write/read packet. The spec recommends 512
// or 1024 and caps packets at 1024 bytes of data.
const chunkSize = 512

// fileLabel pads a file name to the 12-byte label field.
func fileLabel(label string) ([12]byte, error) {
	var l [12]byte
	if label == "" || len(label) > 12 {
		return l, fmt.Errorf("jetfile: file label %q must be 1-12 characters", label)
	}
	copy(l[:], label)
	return l, nil
}

// labelChar is the single-character label used in the stored file header.
func labelChar(label string) byte {
	if label == "" {
		return '0'
	}
	return label[0]
}

// WriteTextFile stores content as text file label in the T folder of the
// client's partition and (re)plays it according to the playlist.
func (c *Client) WriteTextFile(ctx context.Context, label string, content *Text) error {
	return c.writeLabeled(ctx, CmdWriteTextFile, label, content.file('A', labelChar(label)))
}

// WriteStringFile stores content as string file label in the S folder.
// String files don't display on their own; text files embed them via
// Text.InsertString.
func (c *Client) WriteStringFile(ctx context.Context, label string, content *Text) error {
	return c.writeLabeled(ctx, CmdWriteStringFile, label, content.file('G', labelChar(label)))
}

// WritePictureFile stores a BMP (RG tri-color, 16/256 colors, 16 or 24 bit)
// as picture file label in the P folder.
func (c *Client) WritePictureFile(ctx context.Context, label string, bmp []byte) error {
	return c.writeLabeled(ctx, CmdWritePictureFile, label, bmp)
}

// WriteSystemFile stores a system file such as CONFIG.SYS, SEQUENT.SYS
// (playlist) or RUNTIME.SYS.
func (c *Client) WriteSystemFile(ctx context.Context, name string, data []byte) error {
	lab, err := fileLabel(name)
	if err != nil {
		return err
	}
	return c.writeChunked(ctx, CmdWriteSystemFile, len(data), func(arg []byte) {
		copy(arg[0:12], lab[:])
	}, 24, 12, data)
}

// WriteFile writes data to an explicit path like `D:\T\WELCOME.NMG`.
func (c *Client) WriteFile(ctx context.Context, path string, data []byte) error {
	if path == "" {
		return fmt.Errorf("jetfile: empty path")
	}
	argLen := 10 + len(path) + 1
	return c.writeChunked(ctx, CmdWriteFile, len(data), func(arg []byte) {
		copy(arg[10:], path)
	}, argLen, 0, data)
}

// writeLabeled sends the shared 24-byte argument layout of the labeled
// write commands (0x0204..0x0207).
func (c *Client) writeLabeled(ctx context.Context, cmd Command, label string, file []byte) error {
	lab, err := fileLabel(label)
	if err != nil {
		return err
	}
	partition := c.partition
	return c.writeChunked(ctx, cmd, len(file), func(arg []byte) {
		arg[0] = partition
		arg[1] = 0 // buzzer count / reserved
		copy(arg[2:14], lab[:])
	}, 24, 14, file)
}

// writeChunked splits data into chunkSize packets and sends them in order.
// fill writes the command-specific prefix into the argument; sizeOff is
// where the common [4]total [2]packet size [2]quantity [2]current block
// starts, argLen the total argument length.
func (c *Client) writeChunked(ctx context.Context, cmd Command, total int, fill func(arg []byte), argLen, sizeOff int, data []byte) error {
	qty := (total + chunkSize - 1) / chunkSize
	if qty == 0 {
		qty = 1
	}
	if qty > 0xFFFF {
		return fmt.Errorf("jetfile: file too big (%d bytes)", total)
	}
	for i := 0; i < qty; i++ {
		part := data[i*chunkSize : min((i+1)*chunkSize, total)]
		arg := make([]byte, argLen)
		fill(arg)
		binary.LittleEndian.PutUint32(arg[sizeOff:], uint32(total))
		binary.LittleEndian.PutUint16(arg[sizeOff+4:], chunkSize)
		binary.LittleEndian.PutUint16(arg[sizeOff+6:], uint16(qty))
		binary.LittleEndian.PutUint16(arg[sizeOff+8:], uint16(i+1))
		if err := c.exec(ctx, cmd, arg, part); err != nil {
			return fmt.Errorf("packet %d/%d: %w", i+1, qty, err)
		}
	}
	return nil
}

// ReadTextFile reads text file label from the T folder of the client's
// partition. The returned bytes are the stored file including its header
// and EOF marker.
func (c *Client) ReadTextFile(ctx context.Context, label string) ([]byte, error) {
	return c.readLabeled(ctx, CmdReadTextFile, label)
}

// ReadStringFile reads string file label from the S folder.
func (c *Client) ReadStringFile(ctx context.Context, label string) ([]byte, error) {
	return c.readLabeled(ctx, CmdReadStringFile, label)
}

// ReadPictureFile reads picture file label from the P folder.
func (c *Client) ReadPictureFile(ctx context.Context, label string) ([]byte, error) {
	return c.readLabeled(ctx, CmdReadPictureFile, label)
}

// ReadSystemFile reads a system file such as CONFIG.SYS or SEQUENT.SYS.
func (c *Client) ReadSystemFile(ctx context.Context, name string) ([]byte, error) {
	lab, err := fileLabel(name)
	if err != nil {
		return nil, err
	}
	return c.readPaged(ctx, CmdReadSystemFile, func(page uint16) []byte {
		arg := make([]byte, 16)
		copy(arg[0:12], lab[:])
		binary.LittleEndian.PutUint16(arg[12:], chunkSize)
		binary.LittleEndian.PutUint16(arg[14:], page)
		return arg
	})
}

// ReadFile reads a file from an explicit path like `D:\T\WELCOME.NMG`.
func (c *Client) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("jetfile: empty path")
	}
	return c.readPaged(ctx, CmdReadFile, func(page uint16) []byte {
		arg := make([]byte, 4+len(path)+1)
		binary.LittleEndian.PutUint16(arg[0:], chunkSize)
		binary.LittleEndian.PutUint16(arg[2:], page)
		copy(arg[4:], path)
		return arg
	})
}

func (c *Client) readLabeled(ctx context.Context, cmd Command, label string) ([]byte, error) {
	lab, err := fileLabel(label)
	if err != nil {
		return nil, err
	}
	partition := c.partition
	return c.readPaged(ctx, cmd, func(page uint16) []byte {
		arg := make([]byte, 20)
		arg[0] = partition
		copy(arg[4:16], lab[:])
		binary.LittleEndian.PutUint16(arg[16:], chunkSize)
		binary.LittleEndian.PutUint16(arg[18:], page)
		return arg
	})
}

// readPaged requests page 1, 2, ... until the file size announced in the
// response has been received.
func (c *Client) readPaged(ctx context.Context, cmd Command, arg func(page uint16) []byte) ([]byte, error) {
	var out []byte
	for page := uint16(1); ; page++ {
		resp, err := c.query(ctx, cmd, arg(page))
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Data...)

		// response arg: [2]file size, [2]page, [4]file size for big files
		total := -1
		if len(resp.Arg) >= 2 {
			total = int(binary.LittleEndian.Uint16(resp.Arg))
		}
		if len(resp.Arg) >= 8 {
			if big := binary.LittleEndian.Uint32(resp.Arg[4:]); int(big) > total {
				total = int(big)
			}
		}
		if total >= 0 && len(out) >= total {
			return out[:total], nil
		}
		if len(resp.Data) == 0 { // no progress; don't loop forever
			return out, nil
		}
	}
}

// WriteEmergency displays a message immediately for the given duration
// (0 = until removed), optionally sounding the buzzer. The message must fit
// in a single packet (1024 bytes).
func (c *Client) WriteEmergency(ctx context.Context, content *Text, ttl time.Duration, sound bool) error {
	file := content.file('A', '0')
	if len(file) > 1024 {
		return fmt.Errorf("jetfile: emergency message too big (%d bytes, max 1024)", len(file))
	}
	secs := int64(ttl / time.Second)
	if secs > 0xFFFF {
		secs = 0xFFFF
	}
	arg := make([]byte, 4)
	binary.LittleEndian.PutUint16(arg[0:], uint16(secs))
	if sound {
		arg[2] = 1
	}
	return c.exec(ctx, CmdWriteEmergency, arg, file)
}
