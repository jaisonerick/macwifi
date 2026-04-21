## Summary

Describe what changed and why.

## Testing

- [ ] `go test ./...`
- [ ] `go run ./examples/scan`
- [ ] `go run ./examples/scan --password MyHomeWiFi`
- [ ] If the Swift helper changed: `make scanner`
- [ ] If the Swift helper changed: `MACWIFI_APP=$PWD/WifiScanner.app go run ./examples/scan`

## macOS Permissions

- [ ] I tested or considered the Location Services prompt.
- [ ] I tested or considered the Keychain prompt.
- [ ] This change does not attempt to bypass macOS user consent.

## Notes

Mention supported macOS versions, Apple Silicon behavior, or release/signing
impact if relevant.
