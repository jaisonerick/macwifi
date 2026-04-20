# macwifi

macOS WiFi scanning from Go, with real (unredacted) SSIDs, BSSIDs, channel
info, security mode, and saved-network passwords.

macOS 15+ redacts SSIDs from every CLI tool that doesn't have Location
Services permission, and only app bundles with a specific launch context
can hold that permission. This library wraps a small signed Swift helper
app (`WifiScanner.app`) that does; the Go side handles IPC and parsing.

## Usage

```go
import (
    "context"
    "fmt"
    "github.com/jaisonerick/macwifi"
)

func main() {
    appPath, _ := macwifi.DefaultAppPath()
    s := &macwifi.Scanner{AppPath: appPath}

    nets, err := s.Scan(context.Background())
    if err != nil { panic(err) }

    for _, n := range nets {
        fmt.Printf("%-25s %ddBm %s ch=%d/%dMHz %s\n",
            n.SSID, n.RSSI, n.Security, n.Channel, n.ChannelWidth, n.BSSID)
    }

    // Password lookup is a separate call (scan is silent; password prompts
    // are handled by /usr/bin/security, which Apple pre-approved).
    pw, _ := macwifi.Password("MyHomeWiFi")
    fmt.Println("password:", pw)
}
```

Each `Network` exposes:

```go
SSID, BSSID, PHYMode string
RSSI, Noise, Channel int
ChannelBand          Band      // 2.4GHz / 5GHz / 6GHz / unknown
ChannelWidth         int       // MHz
Security             Security  // WPA2 / WPA3 / Open / Enterprise / ...
Password             string    // always "" from Scan; use macwifi.Password
Current, Saved       bool
```

## Build & install

```sh
make install   # builds scanner + copies WifiScanner.app → ~/.local/share/macwifi/
```

Then in any Go program, `macwifi.DefaultAppPath()` will find it.

### Signing identity

`scanner/build.sh` auto-selects the first of **Apple Development → Developer
ID Application → Apple Distribution** from your login keychain. Override
via `SIGN_IDENTITY='Apple Development: Your Name (TEAMID)'`.

The TCC Location Services grant is tied to this identity — the grant
persists across rebuilds as long as you keep signing with the same cert.

### First-run permission prompt

The first time the library launches `WifiScanner.app`, macOS will prompt
for Location Services access. Accept it, or flip the toggle manually in
**System Settings → Privacy & Security → Location Services → macwifi
WiFi Scanner**. Subsequent scans are silent.

## Architecture

```
┌────────────┐   open -W --env MACWIFI_PORT   ┌─────────────────────┐
│ Go library │ ───────────────────────────►  │ WifiScanner.app     │
│            │                                │ (LaunchServices:    │
│            │ ◄─────── 127.0.0.1:PORT ────── │  CoreWLAN + loc)    │
│            │    binary response (MWIF…)    └─────────────────────┘
└────────────┘
      │
      │  /usr/bin/security find-generic-password   (on demand)
      ▼
  Keychain
```

The scanner app is launched via `open -W` so it runs as a foreground
LaunchServices app (required for Location Services to hand back real
SSIDs). It connects back to a Go-listened ephemeral TCP port, writes a
length-prefixed binary message, and exits.

Password lookup is **not** done from the scanner, because our custom
code signature isn't in the System keychain's pre-approved ACL for WiFi
passwords — each lookup would trigger an "allow access" prompt. Apple's
`/usr/bin/security` binary IS on that list, so Go shells out to it
instead; the lookup is silent.

### Binary protocol

```
header  = "MWIF" | u8 version=1 | u8 msgType=0x01 | u16 errLen | errLen·utf8 | u32 count
network = u16 ssidLen | ssid | u8 bssidLen | bssid |
          i16 rssi | i16 noise | u16 channel | u8 band | u16 widthMHz |
          u8 security | u8 phyLen | phy | u16 pwdLen | pwd | u8 flags
```

All multi-byte integers little-endian. See `protocol.go` (decoder) and
`scanner/Sources/main.swift` (encoder). EOF terminates the stream.

## Requirements

- macOS 13+
- Go 1.22+
- Xcode Command Line Tools (`swiftc`, `codesign`)
- A codesigning identity in your login keychain

## License

MIT.
