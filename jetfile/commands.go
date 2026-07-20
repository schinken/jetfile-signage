package jetfile

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"
)

// ---- system information (0x01, 0x03) ----

// ConnectionInfo is the reply to a connection test.
type ConnectionInfo struct {
	ProgramVersion uint16
	FPGAVersion    uint16
	IP             net.IP
	Addr           Address
}

// ConnectionTest pings the sign and returns its basic parameters.
func (c *Client) ConnectionTest(ctx context.Context) (*ConnectionInfo, error) {
	resp, err := c.query(ctx, CmdConnectionTest, nil)
	if err != nil {
		return nil, err
	}
	a := resp.Arg
	if len(a) < 10 {
		return nil, fmt.Errorf("jetfile: short connection test response (%d bytes)", len(a))
	}
	return &ConnectionInfo{
		ProgramVersion: binary.LittleEndian.Uint16(a[0:]),
		FPGAVersion:    binary.LittleEndian.Uint16(a[2:]),
		IP:             net.IPv4(a[7], a[6], a[5], a[4]), // wire is low byte first
		Addr:           Address{Group: a[8], Unit: a[9]},
	}, nil
}

// SystemParams describe the sign's hardware and firmware.
type SystemParams struct {
	CPUVersion        uint16
	TCPIPVersion      uint16
	FileSystemVersion uint16
	FPGAVersion       uint16
	Width, Height     uint16 // pixels
	ProtocolVersion   uint16
	Addr              Address
}

// SystemParams reads the sign's hardware parameters (0x010A).
func (c *Client) SystemParams(ctx context.Context) (*SystemParams, error) {
	resp, err := c.query(ctx, CmdReadSystemParams, nil)
	if err != nil {
		return nil, err
	}
	a := resp.Arg
	if len(a) < 16 {
		return nil, fmt.Errorf("jetfile: short system params response (%d bytes)", len(a))
	}
	return &SystemParams{
		CPUVersion:        binary.LittleEndian.Uint16(a[0:]),
		TCPIPVersion:      binary.LittleEndian.Uint16(a[2:]),
		FileSystemVersion: binary.LittleEndian.Uint16(a[4:]),
		FPGAVersion:       binary.LittleEndian.Uint16(a[6:]),
		Width:             binary.LittleEndian.Uint16(a[8:]),
		Height:            binary.LittleEndian.Uint16(a[10:]),
		ProtocolVersion:   binary.LittleEndian.Uint16(a[12:]),
		Addr:              Address{Group: a[14], Unit: a[15]},
	}, nil
}

// DisplayMode is what the sign is currently showing.
type DisplayMode byte

const (
	ModeSchedule   DisplayMode = 0 // normal playlist
	ModeEmergency  DisplayMode = 1
	ModeBlack      DisplayMode = 2
	ModeRemote     DisplayMode = 3
	ModeTest       DisplayMode = 4
	ModeStream     DisplayMode = 5 // unlimited connection display
	ModeNoWordWrap DisplayMode = 6
)

// SystemStatus is the sign's live state.
type SystemStatus struct {
	Mode        DisplayMode
	CabinetTemp int8 // °C; -1 (0xFF) means no sensor
	OutdoorTemp int8 // °C; -1 (0xFF) means no sensor
	AutoPower   bool // scheduled on/off enabled
	Humidity    int8 // percent
}

// SystemStatus reads temperature, humidity and display state (0x010B).
func (c *Client) SystemStatus(ctx context.Context) (*SystemStatus, error) {
	resp, err := c.query(ctx, CmdReadSystemStatus, nil)
	if err != nil {
		return nil, err
	}
	a := resp.Arg
	if len(a) < 5 {
		return nil, fmt.Errorf("jetfile: short system status response (%d bytes)", len(a))
	}
	return &SystemStatus{
		Mode:        DisplayMode(a[0]),
		CabinetTemp: int8(a[1]),
		OutdoorTemp: int8(a[2]),
		AutoPower:   a[3] == 1,
		Humidity:    int8(a[4]),
	}, nil
}

