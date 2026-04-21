package macwifi

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestCopyEmbedTree(t *testing.T) {
	src := fstest.MapFS{
		"embedded/WifiScanner.app/Contents/Info.plist": {
			Data: []byte("plist"),
		},
		"embedded/WifiScanner.app/Contents/MacOS/wifi-scanner": {
			Data: []byte("#!/bin/sh\n"),
		},
		"embedded/WifiScanner.app/Contents/_CodeSignature/CodeResources": {
			Data: []byte("signature"),
		},
	}
	dst := filepath.Join(t.TempDir(), "WifiScanner.app")

	if err := copyEmbedTree(src, scannerSourcePrefix, dst); err != nil {
		t.Fatal(err)
	}

	assertFile(t, filepath.Join(dst, "Contents", "Info.plist"), "plist", 0o644)
	assertFile(t, filepath.Join(dst, "Contents", "MacOS", "wifi-scanner"), "#!/bin/sh\n", 0o755)
	assertFile(t, filepath.Join(dst, "Contents", "_CodeSignature", "CodeResources"), "signature", 0o644)
}

func TestFSTreeDigestChangesWhenContentChanges(t *testing.T) {
	src := fstest.MapFS{
		"embedded/WifiScanner.app/Contents/Info.plist": {
			Data: []byte("one"),
		},
	}
	changed := fstest.MapFS{
		"embedded/WifiScanner.app/Contents/Info.plist": {
			Data: []byte("two"),
		},
	}

	got, err := fsTreeDigest(src, scannerSourcePrefix)
	if err != nil {
		t.Fatal(err)
	}
	gotAgain, err := fsTreeDigest(src, scannerSourcePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if got != gotAgain {
		t.Fatalf("fsTreeDigest() = %q, then %q; want stable digest", got, gotAgain)
	}

	changedDigest, err := fsTreeDigest(changed, scannerSourcePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if got == changedDigest {
		t.Fatalf("fsTreeDigest() = %q for different content", got)
	}
}

func TestResolveAppPathUsesExistingOverride(t *testing.T) {
	app := filepath.Join(t.TempDir(), "WifiScanner.app")
	if err := os.Mkdir(app, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MACWIFI_APP", app)

	got, err := resolveAppPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != app {
		t.Fatalf("resolveAppPath() = %q, want %q", got, app)
	}
}

func assertFile(t *testing.T, path, want string, wantMode os.FileMode) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != wantMode {
		t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), wantMode)
	}
}
