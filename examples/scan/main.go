// Example: scan once, print everything.
//
//	MACWIFI_APP=/path/to/WifiScanner.app go run ./examples/scan
//
// Or, after `make install`, simply `go run ./examples/scan`.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/jaisonerick/macwifi"
)

func main() {
	showPw := flag.String("password", "", "look up Keychain password for this SSID and print it")
	flag.Parse()

	if *showPw != "" {
		pw, err := macwifi.Password(*showPw)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if pw == "" {
			fmt.Fprintln(os.Stderr, "no saved password for", *showPw)
			os.Exit(2)
		}
		fmt.Println(pw)
		return
	}

	appPath, err := macwifi.DefaultAppPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	scanner := &macwifi.Scanner{AppPath: appPath}
	nets, err := scanner.Scan(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan:", err)
		os.Exit(1)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SSID\tRSSI\tCH\tBAND\tWIDTH\tSEC\tBSSID\tFLAGS")
	for _, n := range nets {
		flags := ""
		if n.Current {
			flags += "C"
		}
		if n.Saved {
			flags += "S"
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%d\t%s\t%s\t%s\n",
			n.SSID, n.RSSI, n.Channel, n.ChannelBand, n.ChannelWidth,
			n.Security, n.BSSID, flags)
	}
	tw.Flush()
}
