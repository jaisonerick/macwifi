# macwifi

macOS WiFi scanning from Go, with real (unredacted) SSIDs, BSSIDs, channel
info, security modes, and saved-network passwords.

On macOS 15+, only apps granted Location Services can see live SSIDs. This
library ships a small signed Swift helper (`WifiScanner.app`) that handles
that side; the Go library handles everything else.

## Usage

Session-based (recommended when you'll do more than one op — one helper
launch serves all requests):

```go
import (
    "context"
    "fmt"
    "github.com/jaisonerick/macwifi"
)

func main() {
    ctx := context.Background()
    c, err := macwifi.New(ctx)
    if err != nil { panic(err) }
    defer c.Close()

    nets, _ := c.Scan(ctx)
    for _, n := range nets {
        fmt.Printf("%-25s %ddBm %s ch=%d/%dMHz %s\n",
            n.SSID, n.RSSI, n.Security, n.Channel, n.ChannelWidth, n.BSSID)
    }

    pw, _ := c.Password(ctx, "MyHomeWiFi",
        macwifi.OnKeychainAccess(func(ssid string) {
            fmt.Printf("macOS may prompt for access to %q\n", ssid)
        }))
    fmt.Println("password:", pw)
}
```

One-shot helpers (`macwifi.Scan(ctx)` / `macwifi.Password(ctx, ssid, opts...)`)
are also available for callers that only need a single operation.

### `Network` fields

```go
SSID, BSSID, PHYMode string
RSSI, Noise, Channel int
ChannelBand          Band      // 2.4GHz / 5GHz / 6GHz / unknown
ChannelWidth         int       // MHz
Security             Security  // WPA2 / WPA3 / Open / Enterprise / OWE …
Password             string    // always "" from Scan (see macwifi.Password)
Current, Saved       bool
```

## Installation

```sh
go get github.com/jaisonerick/macwifi
```

That's it. The signed + notarized helper app is embedded in the Go module
via `go:embed`. On first call, the library extracts it to
`~/Library/Caches/macwifi/<version>/WifiScanner.app` and launches it with
`open -W`. No separate install step, no build dependencies for consumers.

For dev iteration against an unreleased Swift change, set `$MACWIFI_APP`
to a local build and the library will use that instead of the embedded
copy.

### First-run Location Services prompt

The first time a consumer calls `Scan()`, macOS shows a permission dialog
for "macwifi WiFi Scanner". Clicking Allow persists the TCC grant until
the binary's signing identity changes — which, for releases signed by the
same Developer ID, means "forever across library updates."

## Contributing / building the helper locally

If you're editing `scanner/Sources/main.swift`, you build and sign with
your own cert:

```sh
make scanner               # produces ./WifiScanner.app (debug-signed)
MACWIFI_APP=$PWD/WifiScanner.app go run ./examples/scan
```

To cut a release with an updated embedded bundle:

```sh
make release               # build → sign (Developer ID) → notarize → staple
                           # → copy to embedded/WifiScanner.app
# bump embeddedVersion in embed.go
# commit + tag + push
```

`make release` requires:

- A **Developer ID Application** cert in your login keychain.
- A `xcrun notarytool` credential profile named `macwifi-notary`
  (override via `$NOTARY_PROFILE`). Set it up once:

  ```sh
  xcrun notarytool store-credentials macwifi-notary \
      --apple-id YOUR_APPLE_ID \
      --team-id YOUR_TEAM_ID \
      --password YOUR_APP_SPECIFIC_PASSWORD
  ```

  (Generate an app-specific password at
  <https://appleid.apple.com/account/manage> under "Sign-In and Security →
  App-Specific Passwords".)

### Keychain passwords

WiFi passwords live in `/Library/Keychains/System.keychain` with an ACL
that only whitelists Apple's WiFi daemons (`airportd`, the AirPort app
group). Anything else — our `/usr/bin/security` shell-out, a custom signed
app, anything — triggers macOS's consent dialog.

On recent macOS versions (13+), that dialog only offers **Allow** (one-
time) and **Deny** for these items. The legacy "Always Allow" button has
been removed for third-party CLI access; clicking Allow does **not**
persist a grant. **You'll see the dialog every time** you invoke
`macwifi.Password()` for an SSID.

Workarounds:

- **Use `OnKeychainAccess`** to display a heads-up before the dialog, so
  the user isn't surprised.
- **Let the user type it manually** — cancel the dialog, prompt instead.
- **Pre-approve via Keychain Access.app** — the GUI *does* still let you
  edit the ACL directly: open the app, browse to System → the WiFi item
  → Access Control tab → add `/usr/bin/security` to the allowed-apps list.
  This is a one-time setup per SSID, and the grant persists.

There is no programmatic bypass for unprivileged library code; it's a
core part of the macOS security model. Privileged helpers (SMJobBless)
that run as root can read System keychain items without ACL prompts, but
installing one requires an admin password and is overkill for most use
cases.

## Architecture

```
┌────────────┐  open -W --env MACWIFI_PORT  ┌─────────────────────┐
│ Go Client  │ ───────────────────────────► │ WifiScanner.app     │
│ (New)      │                              │ (LaunchServices:    │
│            │ ◄── 127.0.0.1:PORT ───────── │  CoreWLAN + Security) │
│            │                              │                     │
│  Scan()   →│ scan_request ─────────────►  │  runScan()          │
│            │ ◄── scan_response ────────── │                     │
│  Password →│ password_request ─────────►  │  SecItemCopyMatching│
│            │ ◄── password_response ────── │  (macOS dialog)     │
│  Close()  →│ close_request ────────────►  │  exit               │
└────────────┘                              └─────────────────────┘
```

One helper process per Client session, serving a loop of requests until
`Close()` is called. Launched via `open -W` so macOS classifies it as a
foreground LaunchServices app (required for unredacted CoreWLAN results).
Communication is over a loopback TCP socket using a fixed-layout binary
protocol — see `protocol.go` (Go side) and `scanner/Sources/main.swift`
(Swift side) for the wire format.
