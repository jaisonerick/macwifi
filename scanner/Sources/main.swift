// WifiScanner.app — signed CoreWLAN scanner for the macwifi Go library.
//
// Runs under LaunchServices (invoked via `open -W`) so macOS grants it
// foreground-app status, unlocking unredacted SSIDs. Connects back to the
// Go listener on 127.0.0.1:$MACWIFI_PORT and writes a binary protocol
// message; see protocol.go in the macwifi Go package for the wire layout.
//
// The app has two modes, selected by environment variable:
//
//   MACWIFI_MODE=scan      (default) — CoreWLAN scan, needs Location Services
//   MACWIFI_MODE=password  — read one saved WiFi password from System keychain
//                           (triggers macOS's Keychain "Allow access" dialog)
//
// In both modes, the result is written to a loopback TCP connection
// established against MACWIFI_PORT in the calling process (see protocol.go
// in the Go library for the binary wire format).
import CoreLocation
import CoreWLAN
import Darwin
import Foundation
import Security

// ─────────────────────────── location authorization ────────────────────────

final class LocationDelegate: NSObject, CLLocationManagerDelegate {
    var changed = false
    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        changed = true
    }
}

func ensureAuthorized() throws {
    let manager = CLLocationManager()
    let delegate = LocationDelegate()
    manager.delegate = delegate

    var status = manager.authorizationStatus
    if status == .notDetermined {
        manager.requestAlwaysAuthorization()
        let deadline = Date().addingTimeInterval(60)
        while manager.authorizationStatus == .notDetermined, Date() < deadline {
            RunLoop.current.run(until: Date().addingTimeInterval(0.25))
        }
        status = manager.authorizationStatus
    }

    switch status {
    case .authorizedAlways, .authorizedWhenInUse:
        return
    case .denied, .restricted:
        throw ScanError.message("Location Services denied. Grant access in System Settings → Privacy & Security → Location Services.")
    case .notDetermined:
        throw ScanError.message("Location Services authorization timed out.")
    @unknown default:
        throw ScanError.message("Unexpected Location Services status.")
    }
}

// ─────────────────────────── scan types + mapping ──────────────────────────

struct ScannedNetwork {
    var ssid: String
    var bssid: Data            // 0 or 6 bytes
    var rssi: Int16
    var noise: Int16
    var channel: UInt16
    var band: UInt8
    var channelWidth: UInt16
    var security: UInt8
    var phyMode: String
    var password: String
    var current: Bool
    var saved: Bool
}

enum ScanError: Error {
    case message(String)
}

// Map CWChannelBand (Apple enum) → our wire value.
func mapBand(_ b: CWChannelBand) -> UInt8 {
    switch b.rawValue {
    case 1: return 1  // 2.4 GHz
    case 2: return 2  // 5 GHz
    case 3: return 3  // 6 GHz
    default: return 0
    }
}

// Map CWChannelWidth → MHz.
func mapWidth(_ w: CWChannelWidth) -> UInt16 {
    switch w.rawValue {
    case 0: return 20
    case 1: return 40
    case 2: return 80
    case 3: return 160
    default: return 0
    }
}

// Detect a network's security mode. CWNetwork doesn't expose a single
// property for this on recent SDKs — instead it answers supportsSecurity for
// each CWSecurity case. Query in "most specific wins" order.
func detectSecurity(_ n: CWNetwork) -> UInt8 {
    // Ordered strongest/most-specific → weakest.
    let probes: [(CWSecurity, UInt8)] = [
        (.wpa3Enterprise,      7),
        (.wpa3Personal,        5),
        (.wpa3Transition,      5),
        (.wpa2Enterprise,      6),
        (.wpa2Personal,        4),
        (.personal,            4),   // mixed WPA/WPA2
        (.wpaEnterprise,       6),
        (.wpaEnterpriseMixed,  6),
        (.wpaPersonal,         3),
        (.wpaPersonalMixed,    3),
        (.enterprise,          6),
        (.dynamicWEP,          2),
        (.WEP,                 2),
        (.none,                1),
    ]
    for (mode, wire) in probes where n.supportsSecurity(mode) {
        return wire
    }
    return 0
}

// Map CWPHYMode → "802.11.." label.
func mapPHYMode(_ p: CWPHYMode) -> String {
    switch p.rawValue {
    case 1: return "802.11a"
    case 2: return "802.11b"
    case 3: return "802.11g"
    case 4: return "802.11n"
    case 5: return "802.11ac"
    case 6: return "802.11ax"
    default: return ""
    }
}

