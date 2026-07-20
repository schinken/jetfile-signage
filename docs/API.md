# Getting started & API reference

A grouped, one-line-per-function tour of the `jetfile` package. For full
signatures, types and constants see the generated docs on
[pkg.go.dev](https://pkg.go.dev/github.com/schinken/jetfile-signage/jetfile).

```sh
go get github.com/schinken/jetfile-signage/jetfile
```

```go
import "github.com/schinken/jetfile-signage/jetfile"
```

## 30-second start

```go
c, err := jetfile.Dial("10.0.0.42")   // 1. connect (port defaults to :9520)
if err != nil { log.Fatal(err) }
defer c.Close()

ctx := context.Background()

c.SetClock(ctx, time.Now())           // 2. sync the clock

msg := jetfile.NewText().             // 3. build a message...
	Color(jetfile.ColorRed).
	In(jetfile.EffectScrollLeft).
	Str("hello").
	Pause(3 * time.Second)
c.WriteTextFile(ctx, "0", msg)        // ...and display it

st, _ := c.SystemStatus(ctx)          // 4. read the sign's state
```

> Every `Client` method takes a `context.Context` as its first argument (for
> timeout/cancellation); it is omitted from the tables below for brevity.
> Device-side failures come back as a `*jetfile.DeviceError` — see
> [Errors](#errors).

## Connecting

| Function | What it does |
|---|---|
| `Dial(addr, opts...) (*Client, error)` | Connect to a sign over TCP; `addr` without a port defaults to `:9520`. |
| `NewClient(conn, opts...) *Client` | Wrap an existing `net.Conn` (e.g. a `*net.UDPConn` for UDP-only signs). |
| `Client.Close() error` | Close the underlying connection. |

Options passed to `Dial` / `NewClient`:

| Option | What it does |
|---|---|
| `WithAddress(group, unit byte)` | Target one sign on a shared bus (default: broadcast `0,0`). |
| `WithPartition(p byte)` | Default disk for file commands — `'D'` flash (default), `'E'` RAM disk (no flash wear). |
| `WithTimeout(d)` | Per-request I/O timeout (default 5s). |
| `WithSource(s uint16)` | Set the source address field (rarely needed). |

## System information

| Method | What it does |
|---|---|
| `ConnectionTest()` | Ping the sign; returns program/FPGA version, IP and address. |
| `SystemParams()` | Hardware/firmware parameters, including panel width/height in pixels. |
| `SystemStatus()` | Live state: display mode, cabinet/outdoor temperature, humidity, auto-power. |
| `SNMAC()` | Serial number and MAC address. |
| `SystemInfo()` | Firmware's free-form info string. |

## Displaying content

| Method | What it does |
|---|---|
| `WriteTextFile(label, *Text)` | Store a text file (T folder) and play it per the playlist. |
| `WriteStringFile(label, *Text)` | Store a string file (S folder) for embedding via `Text.InsertString`. |
| `WritePictureFile(label, bmp)` | Store a BMP picture file (P folder). |
| `WriteEmergency(*Text, ttl, sound)` | Show a message immediately for `ttl` (0 = until removed), optional buzzer. |
| `ReadTextFile(label)` / `ReadStringFile(label)` / `ReadPictureFile(label)` | Read a stored file back (raw bytes, incl. header). |

Build the content with the [Text builder](#text-builder).

## Playlist & play control

| Method | What it does |
|---|---|
| `RestartPlaylist()` | Replay the file list from the beginning. |
| `ReplayCurrent()` | Restart the file currently playing. |
| `Pause()` / `Resume()` | Freeze / continue playback. |
| `PlayNext()` / `PlayPrevious()` | Step to the next / previous file. |
| `PlayFile(ft, label)` | Interrupt the playlist to play one file once, then resume. |
| `CurrentFile()` | Name of the file currently playing. |
| `SetBuzzer(on, mode, seconds)` | Configure the buzzer (board dependent). |

## Clock

| Method | What it does |
|---|---|
| `Clock()` | Read the sign's current date/time (in its configured time zone). |
| `SetClock(t)` | Set the clock; tries second precision, falls back to minute precision. |

## Power & screen

| Method | What it does |
|---|---|
| `BlackScreen(on)` | Blank / unblank the display. |
| `PowerOff(showNotice)` | Semi-off state; optionally show a shutdown notice instead of a dark panel. |
| `PowerOn()` | Return from semi-off to normal display. |
| `PowerStatus()` | Whether control and driver boards are switched on. |
| `Reset()` | Reboot the sign. |

## Brightness & display tests

| Method | What it does |
|---|---|
| `SetBrightness(x, y, w, h, r, g, b)` | Write a brightness block for a region (board dependent). |
| `StartTest(pattern)` | Enter a built-in test pattern (all-on, all-red, scans, …). |
| `EndTest()` | Leave test mode. |
| `GrayscaleTest(gradual, r, g, b, levels)` | Run a grayscale test (not on all boards). |

## Filesystem

| Method | What it does |
|---|---|
| `ReadDir(path)` | List a folder, e.g. `D:\T\`; returns name, dir flag, size, mtime. |
| `DiskInfo(partition)` | Total/free space and label of a partition (`'D'`, `'E'`, …). |
| `FileExists(path)` | Whether a path exists on the sign. |
| `Mkdir(path)` | Create a single folder. |
| `Rename(from, to)` / `Move(from, to)` | Rename / move a file or folder. |
| `Remove(path)` | Delete one file by path. |
| `RemoveAll(ft)` | Delete all files of a type from the client's partition. |
| `FormatPartition(partition)` | Format a partition (destroys its contents). |
| `ReadFile(path)` / `WriteFile(path, data)` | Read / write raw bytes at an explicit path. |
| `ReadSystemFile(name)` / `WriteSystemFile(name, data)` | Read / write a system file (`CONFIG.SYS`, `SEQUENT.SYS` playlist, …). |

## Pixel streaming

| Method | What it does |
|---|---|
| `StartStream(dir, speed, oneBitRG)` | Enter unlimited-connection display mode for raw pixel pushes. |
| `StreamData(pixels)` | Push pixel data into the stream buffer (split into ordered packets). |
| `StreamStatus()` | Current stream buffer status code. |
| `StopStream()` | Leave stream mode and resume the schedule. |

## Login

| Method | What it does |
|---|---|
| `Login(user, password)` | Authenticate against signs with password management enabled. |
| `Logout()` | End the session. |
| `ChangePassword(user, old, new)` | Change a user's password (requires a prior `Login`). |

## Fire-and-forget (first format)

Lightweight ASCII framing for fast writes; no response awaited.

| Method | What it does |
|---|---|
| `WriteTextSimple(target, *Text)` | Store a text file fire-and-forget; `target` is a 1–2 char label or a 4-char `disk+folder+name` path (e.g. `"ETAA"`). |
| `SendSimple(payload)` | Send one raw first-format frame. |

## Escape hatch

| Method | What it does |
|---|---|
| `Do(*Packet) (*Packet, error)` | Send any command without a typed wrapper; framing, checksum, serial matching and status mapping still apply. |

```go
resp, err := c.Do(ctx, &jetfile.Packet{Cmd: 0x0901, Arg: []byte{0, 0, 3, 0}})
```

## Text builder

`jetfile.NewText()` builds text/string file content by chaining methods.
`Bytes()` returns the raw content; the write methods above wrap it for you.

**Content**

| Method | What it does |
|---|---|
| `Str(s)` | Append text (`\n` becomes a line feed). |
| `Strf(format, a...)` | Append `fmt.Sprintf`-formatted text. |
| `Line()` | Start a new line. |
| `Frame()` | Start a new page. |
| `HalfSpace()` | Insert a half-width space. |
| `Raw(b...)` | Append raw control bytes (for sequences without a helper). |

**Typography & color**

| Method | What it does |
|---|---|
| `Font(f)` | Select a font/size (`Font7x6`, `FontBold16x12`, …). |
| `Color(c)` / `ColorRGB(r, g, b)` | Font color — palette entry or 24-bit RGB. |
| `Background(c)` / `BackgroundRGB(r, g, b)` | Background color. |
| `Flash(on)` | Turn character flashing on/off. |

**Animation & timing**

| Method | What it does |
|---|---|
| `In(e)` / `Out(e)` | Entry / exit animation of the current frame. |
| `Speed(s)` | Animation speed. |
| `Pause(d)` | How long the current frame stays on screen. |

**Layout**

| Method | What it does |
|---|---|
| `AlignH(a)` / `AlignV(a)` | Horizontal / vertical alignment. |
| `LineSpacing(px)` | Spacing between lines (0–9 px). |

**Live inserts & embeds**

| Method | What it does |
|---|---|
| `Special(s)` | Insert a live element the firmware renders — clock, date, temperature, humidity, … |
| `InsertString(drive, label)` | Embed another string file. |
| `InsertPicture(drive, label)` | Embed a picture file. |

## Errors

Device-side failures are returned as `*DeviceError`, carrying the sign's
status code. Match specific conditions with `errors.Is`:

```go
if errors.Is(err, &jetfile.DeviceError{Code: jetfile.StatusFileNotFound}) {
	// file wasn't there
}
```
