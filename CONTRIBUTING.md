# Contributing to macwifi

Thanks for helping improve `macwifi`. This project is focused on making macOS
WiFi discovery practical from Go while respecting macOS privacy controls.

## Good Contributions

The most useful contributions are:

- Compatibility reports across macOS versions and Apple Silicon hardware.
- Fixes for incorrect WiFi metadata mapping.
- Better examples for diagnostics, inventory, support, security, or TUI use
  cases.
- Clear bug reports with reproduction steps.
- Documentation improvements that make first-time usage easier.

Please do not submit changes that try to bypass Location Services, Keychain
consent, or other macOS user-approval flows.

## Before Opening An Issue

Check whether the behavior is already documented in the README, especially:

- The first scan may require Location Services approval.
- Password reads may trigger a Keychain prompt every time.
- Saved networks that are not currently visible may have zero signal/channel
  fields.
- The embedded helper currently targets Apple Silicon.

If the issue still looks valid, include:

- macOS version.
- Mac model and architecture.
- Go version.
- The command or code snippet you ran.
- Expected behavior.
- Actual behavior and full error output.

Do not include real WiFi passwords, private SSIDs, internal BSSIDs, or other
sensitive network details unless you have intentionally anonymized them.

## Development Setup

Run the Go tests:

```sh
make ci-go
```

Run the scanner example:

```sh
go run ./examples/scan
go run ./examples/scan --password MyHomeWiFi
```

If you are editing the Swift helper:

```sh
make scanner
MACWIFI_APP=$PWD/WifiScanner.app go run ./examples/scan
```

`MACWIFI_APP` points the Go package at your local helper instead of the embedded
release helper.

## Pull Requests

Before opening a PR:

- Keep the change focused.
- Run `make ci-go`.
- Update docs or examples when behavior changes.
- Mention whether you tested the Location Services prompt, Keychain prompt, or
  both.
- If you changed `scanner/Sources/main.swift`, test with `make scanner` and
  `MACWIFI_APP=$PWD/WifiScanner.app go run ./examples/scan`.

Changes to the embedded helper need maintainer approval. Pushes to `main` that
touch the helper run the signed companion workflow, which signs, notarizes,
staples, and opens a generated PR for `embedded/WifiScanner.app`.

## Release Notes

If your change affects users, include a short note in the PR description:

- What changed.
- Who is affected.
- Whether permissions, prompts, or supported macOS versions changed.