// Parse a BSSID string ("aa:bb:cc:dd:ee:ff") into 6 raw bytes.
func parseBSSID(_ s: String?) -> Data {
    guard let s = s else { return Data() }
    let parts = s.split(separator: ":")
    guard parts.count == 6 else { return Data() }
    var bytes = Data(capacity: 6)
    for p in parts {
        guard let b = UInt8(p, radix: 16) else { return Data() }
        bytes.append(b)
    }
    return bytes
}

// ─────────────────────────── scan orchestration ────────────────────────────

func preferredNetworkSSIDs() -> Set<String> {
    let proc = Process()
    proc.executableURL = URL(fileURLWithPath: "/usr/sbin/networksetup")
    proc.arguments = ["-listpreferredwirelessnetworks", "en0"]
    let pipe = Pipe()
    proc.standardOutput = pipe
    proc.standardError = Pipe()
    do { try proc.run() } catch { return [] }
    proc.waitUntilExit()
    guard let out = String(data: pipe.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) else { return [] }
    var set = Set<String>()
    for line in out.split(separator: "\n") {
        let s = String(line)
        if s.hasPrefix("\t") {
            set.insert(String(s.dropFirst()))
        }
    }
    return set
}

func runScan() throws -> [ScannedNetwork] {
    try ensureAuthorized()

    guard let iface = CWWiFiClient.shared().interface() else {
        throw ScanError.message("no WiFi interface available")
    }

    let scanned: Set<CWNetwork>
    do { scanned = try iface.scanForNetworks(withName: nil) }
    catch { throw ScanError.message("scan failed: \(error.localizedDescription)") }

    let saved = preferredNetworkSSIDs()
    let currentSSID = iface.ssid()

    // Deduplicate by SSID, keep the strongest-signal entry per name.
    var byName: [String: CWNetwork] = [:]
    for n in scanned {
        guard let ssid = n.ssid, !ssid.isEmpty else { continue }
        if let prev = byName[ssid], prev.rssiValue >= n.rssiValue { continue }
        byName[ssid] = n
    }

    var out: [ScannedNetwork] = []
    out.reserveCapacity(byName.count + saved.count)

    for (ssid, n) in byName {
        let ch = n.wlanChannel
        out.append(ScannedNetwork(
            ssid: ssid,
            bssid: parseBSSID(n.bssid),
            rssi: Int16(clamping: n.rssiValue),
            noise: Int16(clamping: n.noiseMeasurement),
            channel: UInt16(clamping: ch?.channelNumber ?? 0),
            band: ch.map { mapBand($0.channelBand) } ?? 0,
            channelWidth: ch.map { mapWidth($0.channelWidth) } ?? 0,
            security: detectSecurity(n),
            phyMode: "",  // CWNetwork doesn't expose PHY mode directly; interface-level only
            password: "",  // looked up in Go via /usr/bin/security
            current: currentSSID == ssid,
            saved: saved.contains(ssid)
        ))
    }

    // Add saved-but-not-scanned networks as stubs.
    for ssid in saved where byName[ssid] == nil {
        out.append(ScannedNetwork(
            ssid: ssid,
            bssid: Data(),
            rssi: 0, noise: 0, channel: 0, band: 0, channelWidth: 0,
            security: 0, phyMode: "", password: "",
            current: currentSSID == ssid, saved: true
        ))
    }

    // Sort: stronger RSSI first (0 for stubs sorts to the end).
    out.sort { ($0.rssi == 0 ? -999 : Int($0.rssi)) > ($1.rssi == 0 ? -999 : Int($1.rssi)) }
    return out
}

// ─────────────────────────── binary encoder ────────────────────────────────

struct BinaryWriter {
    private(set) var data = Data()

    mutating func putU8(_ v: UInt8)   { data.append(v) }
    mutating func putU16LE(_ v: UInt16) { var x = v.littleEndian; withUnsafeBytes(of: &x) { data.append(contentsOf: $0) } }
    mutating func putU32LE(_ v: UInt32) { var x = v.littleEndian; withUnsafeBytes(of: &x) { data.append(contentsOf: $0) } }
    mutating func putI16LE(_ v: Int16)  { putU16LE(UInt16(bitPattern: v)) }

    mutating func putMagic(_ s: String) { data.append(s.data(using: .ascii)!) }
    mutating func putString8(_ s: String) {
        let b = Array(s.utf8)
        precondition(b.count <= 0xFF)
        putU8(UInt8(b.count))
        data.append(contentsOf: b)
    }
    mutating func putString16(_ s: String) {
        let b = Array(s.utf8)
        precondition(b.count <= 0xFFFF)
        putU16LE(UInt16(b.count))
        data.append(contentsOf: b)
    }
    mutating func putBytes8(_ b: Data) {
        precondition(b.count <= 0xFF)
        putU8(UInt8(b.count))
        data.append(b)
    }
}

