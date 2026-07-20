# ledboard-lib

Go client for LED signs speaking the **JetFile II** protocol (Chainzone /
Texcellent controllers, e.g. Sigma series). Implements the binary
communication format of spec v2.5.4 (`docs/JetFileII_v2.5.4.pdf`) over TCP
port 9520, plus the lightweight ASCII format for quick fire-and-forget
updates.

Pure standard library, no dependencies.

```
go get github.com/b4ckspace/ledboard-lib/jetfile
```

## Quick start

```go
c, err := jetfile.Dial("10.0.0.42") // port defaults to :9520
if err != nil { ... }
defer c.Close()

ctx := context.Background()

// say hello
msg := jetfile.NewText().
	Font(jetfile.Font7x6).
	Color(jetfile.ColorRed).
	In(jetfile.EffectMoveLeft).
	Str("Hello").
	Pause(3 * time.Second)
err = c.WriteTextFile(ctx, "0", msg)

// keep the sign's clock right
err = c.SetClock(ctx, time.Now())

// what's it doing?
status, err := c.SystemStatus(ctx)
```

A runnable version lives in `examples/basic`.

## Text builder

`jetfile.NewText()` builds text/string file content from the protocol's
display control characters. Everything chains:

| Method | Effect |
|---|---|
| `Str`, `Strf`, `Line`, `Frame` | content, new line, new page (`\n` in `Str` = new line) |
| `Font`, `Color`, `ColorRGB`, `Background` | typography and palette |
| `In`, `Out`, `Speed`, `Pause`, `Flash` | frame animations and timing |
| `AlignH`, `AlignV`, `LineSpacing` | layout |
| `Special` | live clock / date / temperature / humidity inserts |
| `InsertString`, `InsertPicture` | embed other files |
| `Raw` | any control sequence without a helper |

## Command coverage

| Protocol area | Methods |
|---|---|
| Info (0x01, 0x03) | `ConnectionTest`, `SystemParams`, `SystemStatus`, `SNMAC`, `SystemInfo` |
| Files: labeled (0x0104-07, 0x0204-07) | `ReadTextFile`, `ReadStringFile`, `ReadPictureFile`, `WriteTextFile`, `WriteStringFile`, `WritePictureFile` |
| Files: paths & system (0x0102/0202, 0x0108/0208) | `ReadSystemFile`, `WriteSystemFile` (playlist etc.), `ReadFile`, `WriteFile` |
| Emergency / brightness (0x0209, 0x020A) | `WriteEmergency`, `SetBrightness` |
| Display tests (0x03) | `StartTest`, `EndTest`, `GrayscaleTest` |
| Black screen / power (0x04) | `BlackScreen`, `Reset`, `PowerOff`, `PowerOn`, `PowerStatus` |
| Clock (0x05) | `Clock`, `SetClock` (BCD, time zone codes, legacy fallback) |
| Play control (0x06) | `RestartPlaylist`, `ReplayCurrent`, `Pause`, `Resume`, `PlayNext`, `PlayPrevious`, `PlayFile`, `CurrentFile`, `SetBuzzer` |
| File control (0x07) | `FormatPartition`, `Mkdir`, `Rename`, `Move`, `Remove`, `RemoveAll`, `ReadDir`, `DiskInfo`, `FileExists` |
| Pixel streaming (0x08) | `StartStream`, `StreamData`, `StreamStatus`, `StopStream` |
| Login (0x0A) | `Login`, `Logout`, `ChangePassword` |
| First format | `WriteTextSimple`, `SendSimple` (fire-and-forget, e.g. RAM-disk updates) |

Writes are split into ordered 512-byte packets, reads are reassembled from
pages automatically. Failures surface as `*jetfile.DeviceError` carrying
the sign's status code:

```go
if errors.Is(err, &jetfile.DeviceError{Code: jetfile.StatusFileNotFound}) { ... }
```

### Anything else: the escape hatch

Commands without a wrapper (absolute address access, font uploads, CPU
update checks, non-word-wrap mode ...) can be sent raw; framing, checksum,
serial matching and status mapping still apply:

```go
resp, err := c.Do(ctx, &jetfile.Packet{Cmd: 0x0901, Arg: []byte{0, 0, 3, 0}})
```

## Notes for hardware

- Signs listen on TCP **and** UDP 9520. For UDP, bring your own conn:
  `jetfile.NewClient(udpConn)`.
- Multiple signs on one bus: `jetfile.WithAddress(group, unit)`;
  the default is broadcast (0, 0).
- `jetfile.WithPartition('E')` targets the RAM disk — no flash wear for
  content that updates often. Flash (`'D'`, default) survives power cycles.
- Checksum is the 16-bit truncated byte sum from the length field to the
  end of the frame; verified against the spec's appendix and the
  independent [johnoneil/LEDSign](https://github.com/johnoneil/LEDSign)
  implementation. Field order and quirks (BCD clock, reversed IP bytes,
  FAT directory entries) come straight from the spec's tables.

## Testing

Everything is unit-tested against an in-memory fake sign (`net.Pipe`), no
hardware needed:

```
go test ./...
```

## References

- `docs/JetFileII_v2.5.4.pdf` — the protocol spec this implements
- [b4ckspace/ledboard-v2](https://github.com/b4ckspace/ledboard-v2) — the
  previous UDP/first-format implementation this library replaces