// SNMAC reads the sign's serial number and MAC address (0x010C).
func (c *Client) SNMAC(ctx context.Context) (sn string, mac net.HardwareAddr, err error) {
	resp, err := c.query(ctx, CmdReadSNMAC, nil)
	if err != nil {
		return "", nil, err
	}
	if len(resp.Data) < 22 {
		return "", nil, fmt.Errorf("jetfile: short SN/MAC response (%d bytes)", len(resp.Data))
	}
	return nulString(resp.Data[:12]), net.HardwareAddr(resp.Data[16:22]), nil
}

// SystemInfo reads the firmware's free-form system information string.
func (c *Client) SystemInfo(ctx context.Context) (string, error) {
	resp, err := c.query(ctx, CmdReadSystemInfo, nil)
	if err != nil {
		return "", err
	}
	return nulString(resp.Data), nil
}

// ---- display tests (0x03) ----

// TestPattern is a built-in display test.
type TestPattern byte

const (
	TestAuto           TestPattern = 0x02 // runs all tests in sequence
	TestAllOn          TestPattern = 0x03
	TestAllRed         TestPattern = 0x04
	TestAllGreen       TestPattern = 0x05
	TestAllBlue        TestPattern = 0x06
	TestHorizontalScan TestPattern = 0x07
	TestVerticalScan   TestPattern = 0x08
)

// StartTest puts the sign into the given test pattern until EndTest.
func (c *Client) StartTest(ctx context.Context, p TestPattern) error {
	return c.exec(ctx, Command(0x0300)|Command(p), nil, nil)
}

// EndTest leaves test mode.
func (c *Client) EndTest(ctx context.Context) error {
	return c.exec(ctx, CmdEndTest, nil, nil)
}

// GrayscaleTest runs a grayscale test with the given color channels and
// number of levels. Not supported by all boards.
func (c *Client) GrayscaleTest(ctx context.Context, gradual, r, g, b bool, levels uint16) error {
	arg := make([]byte, 8)
	for i, on := range []bool{gradual, r, g, b} {
		if on {
			arg[i] = 1
		}
	}
	binary.LittleEndian.PutUint16(arg[4:], levels)
	return c.exec(ctx, CmdGrayscaleTest, arg, nil)
}

// ---- black screen and power (0x04) ----

// BlackScreen blanks (true) or unblanks (false) the display.
func (c *Client) BlackScreen(ctx context.Context, on bool) error {
	cmd := CmdBlackScreen
	if !on {
		cmd = CmdEndBlack
	}
	return c.exec(ctx, cmd, nil, nil)
}

// Reset reboots the sign.
func (c *Client) Reset(ctx context.Context) error {
	return c.exec(ctx, CmdReset, nil, nil)
}

// PowerOff puts the sign into semi-off state. showNotice displays
// "USERSHUTDOWNI" instead of a dark panel.
func (c *Client) PowerOff(ctx context.Context, showNotice bool) error {
	arg := []byte{1, 0, 0, 0} // 0 = show, 1 = don't show
	if showNotice {
		arg[0] = 0
	}
	return c.exec(ctx, CmdPowerOff, arg, nil)
}

// PowerOn returns the sign from semi-off state to normal display.
func (c *Client) PowerOn(ctx context.Context) error {
	return c.exec(ctx, CmdPowerOn, nil, nil)
}

// PowerState is the on/off state of the control board.
type PowerState byte

const (
	PowerStateOn        PowerState = 0
	PowerStateOffNotice PowerState = 1 // off, showing "USERSHUTDOWNI"
	PowerStateOffBlack  PowerState = 2
)

// PowerStatus reports whether control and driver boards are switched on.
type PowerStatus struct {
	State    PowerState
	DriverOn bool
}

// PowerStatus reads the sign's on/off state (0x0405).
func (c *Client) PowerStatus(ctx context.Context) (*PowerStatus, error) {
	resp, err := c.query(ctx, CmdPowerStatus, nil)
	if err != nil {
		return nil, err
	}
	if len(resp.Data) < 2 {
		return nil, fmt.Errorf("jetfile: short power status response (%d bytes)", len(resp.Data))
	}
	return &PowerStatus{
		State:    PowerState(resp.Data[0]),
		DriverOn: resp.Data[1] == 0,
	}, nil
}

// ---- play control (0x06) ----

// RestartPlaylist replays the file list from the beginning.
func (c *Client) RestartPlaylist(ctx context.Context) error {
	return c.exec(ctx, CmdRestartPlaylist, nil, nil)
}

