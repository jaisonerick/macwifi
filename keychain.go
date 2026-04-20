package macwifi

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Password returns the saved WiFi password for the given SSID from the
// macOS System keychain. Uses /usr/bin/security, which is pre-approved by
// macOS to read WiFi passwords without prompting — this avoids the Keychain
// "allow access" dialog that would appear for every lookup made from our
// own signed helper app.
//
// Returns "" with no error if there's no saved password for the SSID.
func Password(ssid string) (string, error) {
	cmd := exec.Command("security",
		"find-generic-password",
		"-D", "AirPort network password",
		"-a", ssid,
		"-w")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "could not be found") ||
			strings.Contains(msg, "SecKeychainSearchCopyNext") {
			return "", nil
		}
		return "", fmt.Errorf("macwifi: keychain lookup: %w (%s)", err, msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}
