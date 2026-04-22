module github.com/jaisonerick/macwifi

go 1.26.1

// v0.1.0 was tagged from an off-branch local snapshot that never landed
// on main. v0.1.1 and v0.1.2 shipped a helper compiled without an
// explicit Swift target; the resulting bundle's Mach-O LC_BUILD_VERSION
// pins macOS 15.0 minimum, so the helper refuses to launch on macOS 13
// and 14 even though the README and Info.plist advertise macOS 13+. Use
// v0.1.3 or later.
retract (
	v0.1.2
	v0.1.1
	v0.1.0
)