// ReplayCurrent restarts the currently playing file.
func (c *Client) ReplayCurrent(ctx context.Context) error {
	return c.exec(ctx, CmdReplayCurrent, nil, nil)
}

// Pause freezes playback; Resume continues it.
func (c *Client) Pause(ctx context.Context) error  { return c.exec(ctx, CmdPause, nil, nil) }
func (c *Client) Resume(ctx context.Context) error { return c.exec(ctx, CmdResume, nil, nil) }

// PlayNext skips to the next file, PlayPrevious to the previous one.
func (c *Client) PlayNext(ctx context.Context) error { return c.exec(ctx, CmdPlayNext, nil, nil) }
func (c *Client) PlayPrevious(ctx context.Context) error {
	return c.exec(ctx, CmdPlayPrevious, nil, nil)
}

// FileType selects the folder a file lives in. The spec leaves the values
// of the play command's file type field undocumented; the folder letter is
// the common convention.
type FileType byte

const (
	FileTypeText         FileType = 'T'
	FileTypeString       FileType = 'S'
	FileTypePicture      FileType = 'P'
	FileTypeArrayPicture FileType = 'A'
)

// PlayFile interrupts the playlist to play the given file once, then
// resumes normal scheduling (0x0606).
func (c *Client) PlayFile(ctx context.Context, ft FileType, label string) error {
	lab, err := fileLabel(label)
	if err != nil {
		return err
	}
	arg := make([]byte, 16)
	arg[0] = c.partition
	arg[1] = byte(ft)
	copy(arg[2:14], lab[:])
	return c.exec(ctx, CmdPlayFile, arg, nil)
}

// CurrentFile returns the path of the file currently playing.
func (c *Client) CurrentFile(ctx context.Context) (string, error) {
	resp, err := c.query(ctx, CmdCurrentFile, []byte{1, 0, 0, 0}) // 1 = name only
	if err != nil {
		return "", err
	}
	return nulString(resp.Arg), nil
}

// BuzzerMode selects when the buzzer sounds.
type BuzzerMode byte

const (
	BuzzOnNewFile    BuzzerMode = 0
	BuzzOnFileChange BuzzerMode = 1
)

// SetBuzzer configures the buzzer (0x060F, board dependent). seconds is
// clamped to 0-9.
func (c *Client) SetBuzzer(ctx context.Context, on bool, mode BuzzerMode, seconds int) error {
	arg := make([]byte, 8)
	if on {
		arg[0] = 1
	}
	arg[1] = byte(mode)
	arg[2] = '0' + byte(min(max(seconds, 0), 9))
	return c.exec(ctx, CmdBuzzer, arg, nil)
}

// ---- file control (0x07) ----

// FormatPartition formats a disk partition ('D', 'E', ...), destroying its
// contents.
func (c *Client) FormatPartition(ctx context.Context, partition byte) error {
	return c.exec(ctx, CmdFormat, []byte{partition, 0, 0, 0}, nil)
}

