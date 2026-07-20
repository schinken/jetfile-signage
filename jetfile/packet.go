// Package jetfile implements the JetFile II protocol spoken by Chainzone /
// Texcellent LED signs (spec: JetFileII v2.5.4). Signs listen on TCP/UDP
// port 9520.
//
// The main entry point is Client, obtained via Dial or NewClient. Commands
// without a typed wrapper can be sent through Client.Do using a raw Packet.
package jetfile

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Command is a main/sub command pair as listed in the spec, e.g. 0x0301 is
// main command 0x03 (test) with sub command 0x01 (connection test).
type Command uint16

// Main and sub command bytes of c.
func (c Command) Main() byte { return byte(c >> 8) }
func (c Command) Sub() byte  { return byte(c) }

func (c Command) String() string { return fmt.Sprintf("0x%04X", uint16(c)) }

// Commands of the binary ("2nd") communication format.
const (
	CmdReadSystemFile   Command = 0x0102
	CmdReadFontFile     Command = 0x0103
	CmdReadTextFile     Command = 0x0104
	CmdReadStringFile   Command = 0x0105
	CmdReadPictureFile  Command = 0x0106
	CmdReadArrayPicture Command = 0x0107
	CmdReadFile         Command = 0x0108
	CmdReadPlayLog      Command = 0x0109
	CmdReadSystemParams Command = 0x010A
	CmdReadSystemStatus Command = 0x010B
	CmdReadSNMAC        Command = 0x010C
	CmdReadSystemInfo   Command = 0x0112
	CmdReadErrorLog     Command = 0x0113

	CmdWriteSystemFile   Command = 0x0202
	CmdWriteFontFile     Command = 0x0203
	CmdWriteTextFile     Command = 0x0204
	CmdWriteStringFile   Command = 0x0205
	CmdWritePictureFile  Command = 0x0206
	CmdWriteArrayPicture Command = 0x0207
	CmdWriteFile         Command = 0x0208
	CmdWriteEmergency    Command = 0x0209
	CmdWriteBrightness   Command = 0x020A

	CmdConnectionTest Command = 0x0301
	CmdEndTest        Command = 0x0309
	CmdGrayscaleTest  Command = 0x030A

	CmdReset       Command = 0x0400
	CmdBlackScreen Command = 0x0401
	CmdEndBlack    Command = 0x0402
	CmdPowerOff    Command = 0x0403
	CmdPowerOn     Command = 0x0404
	CmdPowerStatus Command = 0x0405

	CmdReadClock   Command = 0x0501
	CmdSetClock    Command = 0x0502
	CmdEnvironment Command = 0x0503
	CmdSetClockExt Command = 0x0504

	CmdRestartPlaylist Command = 0x0601
	CmdReplayCurrent   Command = 0x0602
	CmdPause           Command = 0x0603
	CmdResume          Command = 0x0604
	CmdPlayNext        Command = 0x0605
	CmdPlayFile        Command = 0x0606
	CmdCurrentFile     Command = 0x0607
	CmdPlayPrevious    Command = 0x0609
	CmdBuzzer          Command = 0x060F

	CmdFormat     Command = 0x0702
	CmdMkdir      Command = 0x0703
	CmdRename     Command = 0x0704
	CmdMove       Command = 0x0705
	CmdRemove     Command = 0x0706
	CmdRemoveAll  Command = 0x0707 // ..0x070A, offset by FileKind
	CmdReadDir    Command = 0x070B
	CmdDiskInfo   Command = 0x070D
	CmdFileExists Command = 0x070E

	CmdStartStream  Command = 0x0801
	CmdStopStream   Command = 0x0802
	CmdStreamStatus Command = 0x0803
	CmdStreamData   Command = 0x0804

	CmdStartNoWrap Command = 0x0901
	CmdStopNoWrap  Command = 0x0902

	CmdLogin          Command = 0x0A01
	CmdLogout         Command = 0x0A02
	CmdChangePassword Command = 0x0A03
)

// Address identifies a sign on a shared bus: group and unit address.
// The zero value {0, 0} is the broadcast address.
type Address struct {
	Group byte
	Unit  byte
}

