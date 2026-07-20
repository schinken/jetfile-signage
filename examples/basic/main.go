// Command basic connects to a JetFile II LED sign, prints its state,
// syncs the clock and displays a message.
//
// Usage: go run . -addr 10.0.0.42 -text "Hello b4ckspace"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/b4ckspace/ledboard-lib/jetfile"
)

func main() {
	addr := flag.String("addr", "", "sign address, host or host:port")
	text := flag.String("text", "Hello", "message to display")
	flag.Parse()
	if *addr == "" {
		log.Fatal("-addr is required")
	}

	c, err := jetfile.Dial(*addr)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := c.ConnectionTest(ctx)
	if err != nil {
		log.Fatalf("connection test: %v", err)
	}
	fmt.Printf("sign at %s, program v%04x, fpga v%04x\n",
		info.IP, info.ProgramVersion, info.FPGAVersion)

	if params, err := c.SystemParams(ctx); err == nil {
		fmt.Printf("panel %dx%d px, protocol v%04x\n",
			params.Width, params.Height, params.ProtocolVersion)
	}

	if err := c.SetClock(ctx, time.Now()); err != nil {
		log.Printf("set clock: %v", err)
	}

	msg := jetfile.NewText().
		Font(jetfile.Font7x6).
		Color(jetfile.ColorRed).
		In(jetfile.EffectMoveLeft).
		Speed(jetfile.SpeedMediumFast).
		Str(*text).
		Pause(3 * time.Second)

	if err := c.WriteTextFile(ctx, "0", msg); err != nil {
		log.Fatalf("write text: %v", err)
	}
	fmt.Println("message sent")
}
