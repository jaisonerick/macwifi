// Package macwifi lists visible WiFi networks on macOS with richer
// information than the built-in CLI tools expose — including BSSID, channel,
// security mode, and saved Keychain passwords.
//
// Under the hood it launches a small signed Swift helper application
// (WifiScanner.app) via `open -W`, which is required for macOS to give it
// the Location Services permission needed for unredacted SSIDs. Results
// come back over a loopback TCP socket as a binary protocol (see
// protocol.go). See the README for how to build and install the helper.
package macwifi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Band classifies a WiFi channel's radio band.
type Band uint8

const (
	BandUnknown Band = 0
	Band24GHz   Band = 1
	Band5GHz    Band = 2
	Band6GHz    Band = 3
)

func (b Band) String() string {
	switch b {
	case Band24GHz:
		return "2.4GHz"
	case Band5GHz:
		return "5GHz"
	case Band6GHz:
		return "6GHz"
	default:
		return "unknown"
	}
}

// Security classifies a WiFi network's authentication mode.
type Security uint8

const (
	SecurityUnknown       Security = 0
	SecurityNone          Security = 1
	SecurityWEP           Security = 2
	SecurityWPA           Security = 3
	SecurityWPA2Personal  Security = 4
	SecurityWPA3Personal  Security = 5
	SecurityWPA2Enterprise Security = 6
	SecurityWPA3Enterprise Security = 7
	SecurityOWE           Security = 8
)

func (s Security) String() string {
	switch s {
	case SecurityNone:
		return "none"
	case SecurityWEP:
		return "WEP"
	case SecurityWPA:
		return "WPA"
	case SecurityWPA2Personal:
		return "WPA2"
	case SecurityWPA3Personal:
		return "WPA3"
	case SecurityWPA2Enterprise:
		return "WPA2-Enterprise"
	case SecurityWPA3Enterprise:
		return "WPA3-Enterprise"
	case SecurityOWE:
		return "OWE"
	default:
		return "unknown"
	}
}

// Network is one WiFi network observed by the scanner.
type Network struct {
	SSID         string
	BSSID        string // six-octet MAC, lower-case colon-separated; empty if unavailable
	RSSI         int    // signal strength, dBm
	Noise        int    // noise floor, dBm
	Channel      int    // channel number
	ChannelBand  Band   // 2.4/5/6 GHz
	ChannelWidth int    // bandwidth in MHz (20/40/80/160)
	Security     Security
	PHYMode      string // "802.11ax" etc.
	Password     string // from macOS Keychain if accessible; "" otherwise
	Current      bool   // connected right now
	Saved        bool   // in the preferred-networks list
}

// Scanner is a zero-config entry point. The path to the bundled Swift helper
// app is resolved internally — set $MACWIFI_APP to override for testing.
type Scanner struct {
	// Timeout bounds a single scan end-to-end. Defaults to 30s if zero.
	Timeout time.Duration
}

// Scan performs a one-shot scan and returns every network in range (plus any
// saved-but-not-in-range networks as stub records with Saved=true).
func (s *Scanner) Scan(ctx context.Context) ([]Network, error) {
	appPath, err := resolveAppPath()
	if err != nil {
		return nil, err
	}

	timeout := s.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("macwifi: listen: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	args := []string{"-W", "--env", fmt.Sprintf("MACWIFI_PORT=%d", port), appPath}

	cmd := exec.CommandContext(ctx, "open", args...)
	launchErr := make(chan error, 1)
	go func() { launchErr <- cmd.Run() }()

	deadline, _ := ctx.Deadline()
	if tcpL, ok := listener.(*net.TCPListener); ok {
		_ = tcpL.SetDeadline(deadline)
	}

	conn, err := listener.Accept()
	if err != nil {
		select {
		case lerr := <-launchErr:
			if lerr != nil {
				return nil, fmt.Errorf("macwifi: helper exited without connecting: %w", lerr)
			}
		default:
		}
		return nil, fmt.Errorf("macwifi: accept helper connection: %w", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(deadline)

	nets, err := decodeScanResponse(conn)
	// Drain launch goroutine.
	if lerr := <-launchErr; lerr != nil && err == nil {
		// The helper connected and wrote but exited non-zero. Surface only
		// if we don't already have a decoded error.
		return nil, fmt.Errorf("macwifi: helper: %w", lerr)
	}
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("macwifi: helper closed before sending results")
		}
		return nil, fmt.Errorf("macwifi: decode: %w", err)
	}
	return nets, nil
}

// resolveAppPath finds WifiScanner.app, in order:
//  1. $MACWIFI_APP (full path) — escape hatch for testing.
//  2. <executable dir>/../share/macwifi/WifiScanner.app
//  3. $HOME/.local/share/macwifi/WifiScanner.app
//  4. /usr/local/share/macwifi/WifiScanner.app
//
// Intentionally not exported: picking the helper binary is a library
// concern, not a caller concern.
func resolveAppPath() (string, error) {
	if p := os.Getenv("MACWIFI_APP"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exe), "..", "share", "macwifi", "WifiScanner.app"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".local/share/macwifi/WifiScanner.app"))
	}
	candidates = append(candidates, "/usr/local/share/macwifi/WifiScanner.app")
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("macwifi: WifiScanner.app not installed. Run `make install` in the macwifi checkout (or set $MACWIFI_APP)")
}
