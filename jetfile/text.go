package jetfile

import (
	"fmt"
	"time"
)

// Text builds the content of a text or string file using the display
// control characters from Part II of the spec. Methods chain:
//
//	t := jetfile.NewText().
//		Font(jetfile.Font7x6).
//		Color(jetfile.ColorRed).
//		In(jetfile.EffectMoveLeft).
//		Str("Hello")
type Text struct{ b []byte }

// NewText returns an empty builder.
func NewText() *Text { return &Text{} }

// Str appends display text. '\n' becomes a protocol line feed.
func (t *Text) Str(s string) *Text {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			t.b = append(t.b, ctlLineFeed)
		} else {
			t.b = append(t.b, s[i])
		}
	}
	return t
}

// Strf appends fmt.Sprintf-formatted display text.
func (t *Text) Strf(format string, a ...any) *Text {
	return t.Str(fmt.Sprintf(format, a...))
}

// Raw appends bytes verbatim, for control sequences without a helper.
func (t *Text) Raw(b ...byte) *Text {
	t.b = append(t.b, b...)
	return t
}

// Font selects the font for the following text.
func (t *Text) Font(f Font) *Text { return t.Raw(ctlFont, byte(f)) }

// Color sets the font color.
func (t *Text) Color(c Color) *Text { return t.Raw(ctlFontColor, byte(c)) }

// ColorRGB sets a self-defined 24-bit font color.
func (t *Text) ColorRGB(r, g, b byte) *Text { return t.Raw(ctlFontColor, '/', b, g, r) }

// Background sets the font background color (ColorBlack..ColorYellow).
func (t *Text) Background(c Color) *Text { return t.Raw(ctlBackground, byte(c)) }

// BackgroundRGB sets a self-defined 24-bit font background color.
func (t *Text) BackgroundRGB(r, g, b byte) *Text { return t.Raw(ctlBackground, '/', b, g, r) }

// In sets the entry animation of the current frame.
func (t *Text) In(e Effect) *Text { return t.Raw(ctlPattern, 'I', byte(e)) }

// Out sets the exit animation of the current frame.
func (t *Text) Out(e Effect) *Text { return t.Raw(ctlPattern, 'O', byte(e)) }

// Speed sets the animation speed.
func (t *Text) Speed(s Speed) *Text { return t.Raw(ctlSpeed, byte(s)) }

// Flash turns character flashing on or off.
func (t *Text) Flash(on bool) *Text {
	v := byte('0')
	if on {
		v = '1'
	}
	return t.Raw(ctlFlash, v)
}

// LineSpacing sets the spacing between lines (0-9 pixels).
func (t *Text) LineSpacing(px int) *Text {
	return t.Raw(ctlLineSpace, '0'+byte(min(max(px, 0), 9)))
}

// AlignH sets horizontal alignment.
func (t *Text) AlignH(a HAlign) *Text { return t.Raw(ctlAlignH, byte(a)) }

// AlignV sets vertical alignment.
func (t *Text) AlignV(a VAlign) *Text { return t.Raw(ctlAlignV, byte(a)) }

// Frame starts a new page.
func (t *Text) Frame() *Text { return t.Raw(ctlFrame) }

// Line starts a new line.
func (t *Text) Line() *Text { return t.Raw(ctlLineFeed) }

// HalfSpace inserts a half-width space.
func (t *Text) HalfSpace() *Text { return t.Raw(0x82) }

// Special inserts a live element (clock, date, temperature, ...).
func (t *Text) Special(s Special) *Text { return t.Raw(ctlSpecial, byte(s)) }

// Pause sets how long the current frame stays on screen. Durations under a
// second are encoded in milliseconds, everything else in whole seconds
// (max 9999s).
func (t *Text) Pause(d time.Duration) *Text {
	unit, v := byte('0'), int(d.Round(time.Second)/time.Second)
	if d < time.Second {
		unit, v = '1', int(d.Milliseconds())
	}
	if v > 9999 {
		v = 9999
	}
	if v > 99 {
		return t.Raw(ctlPause, unit+2).Strf("%04d", v)
	}
	return t.Raw(ctlPause, unit).Strf("%02d", v)
}

// InsertString embeds string file label from the given drive ('_' for the
// default partition). One-character labels use the short form, two-character
// labels the extended form.
func (t *Text) InsertString(drive byte, label string) *Text {
	if len(label) == 1 {
		return t.Raw(ctlNestString, drive, label[0])
	}
	return t.Raw(ctlNestString, 0x0F, drive, 'S').Str(label)
}

// InsertPicture embeds picture file label from the given drive ('_' for the
// default partition).
func (t *Text) InsertPicture(drive, label byte) *Text {
	return t.Raw(ctlNestPicture, drive, label)
}

// Bytes returns the raw content (without file header and EOF marker).
func (t *Text) Bytes() []byte { return t.b }

// file wraps the content in the stored file format:
// <0x01>Z00<0x02> + command code + label char ... content ... <0x04>.
func (t *Text) file(code, labelChar byte) []byte {
	f := make([]byte, 0, len(t.b)+8)
	f = append(f, 0x01, 'Z', '0', '0', 0x02, code, labelChar)
	f = append(f, t.b...)
	return append(f, 0x04)
}

// Control characters (spec Table 4.1.2).
const (
	ctlFlash       = 0x07
	ctlLineSpace   = 0x08
	ctlPattern     = 0x0A
	ctlSpecial     = 0x0B
	ctlFrame       = 0x0C
	ctlLineFeed    = 0x0D
	ctlPause       = 0x0E
	ctlSpeed       = 0x0F
	ctlNestString  = 0x13
	ctlNestPicture = 0x14
	ctlFont        = 0x1A
	ctlFontColor   = 0x1C
	ctlBackground  = 0x1D
	ctlAlignH      = 0x1E
	ctlAlignV      = 0x1F
)

