---
title: Getting started
layout: default
nav_order: 2
description: "Install macwifi, handle the Location Services prompt, scan networks, and read saved Wi-Fi passwords from the Keychain on macOS 13+."
permalink: /getting-started
---

# Getting started with macwifi
{: .no_toc }

This guide walks through installing `macwifi`, handling the macOS
Location Services prompt the first time you run a scan, and reading
saved Wi-Fi passwords from the Keychain. It also covers the things
that surprise people coming from `airport -s` or
`schollz/wifiscan`.

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Background

If you maintain a Go program that touches Wi-Fi on macOS, the last
two years have not been kind:

- **macOS 14.4 removed `/usr/libexec/airport`.** The CLI tool that
  every script and Go library used to enumerate nearby networks
  silently went away. Apple's note: *"The airport command line tool
  is deprecated and will be removed in a future release."*
- **`wdutil info` redacts `BSSID`** even when run with `sudo`.
- **`networksetup` and `ioreg` don't expose nearby networks.**
- **CoreWLAN's `scanForNetworks`** only returns real BSSIDs to apps
  that are signed with a stable Developer ID *and* have Location
  Services permission. Apple's DTS team has confirmed [on the
  developer forums][apple-dts] that scripts and `launchd` daemons
  can't satisfy these requirements.

Every Go library on pkg.go.dev that does Wi-Fi scanning on macOS
parses `airport` output, so all of them silently broke when Sonoma
14.4 shipped. `macwifi` exists because there was no remaining path
that worked from a Go program.

[apple-dts]: https://developer.apple.com/forums/thread/718331

## Install

```sh
go get github.com/jaisonerick/macwifi
```

Requirements:

- macOS 13 or newer on Apple Silicon.
- Go 1.26 or newer.
- The first `Scan()` or `Password()` call will trigger the macOS
  Location Services prompt. There is no API to suppress this — it's
  the entire reason the library can return real BSSIDs.

The Developer-ID-signed and notarized helper bundle is embedded in
the package via `go:embed` and extracted to a temporary directory on
first use. Your end users do not need to install anything separately.

## First scan

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/jaisonerick/macwifi"
)

func main() {
    nets, err := macwifi.Scan(context.Background())
    if err != nil {
        fmt.Fprintln(os.Stderr, "scan:", err)
        os.Exit(1)
    }
    for _, n := range nets {
        fmt.Printf("%-32s  %s  %3d dBm  ch %d  %s\n",
            n.SSID, n.BSSID, n.RSSI, n.Channel, n.Security)
    }
}
```

Expected output (after the user approves the Location Services
prompt the first time):

```
Office WiFi                       aa:bb:cc:dd:ee:ff   -52 dBm  ch 149  WPA2
Guest                             11:22:33:44:55:66   -71 dBm  ch  36  WPA2
Conference Room                   77:88:99:aa:bb:cc   -58 dBm  ch 100  WPA3
```

The `Network` struct returned from `Scan` carries everything CoreWLAN
exposes that a Go developer is likely to want:

| Field          | Description                                             |
| -------------- | ------------------------------------------------------- |
| `SSID`         | Wi-Fi network name.                                     |
| `BSSID`        | Access point MAC address. Empty until Location Services is approved. |
| `RSSI`         | Signal strength in dBm. Closer to zero is stronger.     |
| `Noise`        | Noise floor in dBm, when macOS reports it.              |
| `Channel`      | Wi-Fi channel number.                                   |
| `ChannelBand`  | `2.4GHz`, `5GHz`, `6GHz`, or `unknown`.                 |
| `ChannelWidth` | Channel bandwidth in MHz.                               |
| `Security`     | Open, WEP, WPA, WPA2, WPA3, enterprise, OWE, or unknown. |
| `PHYMode`      | 802.11 mode when available.                             |
| `Current`      | Whether the Mac is connected to this network now.       |
| `Saved`        | Whether the SSID is in the preferred-networks list.     |
| `Password`     | Always `""` from `Scan`; use `Password(ctx, ssid)`.     |

Saved networks that are not currently visible may be included with
the signal and channel fields set to zero.

## Reusing the helper across calls

`macwifi.Scan` and `macwifi.Password` are one-shot helpers that spawn
the embedded app, run a single request, and tear it down. If you
plan to do more than one operation in the same run — for example,
scan and then look up a password for the strongest network — keep a
client open:

```go
ctx := context.Background()

