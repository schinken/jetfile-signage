// Command badapple plays a monochrome video on the LED panel through the
// pixel-streaming API (0x08) — the traditional "Bad Apple!!" torture test,
// but any video works. It reads a stream of 8-bit grayscale frames (one byte
// per pixel, panel-sized, row-major), thresholds each to on/off, and pushes
// it as an RG frame.
//
// Point it at a file and it drives ffmpeg for you:
//
//	go run . -addr 10.0.0.42 -video badapple.mp4 -loop
//
// Or pipe frames in yourself (handy for other tools or a different scaler):
//
//	ffmpeg -i badapple.mp4 -vf scale=96:32,format=gray -r 30 \
//	       -f rawvideo -pix_fmt gray - | go run . -addr 10.0.0.42 -w 96 -h 32
//
// Hardware/packing notes live in the shared ledframe package.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/schinken/jetfile-signage/examples/internal/ledframe"
	"github.com/schinken/jetfile-signage/jetfile"
)

func main() {
	addr := flag.String("addr", "", "sign address, host or host:port")
	wFlag := flag.Int("w", 0, "panel width in pixels (0 = ask the sign)")
	hFlag := flag.Int("h", 0, "panel height in pixels (0 = ask the sign)")
	fps := flag.Int("fps", 30, "frames per second")
	video := flag.String("video", "", "video file to play (needs ffmpeg on PATH); empty = read gray frames from stdin")
	threshold := flag.Int("threshold", 128, "gray level (0-255) above which a pixel is lit")
	invert := flag.Bool("invert", false, "light the dark areas instead of the bright ones")
	color := flag.String("color", "green", "lit color: green|amber|red")
	loop := flag.Bool("loop", false, "loop the video (only with -video)")
	flag.Parse()
	if *addr == "" {
		log.Fatal("-addr is required")
	}

	c, err := jetfile.Dial(*addr)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	w, h := *wFlag, *hFlag
	if w == 0 || h == 0 {
		if p, err := c.SystemParams(ctx); err == nil && p.Width > 0 && p.Height > 0 {
			w, h = int(p.Width), int(p.Height)
		}
	}
	if w <= 0 || h <= 0 {
		w, h = 64, 16
	}

	lit, ok := colors[*color]
	if !ok {
		log.Fatalf("unknown -color %q (green|amber|red)", *color)
	}

	// Grayscale frame source: ffmpeg for a file, otherwise stdin.
	var src io.Reader = os.Stdin
	if *video != "" {
		r, cleanup, err := ffmpeg(ctx, *video, w, h, *fps, *loop)
		if err != nil {
			log.Fatal(err)
		}
		defer cleanup()
		src = r
	} else if isTerminal(os.Stdin) {
		log.Fatalf("no -video and nothing piped in; try -video FILE, or pipe %dx%d gray frames on stdin", w, h)
	}

	log.Printf("playing on %dx%d, %d fps (Ctrl-C to stop)", w, h, *fps)
	if err := c.StartStream(ctx, jetfile.MoveLeft, 0, true); err != nil {
		log.Fatalf("start stream: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c.StopStream(ctx)
	}()

	gray := make([]byte, w*h)
	tick := time.NewTicker(time.Second / time.Duration(max(*fps, 1)))
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("bye")
			return
		case <-tick.C:
			if _, err := io.ReadFull(src, gray); err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF || ctx.Err() != nil {
					log.Println("end of video")
					return
				}
				log.Fatalf("read frame: %v", err)
			}
			if err := c.StreamData(ctx, render(gray, w, h, *threshold, *invert, lit)); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("stream: %v", err)
			}
		}
	}
}

var colors = map[string]int{
	"green": ledframe.Green,
	"amber": ledframe.Amber,
	"red":   ledframe.Red,
}

// render thresholds one grayscale frame into a packed RG frame.
func render(gray []byte, w, h, threshold int, invert bool, lit int) []byte {
	f := ledframe.New(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			on := int(gray[y*w+x]) >= threshold
			if invert {
				on = !on
			}
			if on {
				f.Set(x, y, lit)
			}
		}
	}
	return f.Bytes()
}

// ffmpeg starts ffmpeg decoding video into panel-sized 8-bit gray frames on
// its stdout, and returns a reader over them plus a cleanup func.
func ffmpeg(ctx context.Context, video string, w, h, fps int, loop bool) (io.Reader, func(), error) {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if loop {
		args = append(args, "-stream_loop", "-1")
	}
	args = append(args,
		"-i", video,
		"-vf", fmt.Sprintf("scale=%d:%d,format=gray", w, h),
		"-r", fmt.Sprint(fps),
		"-f", "rawvideo", "-pix_fmt", "gray", "-",
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("starting ffmpeg (is it installed?): %w", err)
	}
	return out, func() { cmd.Wait() }, nil
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
