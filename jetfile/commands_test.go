package jetfile

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"
)

var ctx = context.Background()

func TestConnectionTest(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		return []*Packet{{Flag: 0, Arg: []byte{
			0x34, 0x12, // program version
			0x02, 0x01, // fpga version
			0x31, 0x0A, 0xFE, 0xA9, // 169.254.10.49, low byte first
			1, 2, // group, unit
			0, 0,
		}}}
	})
	info, err := c.ConnectionTest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.ProgramVersion != 0x1234 || info.FPGAVersion != 0x0102 {
		t.Errorf("versions: %+v", info)
	}
	if info.IP.String() != "169.254.10.49" {
		t.Errorf("ip = %s", info.IP)
	}
	if info.Addr != (Address{1, 2}) {
		t.Errorf("addr = %+v", info.Addr)
	}
}

func TestSystemParams(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		arg := make([]byte, 16)
		binary.LittleEndian.PutUint16(arg[8:], 128) // width
		binary.LittleEndian.PutUint16(arg[10:], 32) // height
		arg[14], arg[15] = 1, 7
		return []*Packet{{Flag: 0, Arg: arg}}
	})
	p, err := c.SystemParams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Width != 128 || p.Height != 32 || p.Addr != (Address{1, 7}) {
		t.Errorf("%+v", p)
	}
}

func TestSystemStatus(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		return []*Packet{{Flag: 0, Arg: []byte{2, 35, 0xFF, 1, 60, 0, 0, 0}}}
	})
	s, err := c.SystemStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != ModeBlack || s.CabinetTemp != 35 || s.OutdoorTemp != -1 ||
		!s.AutoPower || s.Humidity != 60 {
		t.Errorf("%+v", s)
	}
}

func TestSNMAC(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		data := make([]byte, 22)
		copy(data, "SN12345")
		copy(data[16:], []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x01})
		return []*Packet{{Flag: 0, Data: data}}
	})
	sn, mac, err := c.SNMAC(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sn != "SN12345" || mac.String() != "de:ad:be:ef:00:01" {
		t.Errorf("sn=%q mac=%s", sn, mac)
	}
}

func TestStartTestSubcommands(t *testing.T) {
	var got []Command
	c := newTestClient(t, func(req *Packet) []*Packet {
		got = append(got, req.Cmd)
		return ok()
	})
	if err := c.StartTest(ctx, TestAllRed); err != nil {
		t.Fatal(err)
	}
	if err := c.EndTest(ctx); err != nil {
		t.Fatal(err)
	}
	if got[0] != 0x0304 || got[1] != 0x0309 {
		t.Errorf("cmds = %v", got)
	}
}

func TestPowerStatus(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		data := make([]byte, 16)
		data[0], data[1] = 1, 1 // off with notice, driver off
		return []*Packet{{Flag: 0, Data: data}}
	})
	s, err := c.PowerStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != PowerStateOffNotice || s.DriverOn {
		t.Errorf("%+v", s)
	}
}

func TestPlayFile(t *testing.T) {
	var req *Packet
	c := newTestClient(t, func(r *Packet) []*Packet { req = r; return ok() })
	if err := c.PlayFile(ctx, FileTypeText, "AB"); err != nil {
		t.Fatal(err)
	}
	if req.Arg[0] != 'D' || req.Arg[1] != 'T' || nulString(req.Arg[2:14]) != "AB" {
		t.Errorf("arg = % X", req.Arg)
	}
}

func TestCurrentFile(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		if req.Arg[0] != 1 {
			t.Errorf("want name-only request, arg = % X", req.Arg)
		}
		return []*Packet{{Flag: 0, Arg: append([]byte(`D:\T\AB`), 0)}}
	})
	name, err := c.CurrentFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != `D:\T\AB` {
		t.Errorf("name = %q", name)
	}
}

