# Changelog

## [0.1.2](https://github.com/jaisonerick/macwifi/compare/v0.1.1...v0.1.2) (2026-04-21)


### Documentation

* Add Go Reference and CI badges to the README ([#15](https://github.com/jaisonerick/macwifi/pull/15))
* Add runnable examples (`ExampleScan`, `ExamplePassword`, `ExampleNew`) so pkg.go.dev renders usage inline ([#15](https://github.com/jaisonerick/macwifi/pull/15))
* Add header comments on `Band` and `Security` const blocks and doc comments on their `String` methods ([#15](https://github.com/jaisonerick/macwifi/pull/15))


## 0.1.1 (2026-04-21)

Initial public release.


### Features

* Scan nearby WiFi networks with full metadata — SSID, BSSID, RSSI, noise, channel, band, width, security mode, PHY mode
* Read saved WiFi passwords from the macOS Keychain via `SecItemCopyMatching`
* Embedded signed + notarized Swift helper (`go:embed`) — zero-config for consumers, extracted to the user cache on first use
* Persistent `Client` session — reuse one helper process across multiple `Scan`/`Password` calls
* One-shot convenience functions `macwifi.Scan` and `macwifi.Password` for simple use cases
* `OnKeychainAccess` callback for surfacing a CLI/TUI heads-up before the macOS Keychain dialog
* `WithTimeout` option for bounding `Password` calls
