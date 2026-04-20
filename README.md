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

WiFi passwords live in the System keychain with a restrictive ACL — only
Apple's WiFi daemons (`airportd`) have silent access by default. Third
parties (our `security` shell-out, any other tool) trigger a per-item
"Allow access" dialog from macOS the first time they read an SSID.

Picking **"Always Allow"** on that dialog adds `/usr/bin/security` to the
item's ACL — subsequent calls for that SSID are silent. You do this once
per saved network you care about, not per app run.

There is no safe way for an unprivileged library to bypass this prompt;
it's a core part of the macOS security model.

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