func TestReadDir(t *testing.T) {
	entry := make([]byte, 32)
	copy(entry[0:8], "HELLO   ")
	copy(entry[8:11], "TXT")
	// 12:30:10 on 2026-07-20
	binary.LittleEndian.PutUint16(entry[22:], 12<<11|30<<5|5)
	binary.LittleEndian.PutUint16(entry[24:], (2026-1980)<<9|7<<5|20)
	binary.LittleEndian.PutUint32(entry[28:], 1234)

	dir := make([]byte, 32)
	copy(dir[0:8], "SUB     ")
	dir[11] = 0x10

	c := newTestClient(t, func(req *Packet) []*Packet {
		if nulString(req.Arg) != `D:\T\` {
			t.Errorf("path = %q", nulString(req.Arg))
		}
		return []*Packet{{
			Flag: 0,
			Arg:  []byte{2, 0, 0, 0},
			Data: append(bytes.Clone(entry), dir...),
		}}
	})
	entries, err := c.ReadDir(ctx, `D:\T`)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries", len(entries))
	}
	e := entries[0]
	if e.Name != "HELLO.TXT" || e.Dir || e.Size != 1234 {
		t.Errorf("%+v", e)
	}
	want := time.Date(2026, 7, 20, 12, 30, 10, 0, time.UTC)
	if !e.Modified.Equal(want) {
		t.Errorf("modified = %v, want %v", e.Modified, want)
	}
	if entries[1].Name != "SUB" || !entries[1].Dir {
		t.Errorf("%+v", entries[1])
	}
}

func TestDiskInfo(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		if req.Arg[0] != 'D' || req.Arg[1] != ':' {
			t.Errorf("disk arg = % X", req.Arg)
		}
		arg := make([]byte, 20)
		binary.LittleEndian.PutUint32(arg[0:], 1<<20)
		binary.LittleEndian.PutUint32(arg[4:], 1<<19)
		copy(arg[8:], "FLASH")
		return []*Packet{{Flag: 0, Arg: arg}}
	})
	d, err := c.DiskInfo(ctx, 'D')
	if err != nil {
		t.Fatal(err)
	}
	if d.Total != 1<<20 || d.Free != 1<<19 || d.Name != "FLASH" {
		t.Errorf("%+v", d)
	}
}

func TestFileExists(t *testing.T) {
	exists := true
	c := newTestClient(t, func(req *Packet) []*Packet {
		if exists {
			return ok()
		}
		return []*Packet{{Flag: 1, Arg: []byte{0x01, 0x7E, 0, 0}}}
	})
	if got, err := c.FileExists(ctx, `D:\T\A`); err != nil || !got {
		t.Errorf("got %v, %v", got, err)
	}
	exists = false
	if got, err := c.FileExists(ctx, `D:\T\A`); err != nil || got {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestLoginArgs(t *testing.T) {
	var req *Packet
	c := newTestClient(t, func(r *Packet) []*Packet { req = r; return ok() })
	if err := c.Login(ctx, "admin", "secret"); err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 20)
	copy(want, "admin")
	copy(want[14:], "secret")
	if !bytes.Equal(req.Arg, want) {
		t.Errorf("arg = % X", req.Arg)
	}
	if err := c.Login(ctx, "a-way-too-long-user", "x"); err == nil {
		t.Error("long user: want error")
	}
	if err := c.Login(ctx, "u", "toolong"); err == nil {
		t.Error("long password: want error")
	}
}

func TestStreamStatus(t *testing.T) {
	c := newTestClient(t, func(req *Packet) []*Packet {
		return []*Packet{{Flag: 1, Arg: []byte{0x01, 0x83, 0, 0}}}
	})
	code, err := c.StreamStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if code != StatusStreamBuf1 {
		t.Errorf("code = %v", code)
	}
}

func TestStreamDataChunks(t *testing.T) {
	var reqs []*Packet
	c := newTestClient(t, func(r *Packet) []*Packet { reqs = append(reqs, r); return ok() })
	if err := c.StreamData(ctx, make([]byte, 600)); err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 2 {
		t.Fatalf("got %d packets", len(reqs))
	}
	if qty := binary.LittleEndian.Uint16(reqs[0].Arg[2:]); qty != 2 {
		t.Errorf("qty = %d", qty)
	}
	if cur := binary.LittleEndian.Uint16(reqs[1].Arg[4:]); cur != 2 {
		t.Errorf("current = %d", cur)
	}
}

func TestSetBuzzer(t *testing.T) {
	var req *Packet
	c := newTestClient(t, func(r *Packet) []*Packet { req = r; return ok() })
	if err := c.SetBuzzer(ctx, true, BuzzOnFileChange, 3); err != nil {
		t.Fatal(err)
	}
	if req.Arg[0] != 1 || req.Arg[1] != 1 || req.Arg[2] != '3' {
		t.Errorf("arg = % X", req.Arg)
	}
}
