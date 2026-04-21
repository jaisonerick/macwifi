package macwifi_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jaisonerick/macwifi"
)

// One-shot scan of nearby WiFi networks.
func ExampleScan() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	networks, err := macwifi.Scan(ctx)
	if err != nil {
		log.Fatal(err)
	}

	for _, n := range networks {
		fmt.Printf("%s %s %d dBm %s\n",
			n.SSID, n.BSSID, n.RSSI, n.Security)
	}
}

// Read a saved password from the macOS Keychain. The user sees a
// system Allow/Deny dialog; OnKeychainAccess runs just before it
// appears so a CLI or TUI can nudge the user to expect the prompt.
func ExamplePassword() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	password, err := macwifi.Password(ctx, "MyHomeWiFi",
		macwifi.OnKeychainAccess(func(ssid string) {
			fmt.Printf("Approve the macOS Keychain prompt for %q\n", ssid)
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(password)
}

// Reuse one helper session for a scan followed by a password lookup.
// Calling Scan and Password on the same Client avoids relaunching
// the signed helper app between operations.
func ExampleNew() {
	ctx := context.Background()

	c, err := macwifi.New(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	networks, err := c.Scan(ctx)
	if err != nil {
		log.Fatal(err)
	}

	for _, n := range networks {
		if !n.Current {
			continue
		}
		password, err := c.Password(ctx, n.SSID)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("connected: %s (%s)\n", n.SSID, password)
	}
}
