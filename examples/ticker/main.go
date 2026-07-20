// Command ticker turns the sign into a live ticker: every line read from
// stdin is pushed to the board as a scrolling message. Writes go to the
// RAM disk ('E') so a fast feed doesn't wear out the flash.
//
//	# pipe anything line-by-line
//	tail -F /var/log/alerts | go run . -addr 10.0.0.42
//	echo "deploy finished" | go run . -addr 10.0.0.42
//
// Ctrl-C leaves the ticker cleanly and clears the message.
package main

import (
	"bufio"
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/schinken/jetfile-signage/jetfile"
)

func main() {
	addr := flag.String("addr", "", "sign address, host or host:port")
	color := flag.String("color", "green", "red|green|yellow")
	flag.Parse()
	if *addr == "" {
		log.Fatal("-addr is required")
	}

	// RAM disk: no flash wear for content that changes constantly.
	c, err := jetfile.Dial(*addr, jetfile.WithPartition('E'))
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// Ctrl-C cancels in-flight writes and triggers the cleanup below.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if text := strings.TrimSpace(sc.Text()); text != "" {
				lines <- text
			}
		}
	}()

	col := colors[*color]
	log.Println("ticker up; feed lines on stdin, Ctrl-C to stop")
	for {
		select {
		case <-ctx.Done():
			cleanup(c)
			return
		case line, ok := <-lines:
			if !ok {
				cleanup(c)
				return
			}
			msg := jetfile.NewText().
				Font(jetfile.Font7x6).
				Color(col).
				In(jetfile.EffectScrollLeft).
				Speed(jetfile.SpeedMediumFast).
				Str(line).
				Pause(200 * time.Millisecond)
			if err := c.WriteTextFile(ctx, "0", msg); err != nil {
				log.Printf("write %q: %v", line, err)
			}
		}
	}
}

var colors = map[string]jetfile.Color{
	"red":    jetfile.ColorRed,
	"green":  jetfile.ColorGreen,
	"yellow": jetfile.ColorYellow,
}

// cleanup runs with a fresh context because ctx is already cancelled.
func cleanup(c *jetfile.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.WriteTextFile(ctx, "0", jetfile.NewText().Str(" ")); err != nil {
		log.Printf("clear: %v", err)
	}
	log.Println("ticker stopped")
}
