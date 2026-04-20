package macwifi

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// PasswordOption configures Password.
type PasswordOption func(*passwordConfig)

type passwordConfig struct {
	beforeAccess func(ssid string)
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

// Password returns the saved WiFi password for ssid, or "" if there is
// no saved entry. The first call per SSID may trigger a macOS "Allow
// access" dialog from the Keychain; once the user picks "Always Allow"
// on that dialog, subsequent calls are silent.
//
// Use OnKeychainAccess to display a UI notice right before the prompt:
//
//	pw, err := macwifi.Password(ssid,
//	    macwifi.OnKeychainAccess(func(ssid string) {
//	        showHeadsUpDialog(ssid)
//	    }),
//	)
func Password(ssid string, opts ...PasswordOption) (string, error) {
	cfg := &passwordConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.beforeAccess != nil {
		cfg.beforeAccess(ssid)
	}

	cmd := exec.Command("security",
		"find-generic-password",
		"-D", "AirPort network password",
		"-a", ssid,
		"-w")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		// Not-found is an empty password, not an error.
		if strings.Contains(msg, "could not be found") ||
			strings.Contains(msg, "SecKeychainSearchCopyNext") {
			return "", nil
		}
		// User dismissed the consent dialog (errSecUserCanceled = -128).
		if strings.Contains(msg, "User canceled") ||
			strings.Contains(msg, "cancelled by the user") {
			return "", fmt.Errorf("macwifi: keychain access denied by user")
		}
		return "", fmt.Errorf("macwifi: keychain lookup: %w (%s)", err, msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}
