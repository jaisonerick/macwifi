//go:build darwin

package macwifi_test

import (
	"context"
	"testing"
	"time"

	"github.com/jaisonerick/macwifi"
)

// TestClientLifecycle exercises the Go↔helper launch path end-to-end:
// extract the embedded bundle, launch it via `open -W`, accept the
// helper's loopback callback, send a close_request, and wait for a
// clean process exit.
//
// Deliberately avoids Scan/Password so no Location Services or
// Keychain prompt can appear on interactive machines. Skipped in
// -short mode because it spawns a real GUI app and round-trips through
// macOS launch services.
func TestClientLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("launches the signed helper app; skipped under -short")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := macwifi.New(ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Close is documented as idempotent; a second call must not error.
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