// Mkdir creates a single folder, e.g. `C:\TEST\`. Multi-level creation is
// not supported by the sign.
func (c *Client) Mkdir(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("jetfile: empty path")
	}
	if path[len(path)-1] != '\\' {
		path += `\`
	}
	return c.exec(ctx, CmdMkdir, append([]byte(path), 0), nil)
}

// Rename renames a file or folder.
func (c *Client) Rename(ctx context.Context, from, to string) error {
	return c.twoPaths(ctx, CmdRename, from, to)
}

// Move moves a file or folder.
func (c *Client) Move(ctx context.Context, from, to string) error {
	return c.twoPaths(ctx, CmdMove, from, to)
}

func (c *Client) twoPaths(ctx context.Context, cmd Command, from, to string) error {
	if from == "" || to == "" {
		return fmt.Errorf("jetfile: empty path")
	}
	// Spec tables 3.7.4/3.7.5: "source + Space + target + NULL".
	arg := append([]byte(from), ' ')
	arg = append(arg, to...)
	return c.exec(ctx, cmd, append(arg, 0), nil)
}

// Remove deletes a single file by path, e.g. `D:\T\AB`.
func (c *Client) Remove(ctx context.Context, path string) error {
	if path == "" {
		return fmt.Errorf("jetfile: empty path")
	}
	return c.exec(ctx, CmdRemove, append([]byte(path), 0), nil)
}

// RemoveAll deletes all files of the given type from the client's
// partition (0x0707..0x070A).
func (c *Client) RemoveAll(ctx context.Context, ft FileType) error {
	var cmd Command
	switch ft {
	case FileTypeText:
		cmd = 0x0707
	case FileTypeString:
		cmd = 0x0708
	case FileTypePicture:
		cmd = 0x0709
	case FileTypeArrayPicture:
		cmd = 0x070A
	default:
		return fmt.Errorf("jetfile: unknown file type %q", byte(ft))
	}
	return c.exec(ctx, cmd, []byte{c.partition, 0, 0, 0}, nil)
}

// DirEntry is one file or folder in a directory listing.
type DirEntry struct {
	Name     string
	Dir      bool
	Size     uint32
	Modified time.Time // sign-local time, expressed in UTC
}

// ReadDir lists a folder, e.g. `D:\T\` (0x070B).
func (c *Client) ReadDir(ctx context.Context, path string) ([]DirEntry, error) {
	if path == "" {
		return nil, fmt.Errorf("jetfile: empty path")
	}
	if path[len(path)-1] != '\\' {
		path += `\`
	}
	resp, err := c.query(ctx, CmdReadDir, append([]byte(path), 0))
	if err != nil {
		return nil, err
	}
	n := len(resp.Data) / 32
	if len(resp.Arg) >= 2 {
		if c := int(binary.LittleEndian.Uint16(resp.Arg)); c < n {
			n = c
		}
	}
	entries := make([]DirEntry, 0, n)
	for i := 0; i < n; i++ {
		entries = append(entries, parseDirEntry(resp.Data[i*32:]))
	}
	return entries, nil
}

// parseDirEntry decodes a 32-byte FAT directory entry.
func parseDirEntry(b []byte) DirEntry {
	name := nulString(b[0:8])
	if ext := nulString(b[8:11]); ext != "" {
		name += "." + ext
	}
	return DirEntry{
		Name: name,
		Dir:  b[11]&0x10 != 0,
		Size: binary.LittleEndian.Uint32(b[28:]),
		Modified: fatTime(
			binary.LittleEndian.Uint16(b[24:]), // write date
			binary.LittleEndian.Uint16(b[22:]), // write time
		),
	}
}

// fatTime decodes FAT date (day 0-4, month 5-8, year-1980 9-15) and time
// (seconds/2 0-4, minutes 5-10, hours 11-15) fields.
func fatTime(date, tim uint16) time.Time {
	return time.Date(
		1980+int(date>>9), time.Month(date>>5&0x0F), int(date&0x1F),
		int(tim>>11), int(tim>>5&0x3F), int(tim&0x1F)*2, 0, time.UTC,
	)
}

// DiskInfo describes a partition.
type DiskInfo struct {
	Total uint32 // bytes
	Free  uint32 // bytes
	Name  string
}

// DiskInfo reads size and free space of a partition ('D', 'E', ...).
func (c *Client) DiskInfo(ctx context.Context, partition byte) (*DiskInfo, error) {
	resp, err := c.query(ctx, CmdDiskInfo, []byte{partition, ':', 0, 0})
	if err != nil {
		return nil, err
	}
	a := resp.Arg
	if len(a) < 20 {
		return nil, fmt.Errorf("jetfile: short disk info response (%d bytes)", len(a))
	}
	return &DiskInfo{
		Total: binary.LittleEndian.Uint32(a[0:]),
		Free:  binary.LittleEndian.Uint32(a[4:]),
		Name:  nulString(a[8:20]),
	}, nil
}

// FileExists checks whether a path exists on the sign.
func (c *Client) FileExists(ctx context.Context, path string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("jetfile: empty path")
	}
	err := c.exec(ctx, CmdFileExists, append([]byte(path), 0), nil)
	if errors.Is(err, &DeviceError{Code: StatusFileNotExists}) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ---- pixel streaming (0x08) ----

// Direction of stream / no-wrap movement.
type Direction byte

const (
	MoveLeft  Direction = 0
	MoveRight Direction = 1
)

// StartStream puts the sign into unlimited connection display mode, where
// raw pixel data is pushed with StreamData. speed is 0 (fastest) to 6
// (slowest). oneBitRG selects the packed 1:1 RGRGRGRG format; otherwise
// the panel's native format is used (see spec 0x0801 for the layouts).
func (c *Client) StartStream(ctx context.Context, dir Direction, speed byte, oneBitRG bool) error {
	format := byte(1)
	if oneBitRG {
		format = 0
	}
	return c.exec(ctx, CmdStartStream, []byte{byte(dir), speed, format, 0}, nil)
}

// StopStream leaves stream mode and resumes the schedule.
func (c *Client) StopStream(ctx context.Context) error {
	return c.exec(ctx, CmdStopStream, nil, nil)
}

// StreamStatus returns the sign's stream buffer status code
// (StatusStreamBuf1..3, or StatusNotStreaming).
func (c *Client) StreamStatus(ctx context.Context) (StatusCode, error) {
	resp, err := c.Do(ctx, &Packet{Cmd: CmdStreamStatus})
	var derr *DeviceError
	if errors.As(err, &derr) && derr.Code>>8 == 0x83 {
		return derr.Code, nil
	}
	if err != nil {
		return 0, err
	}
	code, _ := resp.Status()
	return code, nil
}

// StreamData downloads pixel data into the sign's stream buffer, split
// into ordered packets. The pixel layout must match the format chosen in
// StartStream.
func (c *Client) StreamData(ctx context.Context, pixels []byte) error {
	qty := (len(pixels) + chunkSize - 1) / chunkSize
	if qty == 0 {
		qty = 1
	}
	for i := 0; i < qty; i++ {
		part := pixels[i*chunkSize : min((i+1)*chunkSize, len(pixels))]
		arg := make([]byte, 8)
		binary.LittleEndian.PutUint16(arg[0:], chunkSize)
		binary.LittleEndian.PutUint16(arg[2:], uint16(qty))
		binary.LittleEndian.PutUint16(arg[4:], uint16(i+1))
		if err := c.exec(ctx, CmdStreamData, arg, part); err != nil {
			return fmt.Errorf("packet %d/%d: %w", i+1, qty, err)
		}
	}
	return nil
}

// ---- brightness (0x020A) ----

// SetBrightness writes a brightness control block for the given region
// (0x020A, not supported by all boards).
func (c *Client) SetBrightness(ctx context.Context, x, y, width, height int, r, g, b byte) error {
	arg := make([]byte, 12)
	binary.LittleEndian.PutUint16(arg[0:], uint16(x))
	binary.LittleEndian.PutUint16(arg[2:], uint16(y))
	arg[4], arg[5], arg[6] = r, g, b
	binary.LittleEndian.PutUint16(arg[7:], uint16(width))
	binary.LittleEndian.PutUint16(arg[9:], uint16(height))
	return c.exec(ctx, CmdWriteBrightness, arg, nil)
}

// ---- login (0x0A) ----

// Login authenticates against signs with password management enabled.
// The user name is at most 13 characters, the password at most 6.
func (c *Client) Login(ctx context.Context, user, password string) error {
	arg, err := loginArg(user, password, 20)
	if err != nil {
		return err
	}
	return c.exec(ctx, CmdLogin, arg, nil)
}

// Logout ends the session; further operations require a new Login.
func (c *Client) Logout(ctx context.Context) error {
	return c.exec(ctx, CmdLogout, nil, nil)
}

// ChangePassword changes the password of user. Requires a prior Login.
func (c *Client) ChangePassword(ctx context.Context, user, old, updated string) error {
	arg, err := loginArg(user, old, 28)
	if err != nil {
		return err
	}
	if len(updated) > 6 {
		return fmt.Errorf("jetfile: password longer than 6 characters")
	}
	copy(arg[20:26], updated)
	return c.exec(ctx, CmdChangePassword, arg, nil)
}

// loginArg builds [14]user\0 [6]password (+ trailing space up to size).
func loginArg(user, password string, size int) ([]byte, error) {
	if len(user) > 13 {
		return nil, fmt.Errorf("jetfile: user name longer than 13 characters")
	}
	if len(password) > 6 {
		return nil, fmt.Errorf("jetfile: password longer than 6 characters")
	}
	arg := make([]byte, size)
	copy(arg[0:14], user)
	copy(arg[14:20], password)
	return arg, nil
}
