// Command dashboard builds a multi-page information display: a welcome
// banner, a live clock/date page, and — if the sign has sensors — a page
// with cabinet temperature and humidity read straight off the hardware.
// It shows off the Text builder (fonts, colors, effects, alignment,
// per-frame timing) and the [Special] live inserts the firmware renders
// on its own.
//
// Usage: go run . -addr 10.0.0.42 -name "b4ckspace"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/schinken/jetfile-signage/jetfile"
)

func main() {
	addr := flag.String("addr", "", "sign address, host or host:port")
	name := flag.String("name", "hello", "banner text")
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

	// A correct clock makes the live [Special] inserts meaningful.
	if err := c.SetClock(ctx, time.Now()); err != nil {
		log.Printf("set clock: %v", err)
	}

	board := jetfile.NewText().
		// Page 1: the banner, scrolled in, held for 4s.
		Font(jetfile.FontBold16x12).
		Color(jetfile.ColorRainbowWave).
		AlignH(jetfile.AlignCenter).
		AlignV(jetfile.AlignMiddle).
		In(jetfile.EffectScrollLeft).
		Speed(jetfile.SpeedMedium).
		Str(*name).
		Pause(4 * time.Second).

		// Page 2: date over a live 24h clock, two lines.
		Frame().
		Font(jetfile.Font7x6).
		Color(jetfile.ColorYellow).
		In(jetfile.EffectFoldLR).
		AlignH(jetfile.AlignCenter).
		Special(jetfile.SpecialWeekdayName).Str(" ").
		Special(jetfile.SpecialDateFull).
		Line().
		Color(jetfile.ColorGreen).
		Special(jetfile.SpecialClock).
		Pause(4 * time.Second)

	// Page 3: only worth showing if a sensor is actually wired up.
	if st, err := c.SystemStatus(ctx); err == nil && st.CabinetTemp != -1 {
		board.Frame().
			Font(jetfile.Font7x6).
			In(jetfile.EffectMoveUp).
			AlignH(jetfile.AlignLeft).
			Color(jetfile.ColorRed).
			Str("TEMP ").Special(jetfile.SpecialTemperature).
			Line().
			Color(jetfile.ColorGreen).
			Str("HUM  ").Special(jetfile.SpecialHumidity).
			Pause(4 * time.Second)
		fmt.Printf("sensor page added (cabinet %d°C, %d%% RH)\n", st.CabinetTemp, st.Humidity)
	}

	if err := c.WriteTextFile(ctx, "0", board); err != nil {
		log.Fatalf("write dashboard: %v", err)
	}
	fmt.Println("dashboard written to text file 0")
}
