---
title: How it works
layout: default
nav_order: 3
description: "Architecture of macwifi: the embedded signed helper, the macOS Location Services and Keychain dance, and the wire protocol between Go and Swift."
permalink: /how-it-works
---

# How macwifi works
{: .no_toc }

A short tour of the architecture, in case you want to know what your
binary is actually doing on the user's machine before you ship it.

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## The constraint

macOS protects Wi-Fi metadata (specifically BSSIDs and the contents
of nearby-network scans) behind the Location Services privilege.
That privilege is governed by **TCC** (Transparency, Consent, and
Control), the subsystem that draws the *"Foo wants to use your
Location"* dialog and stores the answer in `~/Library/Application
Support/com.apple.TCC`.

TCC identifies the requesting program by **code signature**. From
[Apple's DTS team][apple-dts]:

> For TCC to work reliably the calling code must be signed with a
> code signing identity that TCC can use to track the code from build
> to build.
>
> Not unsigned. No ad hoc signed.

In practice, that means a Go program — which is built locally,
unsigned by default, and changes hash on every build — cannot get the
Location Services privilege. CoreWLAN's `scanForNetworks` happily
returns to your unsigned binary, but every BSSID is empty and many
fields are zero. `wdutil info` does the same: it returns
`BSSID : <redacted>`. There is no flag, no entitlement, no plist key
that turns this off from a script.

[apple-dts]: https://developer.apple.com/forums/thread/718331

## The workaround

The only thing TCC understands is a **signed app bundle with a stable
identifier**. So `macwifi` ships one.

```
your-binary  (unsigned Go program)
    │
    │ 1. extract embedded WifiScanner.app to /tmp
    │ 2. exec `open -W` on it, with MACWIFI_PORT=NNNN env var
    ▼
WifiScanner.app  (Developer-ID-signed, notarized)
    │
    │ 3. dial back into your Go process on 127.0.0.1:NNNN
    │ 4. run CoreWLAN / Keychain calls, return responses over TCP
    ▼
