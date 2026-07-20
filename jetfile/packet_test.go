package jetfile

import (
	"bytes"
	"errors"
	"testing"
)

func TestChecksum(t *testing.T) {
	if got := checksum([]byte{1, 2, 3}); got != 6 {
		t.Errorf("checksum = %d, want 6", got)
	}
	// truncates to 16 bits: 300*0xFF = 76500 -> 76500-65536 = 10964
	if got := checksum(bytes.Repeat([]byte{0xFF}, 300)); got != 10964 {
		t.Errorf("checksum = %d, want 10964", got)
	}
}

func TestMarshalGolden(t *testing.T) {
	p := &Packet{Serial: 1, Cmd: CmdConnectionTest}
	got, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x55, 0xA7, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x01, 0x00, 0x03, 0x01, 0x00, 0x00,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got  % X\nwant % X", got, want)
	}
}

func TestMarshalGoldenArgPaddingAndData(t *testing.T) {
	p := &Packet{
		Serial: 0x1234,
		Source: 0x0102,
		Dest:   Address{Group: 3, Unit: 4},
		Cmd:    CmdWriteTextFile,
		Flag:   1,
		Arg:    []byte{0xFF, 0x01, 0x02}, // padded to 4 on the wire
		Data:   []byte("AB"),
	}
	got, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0x55, 0xA7, 0xDF, 0x01, 0x02, 0x00, 0x02, 0x01,
		0x03, 0x04, 0x34, 0x12, 0x02, 0x04, 0x01, 0x01,
		0xFF, 0x01, 0x02, 0x00, 0x41, 0x42,
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got  % X\nwant % X", got, want)
	}
}

func TestRoundtrip(t *testing.T) {
	pkts := []*Packet{
		{Serial: 1, Cmd: CmdConnectionTest},
		{Response: true, Serial: 0xFFFF, Cmd: CmdReadClock, Flag: 1, Arg: []byte{0, 0x90, 0, 0}},
		{Serial: 7, Dest: Address{1, 2}, Cmd: CmdWriteFile, Arg: bytes.Repeat([]byte{9}, 24), Data: bytes.Repeat([]byte{0xAB}, 512)},
	}
	for _, p := range pkts {
		b, err := p.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		got, err := ReadPacket(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%v: %v", p.Cmd, err)
		}
		if got.Response != p.Response || got.Serial != p.Serial || got.Source != p.Source ||
			got.Dest != p.Dest || got.Cmd != p.Cmd || got.Flag != p.Flag {
			t.Errorf("header mismatch: got %+v want %+v", got, p)
		}
		wantArg := make([]byte, (len(p.Arg)+3)/4*4)
		copy(wantArg, p.Arg)
		if !bytes.Equal(got.Arg, wantArg) || !bytes.Equal(got.Data, p.Data) {
			t.Errorf("%v: payload mismatch", p.Cmd)
		}
	}
}

func TestReadPacketErrors(t *testing.T) {
	good, _ := (&Packet{Serial: 1, Cmd: CmdConnectionTest}).Marshal()

	bad := bytes.Clone(good)
	bad[1] = 0x00
	if _, err := ReadPacket(bytes.NewReader(bad)); !errors.Is(err, ErrBadSync) {
		t.Errorf("bad sync: got %v", err)
	}

	bad = bytes.Clone(good)
	bad[2]++
	if _, err := ReadPacket(bytes.NewReader(bad)); !errors.Is(err, ErrBadChecksum) {
		t.Errorf("bad checksum: got %v", err)
	}

	long, _ := (&Packet{Cmd: CmdWriteFile, Data: []byte("hello")}).Marshal()
	if _, err := ReadPacket(bytes.NewReader(long[:len(long)-2])); err == nil {
		t.Error("truncated body: want error, got nil")
	}
}

func TestPacketStatus(t *testing.T) {
	p := &Packet{Flag: 1, Arg: []byte{0x00, 0x90, 0, 0}}
	if code, ok := p.Status(); !ok || code != StatusOK || !code.OK() {
		t.Errorf("got %v %v", code, ok)
	}
	p = &Packet{Flag: 1, Data: []byte{'O', 'K'}}
	if code, ok := p.Status(); !ok || code != StatusOKText || !code.OK() {
		t.Errorf("got %v %v", code, ok)
	}
	p = &Packet{Flag: 0, Data: []byte{1, 2}}
	if _, ok := p.Status(); ok {
		t.Error("flag 0 must not be a status")
	}
}

func TestDeviceError(t *testing.T) {
	err := error(&DeviceError{Cmd: CmdRemove, Code: StatusFileNotFound})
	if !errors.Is(err, &DeviceError{Code: StatusFileNotFound}) {
		t.Error("errors.Is by code failed")
	}
	if errors.Is(err, &DeviceError{Code: StatusDiskFull}) {
		t.Error("errors.Is matched wrong code")
	}
	want := "jetfile: command 0x0706 failed: file not found (0x9008)"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}
