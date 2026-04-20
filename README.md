# macwifi

macOS WiFi scanning from Go, with real (unredacted) SSIDs, BSSIDs, channel
info, security modes, and saved-network passwords.

On macOS 15+, only apps granted Location Services can see live SSIDs. This
library ships a small signed Swift helper (`WifiScanner.app`) that handles
that side; the Go library handles everything else.

## Usage

```go
import (
    "context"
    "fmt"
    "github.com/jaisonerick/macwifi"
)

func main() {
    s := &macwifi.Scanner{}
    nets, err := s.Scan(context.Background())
    if err != nil { panic(err) }

    for _, n := range nets {
        fmt.Printf("%-25s %ddBm %s ch=%d/%dMHz %s\n",
            n.SSID, n.RSSI, n.Security, n.Channel, n.ChannelWidth, n.BSSID)
    }

    // Saved password lookup. macOS shows a per-SSID "Allow access" dialog
    // the first time; OnKeychainAccess lets you warn the user before it
    // appears.
    pw, _ := macwifi.Password("MyHomeWiFi",
        macwifi.OnKeychainAccess(func(ssid string) {
            fmt.Printf("macOS may prompt for access to the %q password — choose 'Always Allow' to skip this next time.\n", ssid)
        }))
    fmt.Println("password:", pw)
}
```

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
┌────────────┐    open -W --env MACWIFI_PORT    ┌─────────────────────┐
│ Go library │ ──────────────────────────────►  │ WifiScanner.app     │
│            │                                   │ (LaunchServices:    │
│            │ ◄─────── 127.0.0.1:PORT ───────── │  CoreWLAN + loc)    │
│            │       binary response (MWIF…)    └─────────────────────┘
└────────────┘
      │
      │  /usr/bin/security find-generic-password   (lazy, per call)
      ▼
  System keychain
```

Scanner launched via `open -W` so macOS gives it foreground-app status
(required for Location Services to return real SSIDs). It calls back to
a Go-listened ephemeral TCP port, writes a length-prefixed binary message
— see `protocol.go` (decoder) and `scanner/Sources/main.swift` (encoder)
— and exits.

Password lookup runs only when `macwifi.Password(ssid)` is called.
