package macwifi

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// embeddedVersion is bumped every time the embedded WifiScanner.app
// changes. Used to invalidate the on-disk cache so consumers pick up
// the newer bundle without manual cleanup.
const embeddedVersion = "0.1.0"

// scannerBundle is the signed + notarized WifiScanner.app. Every file
// under embedded/WifiScanner.app/ is baked into the Go binary — consumers
// `go get` the module and get the helper automatically.
//
// The `all:` prefix is required because the bundle contains a
// `_CodeSignature` directory; embed otherwise skips names starting
// with `_` or `.`.
//
//go:embed all:embedded/WifiScanner.app
var scannerBundle embed.FS

// scannerSourcePrefix is the root path inside scannerBundle we copy out.
const scannerSourcePrefix = "embedded/WifiScanner.app"

// extractScannerApp materializes the embedded bundle to the user's cache
// dir and returns the absolute path to the extracted .app. Idempotent:
// re-extracts only when embeddedVersion changes or the marker is missing.
func extractScannerApp() (string, error) {
	cacheBase, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("macwifi: locate cache dir: %w", err)
	}
	versionDir := filepath.Join(cacheBase, "macwifi", embeddedVersion)
	appPath := filepath.Join(versionDir, "WifiScanner.app")
	marker := filepath.Join(versionDir, ".extracted")

	// Fast path: already extracted at this version.
	if b, err := os.ReadFile(marker); err == nil && string(b) == embeddedVersion {
		return appPath, nil
	}

	// Clean any prior (partial) extraction at this version.
	if err := os.RemoveAll(versionDir); err != nil {
		return "", fmt.Errorf("macwifi: clean cache dir: %w", err)
	}
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return "", fmt.Errorf("macwifi: create cache dir: %w", err)
	}

	if err := copyEmbedTree(scannerBundle, scannerSourcePrefix, appPath); err != nil {
		_ = os.RemoveAll(versionDir)
		return "", fmt.Errorf("macwifi: extract bundle: %w", err)
	}

	if err := os.WriteFile(marker, []byte(embeddedVersion), 0o644); err != nil {
		return "", err
	}
	return appPath, nil
}

// copyEmbedTree walks srcPrefix inside src and mirrors the tree under dst,
// preserving executability for the inner binary.
func copyEmbedTree(src embed.FS, srcPrefix, dst string) error {
	return fs.WalkDir(src, srcPrefix, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(path, srcPrefix)
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := src.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		// The inner Mach-O binary must be executable. Everything else is
		// plain data (Info.plist, code signature blobs, notarization ticket).
		if rel == "Contents/MacOS/wifi-scanner" {
			mode = 0o755
		}
		return os.WriteFile(target, data, mode)
	})
}
