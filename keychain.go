package macwifi

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"time"
)

// PasswordOption configures Password.
type PasswordOption func(*passwordConfig)

type passwordConfig struct {
	beforeAccess func(ssid string)
	timeout      time.Duration
}

// OnKeychainAccess registers fn to run immediately before the macOS
// Keychain lookup. Use this to show a UI notice like "macOS may prompt
// for Keychain access" — the system's own permission dialog can't be
// pre-empted or suppressed, so warning the user is the only way to avoid
// a surprise.
//
// The callback runs synchronously; Password blocks until fn returns.
func OnKeychainAccess(fn func(ssid string)) PasswordOption {
	return func(c *passwordConfig) { c.beforeAccess = fn }
}

// WithTimeout bounds the password lookup. Default 60s (generous because it
// includes the time the user spends looking at the macOS Allow/Deny dialog).
func WithTimeout(d time.Duration) PasswordOption {
	return func(c *passwordConfig) { c.timeout = d }
}

// Password returns the saved WiFi password for ssid, or "" if there is no
// saved entry. The lookup runs inside the signed WifiScanner.app helper so
// the macOS Keychain consent dialog is issued against its stable signing
// identity — the grant (when you add the app in Keychain Access.app) ties
// to that identity and persists across rebuilds.
//
// macOS will show an Allow/Deny dialog the first time per SSID. Clicking
// Allow returns the password once (modern macOS does not offer a
// persistent "Always Allow" button for this class of item). To pre-
// authorize permanently, add WifiScanner.app to the item's Access Control
// tab in Keychain Access.app.
func Password(ssid string, opts ...PasswordOption) (string, error) {
	cfg := &passwordConfig{timeout: 60 * time.Second}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.beforeAccess != nil {
		cfg.beforeAccess(ssid)
	}

	appPath, err := resolveAppPath()
	if err != nil {
		return "", err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("macwifi: listen: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "open",
		"-W",
		"--env", fmt.Sprintf("MACWIFI_PORT=%d", port),
		"--env", "MACWIFI_MODE=password",
		"--env", "MACWIFI_SSID="+ssid,
		appPath,
	)
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
				return "", fmt.Errorf("macwifi: helper exited without connecting: %w", lerr)
			}
		default:
		}
		return "", fmt.Errorf("macwifi: accept helper connection: %w", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(deadline)

	pw, err := decodePasswordResponse(conn)
	// Drain launch goroutine.
	if lerr := <-launchErr; lerr != nil && err == nil {
		return "", fmt.Errorf("macwifi: helper: %w", lerr)
	}
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return "", fmt.Errorf("macwifi: helper closed before sending a password")
		}
		return "", err
	}
	return pw, nil
}
