package jetfile

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestWriteTextFileSinglePacket(t *testing.T) {
	var reqs []*Packet
	c := newTestClient(t, func(req *Packet) []*Packet {
		reqs = append(reqs, req)
		return ok()
	})

	err := c.WriteTextFile(context.Background(), "0", NewText().Str("This is a sample"))
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 1 {
		t.Fatalf("got %d packets, want 1", len(reqs))
	}
	req := reqs[0]
	if req.Cmd != CmdWriteTextFile {
		t.Errorf("cmd = %v", req.Cmd)
	}
	wantFile := append([]byte{0x01, 'Z', '0', '0', 0x02, 'A', '0'}, "This is a sample"...)
	wantFile = append(wantFile, 0x04)
	if !bytes.Equal(req.Data, wantFile) {
		t.Errorf("data = % X\nwant   % X", req.Data, wantFile)
	}
	arg := req.Arg
	if arg[0] != 'D' || arg[1] != 0 {
		t.Errorf("partition/buzzer = %c %d", arg[0], arg[1])
	}
	if got := nulString(arg[2:14]); got != "0" {
		t.Errorf("label = %q", got)
	}
	if total := binary.LittleEndian.Uint32(arg[14:]); int(total) != len(wantFile) {
		t.Errorf("total = %d, want %d", total, len(wantFile))
	}
	if qty := binary.LittleEndian.Uint16(arg[20:]); qty != 1 {
		t.Errorf("qty = %d", qty)
	}
	if cur := binary.LittleEndian.Uint16(arg[22:]); cur != 1 {
		t.Errorf("current = %d", cur)
	}
}

func TestWritePictureFileChunking(t *testing.T) {
	var reqs []*Packet
	c := newTestClient(t, func(req *Packet) []*Packet {
		reqs = append(reqs, req)
		return ok()
	}, WithPartition('E'))

	data := bytes.Repeat([]byte{0xAA}, 1300) // 512 + 512 + 276
	if err := c.WritePictureFile(context.Background(), "P1", data); err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 3 {
		t.Fatalf("got %d packets, want 3", len(reqs))
	}
	var joined []byte
	for i, req := range reqs {
		arg := req.Arg
		if arg[0] != 'E' {
			t.Errorf("packet %d: partition %c", i, arg[0])
		}
		if total := binary.LittleEndian.Uint32(arg[14:]); total != 1300 {
			t.Errorf("packet %d: total %d", i, total)
		}
		if qty := binary.LittleEndian.Uint16(arg[20:]); qty != 3 {
			t.Errorf("packet %d: qty %d", i, qty)
		}
		if cur := binary.LittleEndian.Uint16(arg[22:]); int(cur) != i+1 {
			t.Errorf("packet %d: current %d", i, cur)
		}
		joined = append(joined, req.Data...)
	}
	if !bytes.Equal(joined, data) {
		t.Error("reassembled data differs")
	}
}

func TestWriteFilePath(t *testing.T) {
	var req *Packet
	c := newTestClient(t, func(r *Packet) []*Packet { req = r; return ok() })
	if err := c.WriteFile(context.Background(), `D:\T\WELCOME.NMG`, []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if got := nulString(req.Arg[10:]); got != `D:\T\WELCOME.NMG` {
		t.Errorf("path = %q", got)
	}
	if string(req.Data) != "hi" {
		t.Errorf("data = %q", req.Data)
	}
}

func TestReadTextFilePaged(t *testing.T) {
	content := append([]byte{0x01, 'Z', '0', '0', 0x02, 'A', 'A'},
		bytes.Repeat([]byte{'x'}, 700)...) // forces two 512-byte pages
	content = append(content, 0x04)

	c := newTestClient(t, func(req *Packet) []*Packet {
		if req.Cmd != CmdReadTextFile {
			t.Errorf("cmd = %v", req.Cmd)
		}
		if req.Arg[0] != 'D' || nulString(req.Arg[4:16]) != "AB" {
			t.Errorf("bad request arg: % X", req.Arg)
		}
		size := int(binary.LittleEndian.Uint16(req.Arg[16:]))
		page := int(binary.LittleEndian.Uint16(req.Arg[18:]))
		lo := (page - 1) * size
		hi := min(lo+size, len(content))
		arg := make([]byte, 8)
		binary.LittleEndian.PutUint16(arg[0:], uint16(len(content)))
		binary.LittleEndian.PutUint16(arg[2:], uint16(page))
		return []*Packet{{Flag: 0, Arg: arg, Data: content[lo:hi]}}
	})

	got, err := c.ReadTextFile(context.Background(), "AB")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("got %d bytes, want %d", len(got), len(content))
	}
}

func TestReadFileNotFound(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		return []*Packet{{Flag: 1, Arg: []byte{0x08, 0x90, 0, 0}}}
	})
	_, err := c.ReadFile(context.Background(), `D:\T\NOPE`)
	var derr *DeviceError
	if !errors.As(err, &derr) || derr.Code != StatusFileNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestFileLabelValidation(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet { return ok() })
	if err := c.WriteTextFile(context.Background(), "", NewText()); err == nil {
		t.Error("empty label: want error")
	}
	if err := c.WriteTextFile(context.Background(), strings.Repeat("A", 13), NewText()); err == nil {
		t.Error("13-char label: want error")
	}
}

func TestWriteEmergency(t *testing.T) {
	var req *Packet
	c := newTestClient(t, func(r *Packet) []*Packet { req = r; return ok() })
	err := c.WriteEmergency(context.Background(), NewText().Str("FIRE"), 90e9, true)
	if err != nil {
		t.Fatal(err)
	}
	if ttl := binary.LittleEndian.Uint16(req.Arg); ttl != 90 {
		t.Errorf("ttl = %d", ttl)
	}
	if req.Arg[2] != 1 {
		t.Error("sound flag not set")
	}
	if !bytes.Contains(req.Data, []byte("FIRE")) {
		t.Error("message content missing")
	}

	big := NewText().Str(strings.Repeat("x", 1100))
	if err := c.WriteEmergency(context.Background(), big, 0, false); err == nil {
		t.Error("oversized message: want error")
	}
}
