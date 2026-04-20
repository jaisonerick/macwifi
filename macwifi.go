// Package macwifi lists visible WiFi networks on macOS with rich metadata
// (BSSID, channel, security mode, …) and reads saved passwords from the
// System keychain. Both operations are served by a small signed Swift
// helper app (WifiScanner.app); macOS requires a signed foreground-app
// context for unredacted CoreWLAN scan results.
//
// Typical usage:
//
//	c, err := macwifi.New(ctx)
//	if err != nil { return err }
//	defer c.Close()
//
//	nets, _ := c.Scan(ctx)
//	pw, _  := c.Password(ctx, "MyHomeWiFi",
//	    macwifi.OnKeychainAccess(func(ssid string) { showHeadsUpDialog(ssid) }),
//	)
//
// One-shot helpers (Scan, Password) wrap New+op+Close for simple cases.
package macwifi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
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
	SecurityUnknown        Security = 0
	SecurityNone           Security = 1
	SecurityWEP            Security = 2
	SecurityWPA            Security = 3
	SecurityWPA2Personal   Security = 4
	SecurityWPA3Personal   Security = 5
	SecurityWPA2Enterprise Security = 6
	SecurityWPA3Enterprise Security = 7
	SecurityOWE            Security = 8
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
	Password     string // always "" from Scan; use Password() separately
	Current      bool   // connected right now
	Saved        bool   // in the preferred-networks list
}

// Client is a live session against the scanner helper. One helper process
// is launched on New and stays alive until Close — subsequent Scan and
// Password calls reuse the same connection (no per-call app launch).
type Client struct {
	cmd   *exec.Cmd
	conn  net.Conn
	mu    sync.Mutex // one request at a time
	done  chan error // cmd.Run() completion
	closed bool
}

// New launches the helper app and returns a ready Client. Close must be
// called to release the helper process. The passed context only bounds
// startup; per-call timeouts are separate.
func New(ctx context.Context) (*Client, error) {
	appPath, err := resolveAppPath()
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("macwifi: listen: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	// MACWIFI_PARENT_PID lets the helper watch our PID via kqueue and
	// exit the instant we die (crash, SIGKILL, panic). Without this, a
	// helper stuck in SecItemCopyMatching would outlive the client.
	cmd := exec.Command("open",
		"-W",
		"--env", fmt.Sprintf("MACWIFI_PORT=%d", port),
		"--env", fmt.Sprintf("MACWIFI_PARENT_PID=%d", os.Getpid()),
		appPath,
	)
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("macwifi: start helper: %w", err)
	}
	go func() { done <- cmd.Wait() }()

	// Wait for the helper to connect back. Bound startup time; 30s covers
	// a first-time Location Services prompt the user has to answer.
	startupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if d, ok := startupCtx.Deadline(); ok {
		_ = listener.(*net.TCPListener).SetDeadline(d)
	}

	conn, err := listener.Accept()
	if err != nil {
		_ = cmd.Process.Kill()
		<-done
		return nil, fmt.Errorf("macwifi: waiting for helper to connect: %w", err)
	}

	return &Client{cmd: cmd, conn: conn, done: done}, nil
}

// Scan requests a one-shot scan and returns the decoded result.
func (c *Client) Scan(ctx context.Context) ([]Network, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("macwifi: client closed")
	}
	if err := setDeadline(c.conn, ctx); err != nil {
		return nil, err
	}
	if err := writeScanRequest(c.conn); err != nil {
		return nil, fmt.Errorf("macwifi: write scan request: %w", err)
	}
	msgType, err := readHeader(c.conn)
	if err != nil {
		return nil, fmt.Errorf("macwifi: read scan response: %w", err)
	}
	if msgType != msgTypeScanResponse {
		return nil, fmt.Errorf("macwifi: expected scan_response, got 0x%02x", msgType)
	}
	return decodeScanResponse(c.conn)
}

// Password requests a saved WiFi password for ssid. Returns "" (no error)
// if there is no saved entry. The helper runs SecItemCopyMatching, which
// may cause macOS to show its own Allow/Deny dialog — use OnKeychainAccess
// to display a TUI heads-up before that happens.
func (c *Client) Password(ctx context.Context, ssid string, opts ...PasswordOption) (string, error) {
	cfg := &passwordConfig{timeout: 60 * time.Second}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.beforeAccess != nil {
		cfg.beforeAccess(ssid)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return "", errors.New("macwifi: client closed")
	}

	// Use the larger of ctx deadline and cfg.timeout — the helper may sit
	// inside SecItemCopyMatching for however long the user takes to answer
	// the system dialog.
	callCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()
	if err := setDeadline(c.conn, callCtx); err != nil {
		return "", err
	}
	if err := writePasswordRequest(c.conn, ssid); err != nil {
		return "", fmt.Errorf("macwifi: write password request: %w", err)
	}
	msgType, err := readHeader(c.conn)
	if err != nil {
		return "", fmt.Errorf("macwifi: read password response: %w", err)
	}
	if msgType != msgTypePasswordResponse {
		return "", fmt.Errorf("macwifi: expected password_response, got 0x%02x", msgType)
	}
	return decodePasswordResponse(c.conn)
}

// Close sends a clean shutdown request to the helper, closes the socket,
// and waits for the helper process to exit. Safe to call multiple times.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	_ = writeCloseRequest(c.conn)
	_ = c.conn.Close()
	c.mu.Unlock()

	// Wait for the helper to exit, bounded.
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		_ = c.cmd.Process.Kill()
		<-c.done
	}
	return nil
}

// Scan is a one-shot helper: New → Scan → Close.
func Scan(ctx context.Context) ([]Network, error) {
	c, err := New(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return c.Scan(ctx)
}

// Password is a one-shot helper: New → Password → Close. Prefer New/Close
// + multiple calls if you know you'll do more than one operation.
func Password(ctx context.Context, ssid string, opts ...PasswordOption) (string, error) {
	c, err := New(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()
	return c.Password(ctx, ssid, opts...)
}

// PasswordOption configures a Password call.
type PasswordOption func(*passwordConfig)

type passwordConfig struct {
	beforeAccess func(ssid string)
	timeout      time.Duration
}

// OnKeychainAccess registers fn to run just before the helper calls
// SecItemCopyMatching, which is typically when macOS will pop its Keychain
// access dialog. Callback runs synchronously; Password blocks until fn
// returns.
func OnKeychainAccess(fn func(ssid string)) PasswordOption {
	return func(c *passwordConfig) { c.beforeAccess = fn }
}

// WithTimeout bounds a single Password call. Defaults to 60s (generous to
// leave room for the user reading the macOS dialog).
func WithTimeout(d time.Duration) PasswordOption {
	return func(c *passwordConfig) { c.timeout = d }
}

// ─────────────────────────── internals ────────────────────────────────────

func setDeadline(c net.Conn, ctx context.Context) error {
	d, ok := ctx.Deadline()
	if !ok {
		return c.SetDeadline(time.Time{})
	}
	return c.SetDeadline(d)
}

// resolveAppPath returns the path to the signed helper bundle. In the
// common case the bundle is baked into the Go binary (via go:embed) and
// extracted on first use. Developers can override with $MACWIFI_APP to
// point at a locally-built scanner for iteration.
func resolveAppPath() (string, error) {
	if p := os.Getenv("MACWIFI_APP"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return extractScannerApp()
}