your-binary  (decodes, returns []Network or password string)
```

The helper has its own bundle ID and a stable Developer ID
signature, so TCC can track it across runs. The first time it asks
for Location Services, macOS shows the standard prompt; the user
approves, and from then on TCC silently allows the request based on
the signature alone.

## The components

### `WifiScanner.app` (Swift)

A minimal Swift app that:

- Sets `LSUIElement` so it doesn't appear in the Dock or the
  ⌘-Tab switcher.
- Reads `MACWIFI_PORT` and `MACWIFI_PARENT_PID` from its environment.
- Opens a TCP connection back to `127.0.0.1:MACWIFI_PORT` to receive
  requests from the parent Go process.
- Watches `MACWIFI_PARENT_PID` via `kqueue` and exits the moment the
  parent dies — so a helper stuck in `SecItemCopyMatching` waiting
  for the user to answer the Keychain dialog can't outlive the
  caller.
- Calls `CWWiFiClient` for scans and `CWKeychainFindWiFiPassword` for
  saved Wi-Fi passwords.

The bundle is signed with a Developer ID Application certificate and
notarized via Apple's notary service. The signing happens
automatically in CI; see the
[`signed-companion.yml`](https://github.com/jaisonerick/macwifi/blob/main/.github/workflows/signed-companion.yml)
workflow.

### `embedded/WifiScanner.app` (in the Go package)

The signed-and-notarized bundle is checked into the repo at
`embedded/WifiScanner.app` and pulled into the Go binary via
`go:embed`. On first use, the Go side extracts it to
`os.TempDir()/macwifi-<version>/` and `exec`s `open -W` on it.
Subsequent runs reuse the extracted bundle, so the cost is paid once.

Developers iterating on the Swift side can override the embedded
bundle with `MACWIFI_APP=$PWD/WifiScanner.app` to point at a locally
built helper without rebuilding the embed.

### The wire protocol (`protocol.go`)

A small length-prefixed binary protocol between the Go side and the
helper. Three message types:

| Type | Direction | Purpose                                |
| ---- | --------- | -------------------------------------- |
| `0x01` scan request     | Go → helper | "Run a scan, send back results."      |
| `0x02` scan response    | helper → Go | Encoded `[]Network` slice.            |
| `0x03` password request | Go → helper | "Look up Keychain password for SSID." |
| `0x04` password response| helper → Go | Password bytes (or empty).            |
| `0x05` close request    | Go → helper | "Clean shutdown."                     |

There is no JSON, no protobuf, no TLS. The protocol speaks across
loopback, the helper exits when the parent dies, and the connection
is short-lived. If the protocol grows, it'll grow in
[`protocol.go`](https://github.com/jaisonerick/macwifi/blob/main/protocol.go).

## Lifecycle of one scan

1. Your Go code calls `macwifi.Scan(ctx)`.
2. The package extracts `WifiScanner.app` (cached) and binds a TCP
   listener on `127.0.0.1:0`.
3. It runs `open -W /tmp/macwifi-<v>/WifiScanner.app
   --env MACWIFI_PORT=NNNN --env MACWIFI_PARENT_PID=...`.
4. macOS launches the helper. If this is the first run on this
   machine, the user sees a *"WifiScanner wants to use your
   Location"* dialog; nothing else happens until they answer.
5. The helper dials back on `127.0.0.1:NNNN`. The Go side
   `Accept`s and now has a live TCP connection to the helper.
6. Go writes a scan-request frame; the helper calls
   `CWWiFiClient.shared().interface()?.scanForNetworks(...)`,
   serializes the result, and writes a scan-response frame back.
7. Go decodes the frame into `[]macwifi.Network` and returns.
8. On `Close`, Go writes a close-request, the helper exits, and the
   socket is torn down.

If you keep a `*macwifi.Client` open across many calls, steps 1–5
only happen once.

## Why not a CGo bridge?

Skipping CGo and using a separate signed helper is what makes
`macwifi` work at all. CoreWLAN linked into your Go binary via CGo
inherits the Go binary's (lack of) signature. TCC denies it the
Location privilege; you get back redacted, empty data — exactly the
state every existing Go library is in today.

A separately-signed helper is the only path that satisfies macOS
TCC for an unsigned caller. The helper is small (~200 KB compressed)
and the per-call latency is dominated by the user's network and the
Wi-Fi card's scan time, not by the IPC. The trade-off has been worth
it in practice.

## Why a Swift app instead of a tiny Objective-C helper?

Swift is what Apple's CoreWLAN samples target now, the type-checking
caught a couple of Mach-O ABI issues during development, and the
build is just `swiftc` + `codesign` + `notarytool` — nothing exotic.
The helper itself is around 200 lines.

## What you give up

- **No `launchd` daemons.** CoreWLAN's Location Services check is
  per-user-session, so a system-wide daemon won't work even with
  the signed helper.
- **No bypassing the Location Services prompt.** The first scan
  always pops the dialog. This is intentional; Apple has explicitly
  designed against allowing unattended access.
- **No bypassing the Keychain prompt.** Same applies; the
  *Always Allow* button is gone in current macOS, so password lookups
  prompt every time.

These aren't bugs in `macwifi` — they're the user's privacy controls
working as designed.

## Where to read the source

- [`macwifi.go`](https://github.com/jaisonerick/macwifi/blob/main/macwifi.go) — the Go API surface (`Client`, `Scan`, `Password`, `New/Close`).
- [`embed.go`](https://github.com/jaisonerick/macwifi/blob/main/embed.go) — extracts the embedded bundle to `os.TempDir`.
- [`protocol.go`](https://github.com/jaisonerick/macwifi/blob/main/protocol.go) — the wire format.
- [`scanner/Sources/`](https://github.com/jaisonerick/macwifi/tree/main/scanner/Sources) — the Swift helper.
- [`.github/workflows/signed-companion.yml`](https://github.com/jaisonerick/macwifi/blob/main/.github/workflows/signed-companion.yml) — the signing + notarization workflow that re-bakes the embedded bundle whenever the Swift side changes.
