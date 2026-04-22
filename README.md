# macwifi

[![Go Reference](https://pkg.go.dev/badge/github.com/jaisonerick/macwifi.svg)](https://pkg.go.dev/github.com/jaisonerick/macwifi)
[![CI](https://github.com/jaisonerick/macwifi/actions/workflows/ci.yml/badge.svg)](https://github.com/jaisonerick/macwifi/actions/workflows/ci.yml)

Wi-Fi scanning and Keychain password access for Go programs on macOS 13+.

macOS 14.4 removed the `airport` CLI tool. `wdutil info` returns
`BSSID : <redacted>` even with `sudo`, and CoreWLAN's `scanForNetworks`
only returns real BSSIDs to apps signed with a stable Developer ID that
have Location Services permission — which scripts and most CLI tools
can't get ([Apple DTS forum thread][apple-dts]).

`macwifi` closes that gap by embedding a Developer-ID-signed and
notarized Swift helper bundle inside the Go package. Your Go binary
spawns the helper on first use to trigger the macOS Location Services
prompt; subsequent calls reuse it.

Looking for a ready-to-run command instead of a library? See
[`macwifi-cli`](https://github.com/jaisonerick/macwifi-cli) — a
drop-in `airport`-replacement built on this package:

```sh
brew install jaisonerick/tap/macwifi-cli
```

[apple-dts]: https://developer.apple.com/forums/thread/718331

## What you can build with it

- Network diagnostics CLIs and TUIs.
- IT, inventory, and support utilities that need local Wi-Fi context.
- Security and audit tools that need BSSID, channel, band, and security
  mode.
- Migration or recovery tools that need user-approved access to saved
  Wi-Fi passwords.
- Desktop agents that need a Go API while still respecting macOS
  privacy prompts.

Out of scope: cross-platform Wi-Fi abstraction, packet capture,
background daemons that bypass Location Services, or privacy-control
workarounds. Location Services and Keychain access stay under user
control.

## What You Get

WiFi data for saved SSIDs and reachable networks through a simple interface:

```go
// Asks for Location Services permission on first request.
networks, err := macwifi.Scan(context.Background())
// networks: []macwifi.Network{
// 	{
// 		SSID:         "Office WiFi",
// 		BSSID:        "aa:bb:cc:dd:ee:ff",
// 		RSSI:         -52,
// 		Channel:      149,
// 		ChannelBand:  macwifi.Band5GHz,
// 		ChannelWidth: 80,
// 		Security:     macwifi.SecurityWPA2Personal,
// 		Current:      true,
// 		Saved:        true,
// 	},
// }

// Asks for Keychain password access.
password, err := macwifi.Password(context.Background(), "MyHomeWiFi")
// password: '<my-password>'
```

Available data:

| Field | Description |
| --- | --- |
| `SSID` | WiFi network name. |
| `BSSID` | Access point MAC address, available after Location Services approval. |
| `RSSI` | Signal strength in dBm. Closer to zero is stronger. |
| `Noise` | Noise floor in dBm, when macOS reports it. |
| `Channel` | WiFi channel number. |
| `ChannelBand` | `2.4GHz`, `5GHz`, `6GHz`, or `unknown`. |
| `ChannelWidth` | Channel bandwidth in MHz. |
| `Security` | Open, WEP, WPA, WPA2, WPA3, enterprise, OWE, or unknown. |
| `PHYMode` | 802.11 mode when available. |
| `Current` | Whether the Mac is connected to this network now. |
| `Saved` | Whether the SSID is in the Mac's preferred networks list. |
| `Password` | Always empty from `Scan`; use `Password(ctx, ssid)` when needed. |

Saved networks that are not currently visible may be included with signal and
channel fields set to zero.

## Usage

### Install

```sh
go get github.com/jaisonerick/macwifi
```

Requirements:

- macOS 13 or newer on Apple Silicon.
- Go 1.26 or newer.

Permissions requested:

- Location Services for WiFi discovery.
- macOS Keychain for password retrieval.

### Versioning

`macwifi` follows [Semantic Versioning](https://semver.org/). The exported
Go API of the `macwifi` package — types, functions, and option helpers —
is stable across the v1.x line: no breaking changes will land without a
major-version bump. The wire protocol between the Go client and the
embedded helper is an internal implementation detail and is not covered
by this commitment.

Versions before v1.0.0 are retracted in `go.mod` when they cannot meet
the macOS support floor; `go get` and tooling like Dependabot will steer
you to the latest non-retracted release.

### Reuse One Helper Session

When scanning for networks and then fetching a password in one run, create a
client and reuse it:

```go
ctx := context.Background()

c, err := macwifi.New(ctx)
if err != nil {
	panic(err)
}
defer c.Close()

networks, err := c.Scan(ctx)
if err != nil {
	panic(err)
}

password, err := c.Password(ctx, "MyHomeWiFi",
	macwifi.OnKeychainAccess(func(ssid string) {
		fmt.Printf("Approve the macOS Keychain prompt to read %q\n", ssid)
	}))
if err != nil {
	panic(err)
}

fmt.Println(len(networks), password)
```

### Keychain Passwords

The legacy **Always Allow** path is no longer available in the macOS Keychain
access permission dialog, so users will see the prompt every time
`macwifi.Password()` runs for an SSID.

Use `OnKeychainAccess` to prepare users before macOS shows its dialog:

```go
password, err := macwifi.Password(ctx, ssid,
	macwifi.OnKeychainAccess(func(ssid string) {
		fmt.Printf("Approve the macOS Keychain prompt to read %q\n", ssid)
	}))
```

## Development

Run the example scanner:

```sh
go run ./examples/scan
go run ./examples/scan --password MyHomeWiFi
```

Run tests:

```sh
make ci-go
```

Build and use a local helper while editing Swift code:

```sh
make scanner
MACWIFI_APP=$PWD/WifiScanner.app go run ./examples/scan
```

`MACWIFI_APP` tells the Go package to use a local helper bundle instead of the
embedded one.

## Releases

Releases are fully automated. Day-to-day:

1. Land changes on `main` using [Conventional Commits](https://www.conventionalcommits.org/).
2. [Release Please](https://github.com/googleapis/release-please) keeps a
   rolling Release PR open with the next `CHANGELOG.md` entry, the
   `go.mod`-aware version bump, and the `embeddedVersion` constant in
   `embed.go`.
3. Merging the Release PR tags the version, publishes a GitHub Release,
   primes `proxy.golang.org`, and attaches the signed `WifiScanner.app`
   zip to the release.

The signed companion workflow handles the helper bundle. Pull requests
that touch `scanner/Sources/`, `scanner/Info.plist`,
`scanner/entitlements.plist`, or the helper build scripts trigger a
macOS runner that builds, codesigns with Developer ID, notarizes,
staples, and commits the regenerated `embedded/WifiScanner.app` back to
the PR branch before merge. The workflow expects the environment
secrets documented in
[`.github/SIGNING_SECRETS.md`](.github/SIGNING_SECRETS.md).

### Local helper rebuild (development only)

When iterating on Swift code, you don't need notarization — just an
ad-hoc signed bundle pointed at via `MACWIFI_APP`:

```sh
make scanner
MACWIFI_APP=$PWD/WifiScanner.app go run ./examples/scan
```

A full Developer ID + notarized release build can be produced locally
with `make release`, but it is not required to ship — CI handles that
end-to-end.

## Contributing

Issues and pull requests are welcome, especially for:

- Compatibility reports across macOS releases and hardware.
- Better metadata mapping from CoreWLAN into the Go API.
- Documentation improvements for real-world diagnostics, support, and security
  use cases.

For code changes, run `make ci-macos` before opening a pull request. If your
change touches the Swift helper, also test with `make scanner` and
`MACWIFI_APP=$PWD/WifiScanner.app go run ./examples/scan`.

Changes to the helper app must pass the signed companion workflow because
signing and notarization are automated before merge.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full contribution workflow,
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) for community expectations,
[SECURITY.md](SECURITY.md) for vulnerability reporting, and
[SUPPORT.md](SUPPORT.md) for support expectations.

## License

MIT. See [LICENSE](LICENSE).