c, err := macwifi.New(ctx)
if err != nil {
    panic(err)
}
defer c.Close()

nets, err := c.Scan(ctx)
if err != nil {
    panic(err)
}

password, err := c.Password(ctx, "MyHomeWiFi",
    macwifi.OnKeychainAccess(func(ssid string) {
        fmt.Printf("→ approve the Keychain prompt for %q\n", ssid)
    }),
)
if err != nil {
    panic(err)
}

fmt.Println(len(nets), "networks;", "password length:", len(password))
```

Reusing the client matters because each fresh `New` call launches the
helper bundle, which goes through `open -W`, the Location Services
permission check, and a tiny TCP handshake to your Go process.
That's roughly half a second of latency per call.

## Reading saved Wi-Fi passwords

Saved Wi-Fi passwords live in the macOS System keychain and can be
retrieved via `CWKeychainFindWiFiPassword` from CoreWLAN. The first
time `macwifi.Password()` runs for an SSID, macOS will display its
own *Allow* / *Deny* dialog asking permission to read that Keychain
entry.

A few things to know before you ship this:

- The legacy **Always Allow** path is no longer available in the
  Keychain dialog, so the prompt fires *every* time you call
  `Password()` for a given SSID. Plan your UX accordingly.
- Use the `OnKeychainAccess` option to give the user a heads-up
  before macOS shows its dialog:

```go
password, err := macwifi.Password(ctx, ssid,
    macwifi.OnKeychainAccess(func(ssid string) {
        fmt.Printf("Approve the macOS Keychain prompt to read %q\n", ssid)
    }),
)
```

- A return of `("", nil)` means there is no saved entry for that SSID.
- The helper bounds each `Password` call at 60 seconds by default
  (generous, to cover the user reading the dialog). Use
  `macwifi.WithTimeout(d)` to adjust.

## Troubleshooting

### "I get empty BSSIDs back"

The user has not yet approved the Location Services prompt. Open
**System Settings → Privacy & Security → Location Services** and
confirm `WifiScanner` (or whatever bundle name your binary uses) is
enabled. The next scan will return real BSSIDs.

### "I'm running this from a `launchd` daemon and it doesn't work"

This is a hard limit imposed by macOS. From [Apple's DTS
team][apple-dts]:

> Via a `launchd` daemon — That's unlikely to work. CoreWLAN checks
> for the Location privilege and that's hard for a daemon to get.

The helper bundle uses `LSUIElement` so it doesn't appear in the
Dock, but it still runs as a foreground GUI process. That works from
a normal user-session program, an SSH session into the user account,
or a `launchd` *agent*. It does not work from a system-wide
`launchd` daemon.

### "Some saved networks have empty signal/channel fields"

That's expected. macOS includes saved-but-not-visible networks in
the result with `Saved: true`, `Current: false`, and the
signal/channel fields zeroed. Filter on `n.RSSI != 0 || n.Current`
if you only want networks that are reachable now.

### "How do I rebuild the embedded helper for development?"

If you're iterating on the Swift side, point `MACWIFI_APP` at a
locally-built bundle to skip the embedded one:

```sh
make scanner
MACWIFI_APP="$PWD/WifiScanner.app" go run ./examples/scan
```

## Where to next

- [How it works]({{ site.baseurl }}/how-it-works) — the embedded
  helper, the wire protocol, and why this approach exists at all.
- [GoDoc](https://pkg.go.dev/github.com/jaisonerick/macwifi) — full
  type and function reference.
- [`macwifi-cli`](https://github.com/jaisonerick/macwifi-cli) — a
  drop-in `airport`-replacement built on this package, if you'd
  rather not write Go.
