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

## Build & install

```sh
make install   # builds scanner + copies WifiScanner.app → ~/.local/share/macwifi/
```

`Scanner.Scan()` will find the installed app automatically. For testing,
set `$MACWIFI_APP=/path/to/WifiScanner.app` to override.

### First-run Location Services prompt

The first time a Go consumer calls `Scan()`, macOS may show a permission
dialog for "macwifi WiFi Scanner". Allow it once and the TCC grant persists
across rebuilds as long as your signing identity doesn't change.

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
