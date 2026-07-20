// Command fsutil is a small filesystem client for a JetFile II sign: list
// folders, show free space, dump a file, or delete one. It shows the file
// control commands and how device-side failures surface as *DeviceError.
//
//	go run . -addr 10.0.0.42 df D
//	go run . -addr 10.0.0.42 ls 'D:\T\'
//	go run . -addr 10.0.0.42 cat 'D:\T\WELCOME.NMG'
//	go run . -addr 10.0.0.42 rm 'D:\T\AB'
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/schinken/jetfile-signage/jetfile"
)

func main() {
	addr := flag.String("addr", "", "sign address, host or host:port")
	flag.Parse()
	args := flag.Args()
	if *addr == "" || len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: fsutil -addr HOST <ls|df|cat|rm> [arg]")
		os.Exit(2)
	}

	c, err := jetfile.Dial(*addr)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := run(ctx, c, args); err != nil {
		// Turn the sign's own status code into a readable message.
		var derr *jetfile.DeviceError
		if errors.As(err, &derr) {
			log.Fatalf("sign refused command: %v", derr)
		}
		log.Fatal(err)
	}
}

func run(ctx context.Context, c *jetfile.Client, args []string) error {
	cmd := args[0]
	arg := ""
	if len(args) > 1 {
		arg = args[1]
	}

	switch cmd {
	case "ls":
		entries, err := c.ReadDir(ctx, arg)
		if err != nil {
			return err
		}
		for _, e := range entries {
			kind, size := "-", fmt.Sprintf("%8d", e.Size)
			if e.Dir {
				kind, size = "d", "     DIR"
			}
			fmt.Printf("%s %s %s  %s\n", kind, size, e.Modified.Format("2006-01-02 15:04"), e.Name)
		}
		return nil

	case "df":
		if arg == "" {
			arg = "D"
		}
		d, err := c.DiskInfo(ctx, arg[0])
		if err != nil {
			return err
		}
		fmt.Printf("%s: %s free of %s (%.0f%% used)\n",
			d.Name, human(d.Free), human(d.Total),
			100*float64(d.Total-d.Free)/float64(d.Total))
		return nil

	case "cat":
		data, err := c.ReadFile(ctx, arg)
		if err != nil {
			return err
		}
		os.Stdout.Write(data)
		return nil

	case "rm":
		ok, err := c.FileExists(ctx, arg)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s: no such file", arg)
		}
		if err := c.Remove(ctx, arg); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", arg)
		return nil
	}
	return fmt.Errorf("unknown command %q", cmd)
}

func human(n uint32) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
