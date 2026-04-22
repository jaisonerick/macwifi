---
title: Home
layout: default
nav_order: 1
description: "Wi-Fi scanning and Keychain password access for Go programs on macOS 13+, after the airport CLI was removed and wdutil started redacting BSSIDs."
permalink: /
---

# macwifi
{: .fs-9 }

Wi-Fi scanning and Keychain password access for Go programs on macOS 13+.
{: .fs-6 .fw-300 }

[Get started]({{ site.baseurl }}/getting-started){: .btn .btn-primary .fs-5 .mb-4 .mb-md-0 .mr-2 }
[View on GitHub](https://github.com/jaisonerick/macwifi){: .btn .fs-5 .mb-4 .mb-md-0 }

---

## The problem

macOS 14.4 removed `/usr/libexec/airport`, the CLI tool that backed
almost every script and library that wanted to enumerate nearby
Wi-Fi networks or get the BSSID, RSSI, and channel of the current
connection.

Its replacement, `wdutil info`, requires `sudo` and returns
`BSSID : <redacted>` for every entry. `networksetup` and `ioreg`
don't expose nearby networks at all.

The Apple-recommended path is the CoreWLAN framework, but
`scanForNetworks` only returns real BSSIDs to apps that are
**signed with a stable Developer ID** *and* **have been granted
Location Services permission**. Apple's Developer Technical Support
team has confirmed [on the developer forums][apple-dts] that scripts
and `launchd` daemons can't satisfy these requirements:

> I recommend that you try this with native code built in to a native
> app. TCC, the subsystem within macOS that manages the privileges
> visible in System Preferences > Security & Privacy > Privacy,
> doesn't work well for scripts.

The Go libraries that previously did this — `schollz/wifiscan`,
`jarethdisley/wifiscanparser`, and several others — parse `airport`
output. None of them work on Sonoma 14.4 or later.

## What macwifi does

`macwifi` is a Go package that embeds a Developer-ID-signed,
notarized Swift helper bundle. Your Go binary spawns the helper on
first use to trigger the macOS Location Services prompt; subsequent
calls reuse the same helper process.

```go
package main

import (
    "context"
    "fmt"

    "github.com/jaisonerick/macwifi"
)

func main() {
    nets, err := macwifi.Scan(context.Background())
    if err != nil {
        panic(err)
    }
    for _, n := range nets {
        fmt.Printf("%-32s %s  %d dBm  ch %d\n",
            n.SSID, n.BSSID, n.RSSI, n.Channel)
    }
}
```

The package returns rich metadata — SSID, BSSID, RSSI, noise floor,
channel number, channel band (2.4/5/6 GHz), channel width, security
mode, PHY mode, and current/saved flags. There's also
`macwifi.Password(ctx, ssid)` for reading saved Wi-Fi passwords from
the Keychain after the user approves the system prompt.

## Install

```sh
go get github.com/jaisonerick/macwifi
```

Requirements: macOS 13 or newer on Apple Silicon, Go 1.26+.

## Just want a CLI?

If you don't need a Go API and just want a working `airport -s`
replacement on the terminal, install
[`macwifi-cli`](https://github.com/jaisonerick/macwifi-cli):

```sh
go install github.com/jaisonerick/macwifi-cli@latest

macwifi-cli scan
macwifi-cli info
macwifi-cli password "MyHomeWiFi"
```

## What's next

- [Getting started]({{ site.baseurl }}/getting-started) — full setup,
  Location Services dance, troubleshooting, and a longer worked
  example.
- [How it works]({{ site.baseurl }}/how-it-works) — what the embedded
  helper does, why it exists, and the protocol it speaks back to Go.

## Scope

`macwifi` is a macOS-only Go library. It is not a cross-platform
abstraction, a packet capture library, a background daemon, or a
workaround for macOS privacy controls. Location Services and Keychain
prompts still go through the user, every time.

[apple-dts]: https://developer.apple.com/forums/thread/718331
