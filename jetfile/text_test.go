package jetfile

import (
	"bytes"
	"testing"
	"time"
)

func TestTextChainGolden(t *testing.T) {
	got := NewText().
		Font(Font7x6).
		Color(ColorRed).
		Background(ColorBlack).
		In(EffectMoveLeft).
		Out(EffectScrollUp).
		Speed(SpeedFast).
		AlignH(AlignCenter).
		Str("Hi").
		Bytes()
	want := []byte{
		0x1A, '1',
		0x1C, '1',
		0x1D, '0',
		0x0A, 'I', 0x31,
		0x0A, 'O', 0x38,
		0x0F, '1',
		0x1E, '0',
		'H', 'i',
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got  % X\nwant % X", got, want)
	}
}

func TestTextNewlineAndFrames(t *testing.T) {
	got := NewText().Str("a\nb").Frame().Str("c").Bytes()
	want := []byte{'a', 0x0D, 'b', 0x0C, 'c'}
	if !bytes.Equal(got, want) {
		t.Errorf("got % X, want % X", got, want)
	}
}

func TestTextPauseEncodings(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want []byte
	}{
		{3 * time.Second, []byte{0x0E, '0', '0', '3'}},
		{99 * time.Second, []byte{0x0E, '0', '9', '9'}},
		{108 * time.Second, []byte{0x0E, '2', '0', '1', '0', '8'}},
		{50 * time.Millisecond, []byte{0x0E, '1', '5', '0'}},
		{170 * time.Millisecond, []byte{0x0E, '3', '0', '1', '7', '0'}},
		{5 * time.Hour, []byte{0x0E, '2', '9', '9', '9', '9'}}, // clamped
	}
	for _, tc := range cases {
		if got := NewText().Pause(tc.d).Bytes(); !bytes.Equal(got, tc.want) {
			t.Errorf("Pause(%v) = % X, want % X", tc.d, got, tc.want)
		}
	}
}

func TestTextColorRGB(t *testing.T) {
	got := NewText().ColorRGB(0x11, 0x22, 0x33).Bytes()
	want := []byte{0x1C, '/', 0x33, 0x22, 0x11} // BGR order
	if !bytes.Equal(got, want) {
		t.Errorf("got % X, want % X", got, want)
	}
}

func TestTextInsertString(t *testing.T) {
	short := NewText().InsertString('D', "A").Bytes()
	if want := []byte{0x13, 'D', 'A'}; !bytes.Equal(short, want) {
		t.Errorf("short form: got % X, want % X", short, want)
	}
	long := NewText().InsertString('E', "AB").Bytes()
	if want := []byte{0x13, 0x0F, 'E', 'S', 'A', 'B'}; !bytes.Equal(long, want) {
		t.Errorf("long form: got % X, want % X", long, want)
	}
}

func TestTextFileWrapper(t *testing.T) {
	// matches the spec's worked example: text file "0" with plain content
	got := NewText().Str("This is a sample").file('A', '0')
	want := append([]byte{0x01, 'Z', '0', '0', 0x02, 'A', '0'}, "This is a sample"...)
	want = append(want, 0x04)
	if !bytes.Equal(got, want) {
		t.Errorf("got  % X\nwant % X", got, want)
	}
}

func TestTextSpecialClock(t *testing.T) {
	got := NewText().Special(SpecialClock).Bytes()
	if want := []byte{0x0B, 0x2F}; !bytes.Equal(got, want) {
		t.Errorf("got % X, want % X", got, want)
	}
}

func TestTextLineSpacingClamped(t *testing.T) {
	if got := NewText().LineSpacing(42).Bytes(); !bytes.Equal(got, []byte{0x08, '9'}) {
		t.Errorf("got % X", got)
	}
}