func encodeScanResponse(networks: [ScannedNetwork], error: String) -> Data {
    var w = BinaryWriter()
    w.putMagic("MWIF")
    w.putU8(1)           // version
    w.putU8(0x01)        // msg type = scan_response
    w.putString16(error)
    w.putU32LE(UInt32(networks.count))

    for n in networks {
        w.putString16(n.ssid)
        w.putBytes8(n.bssid)
        w.putI16LE(n.rssi)
        w.putI16LE(n.noise)
        w.putU16LE(n.channel)
        w.putU8(n.band)
        w.putU16LE(n.channelWidth)
        w.putU8(n.security)
        w.putString8(n.phyMode)
        w.putString16(n.password)
        var flags: UInt8 = 0
        if n.current { flags |= 0x01 }
        if n.saved   { flags |= 0x02 }
        w.putU8(flags)
    }
    return w.data
}

func encodePasswordResponse(password: String, error: String) -> Data {
    var w = BinaryWriter()
    w.putMagic("MWIF")
    w.putU8(1)           // version
    w.putU8(0x02)        // msg type = password_response
    w.putString16(error)
    w.putString16(password)
    return w.data
}

// ─────────────────────────── IPC / entry point ─────────────────────────────

func sendPayload(_ data: Data) {
    if let portStr = ProcessInfo.processInfo.environment["MACWIFI_PORT"],
       let port = UInt16(portStr),
       let sock = connectLoopback(port: port) {
        sock.write(data)
        try? sock.close()
    } else {
        // Fallback: write to stdout (useful for manual invocation via `open -W --stdout`).
        FileHandle.standardOutput.write(data)
    }
}

func connectLoopback(port: UInt16) -> FileHandle? {
    let fd = socket(AF_INET, SOCK_STREAM, 0)
    guard fd >= 0 else { return nil }

    var addr = sockaddr_in()
    addr.sin_family = sa_family_t(AF_INET)
    addr.sin_port = port.bigEndian
    addr.sin_addr.s_addr = inet_addr("127.0.0.1")

    let rc = withUnsafePointer(to: &addr) { p -> Int32 in
        p.withMemoryRebound(to: sockaddr.self, capacity: 1) { sa in
            Darwin.connect(fd, sa, socklen_t(MemoryLayout<sockaddr_in>.size))
        }
    }
    guard rc == 0 else { close(fd); return nil }
    return FileHandle(fileDescriptor: fd, closeOnDealloc: true)
}

// ─────────────────────────── keychain lookup ───────────────────────────────

func keychainPassword(ssid: String) -> (String, String?) {
    let query: [String: Any] = [
        kSecClass as String:         kSecClassGenericPassword,
        kSecAttrDescription as String: "AirPort network password",
        kSecAttrAccount as String:    ssid,
        kSecReturnData as String:     true,
        kSecMatchLimit as String:     kSecMatchLimitOne,
    ]
    var result: CFTypeRef?
    let status = SecItemCopyMatching(query as CFDictionary, &result)
    switch status {
    case errSecSuccess:
        if let data = result as? Data, let s = String(data: data, encoding: .utf8) {
            return (s, nil)
        }
        return ("", "keychain returned non-utf8 data")
    case errSecItemNotFound:
        return ("", nil)   // empty, no error
    case errSecUserCanceled:
        return ("", "user declined keychain access")
    default:
        return ("", "SecItemCopyMatching status \(status)")
    }
}

// Main.
let mode = ProcessInfo.processInfo.environment["MACWIFI_MODE"] ?? "scan"

switch mode {
case "scan":
    do {
        let networks = try runScan()
        sendPayload(encodeScanResponse(networks: networks, error: ""))
    } catch ScanError.message(let msg) {
        sendPayload(encodeScanResponse(networks: [], error: msg))
        exit(2)
    } catch {
        sendPayload(encodeScanResponse(networks: [], error: "\(error)"))
        exit(1)
    }

case "password":
    let ssid = ProcessInfo.processInfo.environment["MACWIFI_SSID"] ?? ""
    if ssid.isEmpty {
        sendPayload(encodePasswordResponse(password: "", error: "MACWIFI_SSID is empty"))
        exit(2)
    }
    let (pw, err) = keychainPassword(ssid: ssid)
    sendPayload(encodePasswordResponse(password: pw, error: err ?? ""))
    if err != nil { exit(2) }

default:
    sendPayload(encodeScanResponse(networks: [], error: "unknown MACWIFI_MODE=\(mode)"))
    exit(2)
}