// Packet is a single frame of the binary communication format:
//
//	offset 0  [2] sync   0x55 0xA7 (request) / 0x55 0xA8 (response)
//	offset 2  [2] checksum: 16-bit sum of all bytes from offset 4 to the end
//	offset 4  [2] length of Data
//	offset 6  [2] source address
//	offset 8  [2] destination address (group, unit)
//	offset 10 [2] packet serial
//	offset 12 [1] main command
//	offset 13 [1] sub command
//	offset 14 [1] Arg length in 4-byte units
//	offset 15 [1] flag: request 0 = reply wanted, 1 = no reply;
//	              response 1 = Arg holds a status code, 0 = payload
//	offset 16     Arg (padded to a multiple of 4), then Data
//
// All multi-byte integers are little-endian.
type Packet struct {
	Response bool // sync 0x55A8 (sign to host) instead of 0x55A7
	Serial   uint16
	Source   uint16
	Dest     Address
	Cmd      Command
	Flag     byte
	Arg      []byte
	Data     []byte
}

const headerSize = 16

// FlagNoReply, set on a request Packet's Flag, tells the sign not to answer;
// Client.Do then writes the packet and returns without waiting for a reply.
const FlagNoReply byte = 1

var (
	ErrBadSync     = errors.New("jetfile: bad sync bytes")
	ErrBadChecksum = errors.New("jetfile: checksum mismatch")
)

// checksum is the additive checksum over b, truncated to 16 bits
// (MsgCountCheckSumTwo in the spec's appendix).
func checksum(b []byte) uint16 {
	var sum uint16
	for _, v := range b {
		sum += uint16(v)
	}
	return sum
}

// Marshal encodes p into wire format, computing padding and checksum.
func (p *Packet) Marshal() ([]byte, error) {
	if len(p.Arg) > 255*4 {
		return nil, fmt.Errorf("jetfile: arg too long (%d bytes, max %d)", len(p.Arg), 255*4)
	}
	if len(p.Data) > 0xFFFF {
		return nil, fmt.Errorf("jetfile: data too long (%d bytes, max %d)", len(p.Data), 0xFFFF)
	}
	argLen := (len(p.Arg) + 3) / 4
	buf := make([]byte, headerSize+argLen*4+len(p.Data))
	buf[0], buf[1] = 0x55, 0xA7
	if p.Response {
		buf[1] = 0xA8
	}
	binary.LittleEndian.PutUint16(buf[4:], uint16(len(p.Data)))
	binary.LittleEndian.PutUint16(buf[6:], p.Source)
	buf[8], buf[9] = p.Dest.Group, p.Dest.Unit
	binary.LittleEndian.PutUint16(buf[10:], p.Serial)
	buf[12], buf[13] = p.Cmd.Main(), p.Cmd.Sub()
	buf[14] = byte(argLen)
	buf[15] = p.Flag
	copy(buf[headerSize:], p.Arg)
	copy(buf[headerSize+argLen*4:], p.Data)
	binary.LittleEndian.PutUint16(buf[2:], checksum(buf[4:]))
	return buf, nil
}

// ReadPacket reads exactly one frame from r and verifies sync and checksum.
func ReadPacket(r io.Reader) (*Packet, error) {
	var h [headerSize]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	if h[0] != 0x55 || (h[1] != 0xA7 && h[1] != 0xA8) {
		return nil, ErrBadSync
	}
	argLen := int(h[14]) * 4
	dataLen := int(binary.LittleEndian.Uint16(h[4:]))
	body := make([]byte, argLen+dataLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	if want := binary.LittleEndian.Uint16(h[2:]); checksum(h[4:])+checksum(body) != want {
		return nil, ErrBadChecksum
	}
	return &Packet{
		Response: h[1] == 0xA8,
		Serial:   binary.LittleEndian.Uint16(h[10:]),
		Source:   binary.LittleEndian.Uint16(h[6:]),
		Dest:     Address{Group: h[8], Unit: h[9]},
		Cmd:      Command(uint16(h[12])<<8 | uint16(h[13])),
		Flag:     h[15],
		Arg:      body[:argLen],
		Data:     body[argLen:],
	}, nil
}

// Status extracts the status code of a response whose Flag marks it as a
// status reply. ok is false if p carries payload data instead.
func (p *Packet) Status() (code StatusCode, ok bool) {
	if p.Flag != 1 {
		return 0, false
	}
	switch {
	case len(p.Arg) >= 2:
		return StatusCode(binary.LittleEndian.Uint16(p.Arg)), true
	case len(p.Data) >= 2:
		return StatusCode(binary.LittleEndian.Uint16(p.Data)), true
	}
	return 0, false
}
