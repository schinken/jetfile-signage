package jetfile

import "fmt"

// StatusCode is a status returned by the sign (spec Appendix V).
type StatusCode uint16

const (
	StatusOK     StatusCode = 0x9000
	StatusOKText StatusCode = 0x4B4F // ASCII "OK", little-endian

	StatusChecksumError     StatusCode = 0x9002
	StatusBadMainCommand    StatusCode = 0x9004
	StatusBadSubCommand     StatusCode = 0x9005
	StatusBadPacketLength   StatusCode = 0x9006
	StatusFileNotFound      StatusCode = 0x9008
	StatusEndOfFile         StatusCode = 0x9009
	StatusFileOpenFailed    StatusCode = 0x9010
	StatusUnsupported       StatusCode = 0x9011
	StatusWriteFailed       StatusCode = 0x9012
	StatusPacketTooBig      StatusCode = 0x9013
	StatusPacketOutOfOrder  StatusCode = 0x9014
	StatusFileInUse         StatusCode = 0x9015
	StatusLoginRequired     StatusCode = 0x9030
	StatusWrongPassword     StatusCode = 0x9031
	StatusWrongUser         StatusCode = 0x9032
	StatusWrongOldPassword  StatusCode = 0x9033
	StatusLoggedInElsewhere StatusCode = 0x9035

	StatusFileTooBig      StatusCode = 0x2101
	StatusDiskFull        StatusCode = 0x2102
	StatusMessageTooBig   StatusCode = 0x2901
	StatusNoGrayscale     StatusCode = 0x3A01
	StatusClockSetFailed  StatusCode = 0x5201
	StatusNoCurrentFile   StatusCode = 0x6701
	StatusCurrentFileOpen StatusCode = 0x6702
	StatusUseExtendedRead StatusCode = 0x6703
	StatusFormatFailed    StatusCode = 0x7201
	StatusMkdirFailed     StatusCode = 0x7301
	StatusRenameFailed    StatusCode = 0x7401
	StatusRenameBadPath   StatusCode = 0x7402
	StatusMoveFailed      StatusCode = 0x7501
	StatusRemoveFailed    StatusCode = 0x7601
	StatusOpenFailed      StatusCode = 0x7B01
	StatusDiskReadFailed  StatusCode = 0x7D01
	StatusFileNotExists   StatusCode = 0x7E01

	StatusStreamBuf1   StatusCode = 0x8301
	StatusStreamBuf2   StatusCode = 0x8302
	StatusStreamBuf3   StatusCode = 0x8303
	StatusNotStreaming StatusCode = 0x8305
	StatusStreamTooBig StatusCode = 0x8306
	StatusStreamBadFmt StatusCode = 0x8307
)

// OK reports whether s means success.
func (s StatusCode) OK() bool { return s == StatusOK || s == StatusOKText }

var statusText = map[StatusCode]string{
	StatusOK:                "ok",
	StatusOKText:            "ok",
	StatusChecksumError:     "checksum incorrect",
	StatusBadMainCommand:    "invalid main command",
	StatusBadSubCommand:     "invalid sub command",
	StatusBadPacketLength:   "packet length incorrect",
	StatusFileNotFound:      "file not found",
	StatusEndOfFile:         "read past end of file",
	StatusFileOpenFailed:    "file open failed",
	StatusUnsupported:       "command not supported by this sign",
	StatusWriteFailed:       "file write failed (disk full?)",
	StatusPacketTooBig:      "packet bigger than 1500 bytes or declared size",
	StatusPacketOutOfOrder:  "file packets not sent in sequence",
	StatusFileInUse:         "file is open",
	StatusLoginRequired:     "login required",
	StatusWrongPassword:     "wrong password",
	StatusWrongUser:         "wrong user name",
	StatusWrongOldPassword:  "wrong old password",
	StatusLoggedInElsewhere: "already logged in elsewhere",
	StatusFileTooBig:        "file exceeds 320K write limit",
	StatusDiskFull:          "not enough space on disk",
	StatusMessageTooBig:     "emergency message exceeds 1024 bytes",
	StatusNoGrayscale:       "grayscale test not supported",
	StatusClockSetFailed:    "time setting failed",
	StatusNoCurrentFile:     "no file currently playing",
	StatusCurrentFileOpen:   "failed to open current play file",
	StatusUseExtendedRead:   "file too big, use extended read",
	StatusFormatFailed:      "formatting failed",
	StatusMkdirFailed:       "creating folder failed",
	StatusRenameFailed:      "renaming failed",
	StatusRenameBadPath:     "bad path in rename",
	StatusMoveFailed:        "moving file failed",
	StatusRemoveFailed:      "deleting file failed",
	StatusOpenFailed:        "opening file failed",
	StatusDiskReadFailed:    "reading disk info failed (bad disk name?)",
	StatusFileNotExists:     "file does not exist",
	StatusNotStreaming:      "sign is not in stream display mode",
	StatusStreamTooBig:      "stream data exceeds receive buffer",
	StatusStreamBadFmt:      "stream data format not supported by panel",
}

func (s StatusCode) String() string {
	if t, ok := statusText[s]; ok {
		return fmt.Sprintf("%s (0x%04X)", t, uint16(s))
	}
	return fmt.Sprintf("status 0x%04X", uint16(s))
}

// DeviceError is a non-success status code returned by the sign.
type DeviceError struct {
	Cmd  Command
	Code StatusCode
}

func (e *DeviceError) Error() string {
	return fmt.Sprintf("jetfile: command %s failed: %s", e.Cmd, e.Code)
}

// Is allows errors.Is(err, &DeviceError{Code: ...}) matching on the code.
func (e *DeviceError) Is(target error) bool {
	t, ok := target.(*DeviceError)
	return ok && t.Code == e.Code
}