// Font selects a character set ([Font & size] control character).
type Font byte

const (
	Font5x5        Font = '0'
	Font7x6        Font = '1' // factory default
	Font14x8       Font = '2'
	Font15x9       Font = '3'
	Font16x9       Font = '4'
	Font16x16Hanzi Font = '5'
	Font24x16      Font = '6'
	Font24x24Hanzi Font = '7'
	Font32x18      Font = '8'
	Font32x32Hanzi Font = '9'
	Font11x9       Font = ':'
	Font12x7       Font = ';'
	Font22x18      Font = '<'
	Font30x18      Font = '='
	Font40x21      Font = '>'
	FontBold14x10  Font = 'N'
	FontBold15x10  Font = 'O'
	FontBold16x12  Font = 'P'
	FontBold24x8   Font = 'Q'
	FontBold32x8   Font = 'R'
	FontBold11x7   Font = 'S'
	FontBold12x7   Font = 'T'
	FontBold22x12  Font = 'U'
	FontBold40x21  Font = 'V'
	FontCustom1    Font = 'g' // ..FontCustom9 = 'p'
)

// Color indexes the sign's palettes ([Font color] control character).
type Color byte

const (
	ColorBlack           Color = '0'
	ColorRed             Color = '1'
	ColorGreen           Color = '2'
	ColorYellow          Color = '3'
	ColorRainbowChar     Color = '4' // yellow/green/red per character
	ColorRainbowRows     Color = '5'
	ColorRainbowWave     Color = '6'
	ColorRainbowDiagonal Color = '7'
)

// Effect is a frame entry/exit animation ([Pattern] control character).
type Effect byte

const (
	EffectRandom          Effect = 0x2F
	EffectJumpOut         Effect = 0x30
	EffectMoveLeft        Effect = 0x31
	EffectMoveRight       Effect = 0x32
	EffectScrollLeft      Effect = 0x33
	EffectScrollRight     Effect = 0x34
	EffectMoveUp          Effect = 0x35
	EffectMoveDown        Effect = 0x36
	EffectScrollLR        Effect = 0x37
	EffectScrollUp        Effect = 0x38
	EffectScrollDown      Effect = 0x39
	EffectFoldLR          Effect = 0x3A
	EffectFoldUD          Effect = 0x3B
	EffectScrollUD        Effect = 0x3C
	EffectShuttleLR       Effect = 0x3D
	EffectShuttleUD       Effect = 0x3E
	EffectPeelOffLeft     Effect = 0x3F
	EffectPeelOffRight    Effect = 0x40
	EffectShutterUD       Effect = 0x41
	EffectShutterLR       Effect = 0x42
	EffectRaindrops       Effect = 0x43
	EffectRandomMosaic    Effect = 0x44
	EffectTwinkleStars    Effect = 0x45
	EffectHipHop          Effect = 0x46
	EffectRadarScan       Effect = 0x47
	EffectFanOut          Effect = 0x48
	EffectFanIn           Effect = 0x49
	EffectSpiralRight     Effect = 0x4A
	EffectSpiralLeft      Effect = 0x4B
	EffectToFourCorners   Effect = 0x4C
	EffectFromFourCorners Effect = 0x4D
	EffectToFourSides     Effect = 0x4E
	EffectFromFourSides   Effect = 0x4F
	EffectGrowUp          Effect = 0x60
)

// Speed is the animation speed ([Speed] control character).
type Speed byte

const (
	SpeedVeryFast   Speed = '0'
	SpeedFast       Speed = '1'
	SpeedMediumFast Speed = '2'
	SpeedMedium     Speed = '3'
	SpeedMediumSlow Speed = '4'
	SpeedSlow       Speed = '5'
	SpeedVerySlow   Speed = '6'
)

// HAlign / VAlign are text alignment values.
type HAlign byte

const (
	AlignCenter HAlign = '0'
	AlignLeft   HAlign = '1'
	AlignRight  HAlign = '2'
)

type VAlign byte

const (
	AlignMiddle VAlign = '0'
	AlignTop    VAlign = '1'
	AlignBottom VAlign = '2'
)

// Special is a live display element ([Special character]).
type Special byte

const (
	SpecialDateMDY      Special = 0x20 // MM/DD/YY
	SpecialDateDMY      Special = 0x21 // DD/MM/YY
	SpecialDateMDYDash  Special = 0x22 // MM-DD-YY
	SpecialDateDMYDash  Special = 0x23 // DD-MM-YY
	SpecialDateFull     Special = 0x24 // MM.DD.YYYY
	SpecialYear2        Special = 0x25
	SpecialYear4        Special = 0x26
	SpecialMonth        Special = 0x27
	SpecialMonthName    Special = 0x28
	SpecialDay          Special = 0x29
	SpecialWeekdayNum   Special = 0x2A
	SpecialWeekdayName  Special = 0x2B
	SpecialHour         Special = 0x2C
	SpecialMinute       Special = 0x2D
	SpecialSecond       Special = 0x2E
	SpecialClock        Special = 0x2F // HH:MM, 24h
	SpecialClock12      Special = 0x30 // HH:MM, 12h AM/PM
	SpecialTemperature  Special = 0x31 // °C
	SpecialHumidity     Special = 0x32
	SpecialTemperatureF Special = 0x33 // °F
	SpecialHour12       Special = 0x35
)
