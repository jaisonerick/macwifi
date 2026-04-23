# Changelog

## [1.0.1](https://github.com/jaisonerick/macwifi/compare/v1.0.0...v1.0.1) (2026-04-22)


### Documentation

* pain-first README hook + Jekyll site for GitHub Pages ([#27](https://github.com/jaisonerick/macwifi/issues/27)) ([9bc1b12](https://github.com/jaisonerick/macwifi/commit/9bc1b121f44b3414cd3feacc56017f18a59ca5e5))
* point macwifi-cli readers at brew install ([#29](https://github.com/jaisonerick/macwifi/issues/29)) ([b1646ad](https://github.com/jaisonerick/macwifi/commit/b1646adfdadba7d07b0e1cc06dc179178e292c6e))

## [1.0.0](https://github.com/jaisonerick/macwifi/compare/v0.1.4...v1.0.0) (2026-04-22)


### Bug Fixes

* drop Swift 6.1 trailing commas for Swift 5.9/5.10 compat ([0f10718](https://github.com/jaisonerick/macwifi/commit/0f10718f9bcfda315ba4b1ebbc6e0c6f37e4d124))
* retract v0.1.1 and v0.1.2 ([498cd94](https://github.com/jaisonerick/macwifi/commit/498cd948326769b3ee4d3ccd7a3b5871d1dbe8d9))


### Documentation

* declare v1 API stability and refresh release docs ([6e9ce78](https://github.com/jaisonerick/macwifi/commit/6e9ce78c5c150b8a9ace5485d0b9cf0e30fc0b23))

## [0.1.4](https://github.com/jaisonerick/macwifi/compare/v0.1.3...v0.1.4) (2026-04-22)


### Bug Fixes

* retract v0.1.0 ([#23](https://github.com/jaisonerick/macwifi/issues/23)) ([b60b4fb](https://github.com/jaisonerick/macwifi/commit/b60b4fb30a2a4c16e9d829ab417190a7dba32161))

## [0.1.3](https://github.com/jaisonerick/macwifi/compare/v0.1.2...v0.1.3) (2026-04-22)


### Bug Fixes

* pin helper Swift target to arm64-apple-macos13.0 ([#18](https://github.com/jaisonerick/macwifi/issues/18)) ([47160c4](https://github.com/jaisonerick/macwifi/commit/47160c428cd9397e1d21ff6b4c4d4e5d5a81fd94))

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
