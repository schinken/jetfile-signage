package jetfile

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// The sign encodes date/time fields as BCD: the year 2005 is sent as the
// 16-bit value 0x2005, the 22nd as 0x22, 15:00 as hour 0x15.
func bcd(v int) byte   { return byte(v/10<<4 | v%10) }
func unbcd(b byte) int { return int(b>>4)*10 + int(b&0x0F) }

// half-hour time zones that have their own code (spec Appendix I).
var oddZones = map[int]byte{
	-(3*3600 + 1800): 0x1A, // -3:30
	5*3600 + 1800:    0x1B, // +5:30
	5*3600 + 2700:    0x1C, // +5:45
	6*3600 + 1800:    0x1D, // +6:30
	9*3600 + 1800:    0x1E, // +9:30
}

// timezoneCode maps a UTC offset in seconds to the sign's zone code
// (0x00 = UTC-12 ... 0x19 = UTC+13). Offsets without an exact code are
// rounded to the nearest full hour.
func timezoneCode(offsetSec int) byte {
	if code, ok := oddZones[offsetSec]; ok {
		return code
	}
	// round to nearest hour (floor division, as Go's / truncates toward zero)
	n := offsetSec + 30*60
	h := n / 3600
	if n < 0 && n%3600 != 0 {
		h--
	}
	return byte(min(max(h, -12), 13) + 12)
}

// timezoneOffset is the inverse of timezoneCode.
func timezoneOffset(code byte) int {
	for off, c := range oddZones {
		if c == code {
			return off
		}
	}
	return (int(code) - 12) * 3600
}

// Clock reads the sign's current date and time. The location is a fixed
// zone built from the sign's time zone code.
func (c *Client) Clock(ctx context.Context) (time.Time, error) {
	resp, err := c.query(ctx, CmdReadClock, nil)
	if err != nil {
		return time.Time{}, err
	}
	a := resp.Arg
	if len(a) < 8 {
		return time.Time{}, fmt.Errorf("jetfile: short clock response (%d bytes)", len(a))
	}
	offset := timezoneOffset(a[7])
	zone := time.FixedZone(fmt.Sprintf("UTC%+d", offset/3600), offset)
	// arg: [2]year [1]month [1]day [1]hour [1]minute [1]weekday [1]zone,
	// all BCD, year little-endian
	return time.Date(
		unbcd(a[1])*100+unbcd(a[0]), time.Month(unbcd(a[2])), unbcd(a[3]),
		unbcd(a[4]), unbcd(a[5]), 0, 0, zone,
	), nil
}

// SetClock sets the sign's clock to t. It first tries the extended command
// with second precision and falls back to the legacy minute-precision
// command on signs that don't support it.
func (c *Client) SetClock(ctx context.Context, t time.Time) error {
	err := c.exec(ctx, CmdSetClockExt, clockArg(t, true), nil)
	var derr *DeviceError
	if errors.As(err, &derr) &&
		(derr.Code == StatusBadSubCommand || derr.Code == StatusUnsupported) {
		err = c.exec(ctx, CmdSetClock, clockArg(t, false), nil)
	}
	return err
}

// clockArg encodes t for 0x0504 (withSeconds, 12 bytes) or 0x0502 (8 bytes).
// Weekday is 1-7 with Sunday = 1.
func clockArg(t time.Time, withSeconds bool) []byte {
	_, offset := t.Zone()
	a := []byte{
		bcd(t.Year() % 100), bcd(t.Year() / 100),
		bcd(int(t.Month())), bcd(t.Day()),
		bcd(t.Hour()), bcd(t.Minute()),
	}
	if withSeconds {
		a = append(a, bcd(t.Second()))
	}
	a = append(a, byte(t.Weekday())+1, timezoneCode(offset))
	if withSeconds {
		a = append(a, 0, 0, 0) // reserved
	}
	return a
}
