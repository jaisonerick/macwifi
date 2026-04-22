package macwifi

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// embeddedVersion is a human-readable cache namespace. The actual cache key
// also includes a digest of the embedded bundle, so updated helper files are
// extracted without requiring a manual version bump. Release Please keeps
// this in sync with the module version on each release.
const embeddedVersion = "1.0.0" // x-release-please-version

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
	cacheKey, err := scannerCacheKey()
	if err != nil {
		return "", fmt.Errorf("macwifi: hash embedded bundle: %w", err)
	}
	versionDir := filepath.Join(cacheBase, "macwifi", cacheKey)
	appPath := filepath.Join(versionDir, "WifiScanner.app")
	marker := filepath.Join(versionDir, ".extracted")

	// Fast path: already extracted at this version.
	if b, err := os.ReadFile(marker); err == nil && string(b) == cacheKey {
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

	if err := os.WriteFile(marker, []byte(cacheKey), 0o644); err != nil {
		return "", err
	}
	return appPath, nil
}

func scannerCacheKey() (string, error) {
	sum, err := fsTreeDigest(scannerBundle, scannerSourcePrefix)
	if err != nil {
		return "", err
	}
	return embeddedVersion + "-" + sum[:12], nil
}

func fsTreeDigest(src fs.FS, srcPrefix string) (string, error) {
	h := sha256.New()
	if err := fs.WalkDir(src, srcPrefix, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(path, srcPrefix)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return nil
		}

		_, _ = h.Write([]byte(rel))
		_, _ = h.Write([]byte{0})
		if d.IsDir() {
			_, _ = h.Write([]byte{1})
			return nil
		}

		data, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
		return nil
	}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyEmbedTree walks srcPrefix inside src and mirrors the tree under dst,
// preserving executability for the inner binary.
func copyEmbedTree(src fs.FS, srcPrefix, dst string) error {
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

		data, err := fs.ReadFile(src, path)
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
