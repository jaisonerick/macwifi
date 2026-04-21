// WifiScanner.app — signed CoreWLAN + Keychain helper for macwifi.
//
// Launched via `open -W` with MACWIFI_PORT and MACWIFI_PARENT_PID in the
// environment. Connects back to 127.0.0.1:MACWIFI_PORT and serves request
// frames in a loop until the Go client sends a close_request (or the
// connection drops, or the parent process dies).
//
// A DispatchSource watches MACWIFI_PARENT_PID for `.exit` events (kqueue
// EVFILT_PROC / NOTE_EXIT under the hood) and calls exit(0) the instant
// the Go process exits. This guarantees the helper never outlives its
// parent — even if the parent crashes, is SIGKILL'd, or the helper is
// stuck inside SecItemCopyMatching waiting on a keychain dialog.
//
// Protocol: see protocol.go on the Go side for the fixed-layout binary
// wire format.
import CoreLocation
import CoreWLAN
import Darwin
import Foundation
import Security

// ─────────────────────────── helpers: binary IO ────────────────────────────

struct BinaryWriter {
    private(set) var data = Data()
    mutating func putU8(_ v: UInt8)     { data.append(v) }
    mutating func putU16LE(_ v: UInt16) { var x = v.littleEndian; withUnsafeBytes(of: &x) { data.append(contentsOf: $0) } }
    mutating func putU32LE(_ v: UInt32) { var x = v.littleEndian; withUnsafeBytes(of: &x) { data.append(contentsOf: $0) } }
    mutating func putI16LE(_ v: Int16)  { putU16LE(UInt16(bitPattern: v)) }
    mutating func putMagic(_ s: String) { data.append(s.data(using: .ascii)!) }
    mutating func putString8(_ s: String) {
        let b = Array(s.utf8); precondition(b.count <= 0xFF)
        putU8(UInt8(b.count)); data.append(contentsOf: b)
    }
    mutating func putString16(_ s: String) {
        let b = Array(s.utf8); precondition(b.count <= 0xFFFF)
        putU16LE(UInt16(b.count)); data.append(contentsOf: b)
    }
    mutating func putBytes8(_ b: Data) {
        precondition(b.count <= 0xFF); putU8(UInt8(b.count)); data.append(b)
    }
}

/// Reads exactly `n` bytes or throws on EOF / IO error.
func readExact(_ fd: Int32, _ n: Int) -> Data? {
    var out = Data(count: n)
    var got = 0
    while got < n {
        let r = out.withUnsafeMutableBytes { raw -> Int in
            Darwin.read(fd, raw.baseAddress!.advanced(by: got), n - got)
        }
        if r <= 0 { return nil }
        got += r
    }
    return out
}

func readU8(_ fd: Int32) -> UInt8? {
    guard let d = readExact(fd, 1) else { return nil }
    return d[0]
}

func readU16LE(_ fd: Int32) -> UInt16? {
    guard let d = readExact(fd, 2) else { return nil }
    return UInt16(d[0]) | (UInt16(d[1]) << 8)
}

func readString16(_ fd: Int32) -> String? {
    guard let n = readU16LE(fd) else { return nil }
    if n == 0 { return "" }
    guard let d = readExact(fd, Int(n)) else { return nil }
    return String(data: d, encoding: .utf8)
}

func writeAll(_ fd: Int32, _ data: Data) -> Bool {
    var written = 0
    let total = data.count
    return data.withUnsafeBytes { raw -> Bool in
        while written < total {
            let w = Darwin.write(fd, raw.baseAddress!.advanced(by: written), total - written)
            if w <= 0 { return false }
            written += w
        }
        return true
    }
}

// ─────────────────────────── location authorization ────────────────────────

final class LocationDelegate: NSObject, CLLocationManagerDelegate {
    func locationManagerDidChangeAuthorization(_ m: CLLocationManager) {}
}

enum ScanError: Error { case message(String) }

func ensureAuthorized() throws {
    let m = CLLocationManager()
    let d = LocationDelegate()
    m.delegate = d
    var s = m.authorizationStatus
    if s == .notDetermined {
        m.requestAlwaysAuthorization()
        let deadline = Date().addingTimeInterval(60)
        while m.authorizationStatus == .notDetermined, Date() < deadline {
            RunLoop.current.run(until: Date().addingTimeInterval(0.25))
        }
        s = m.authorizationStatus
    }
    switch s {
    case .authorizedAlways, .authorizedWhenInUse: return
    case .denied, .restricted:
        throw ScanError.message("Location Services denied.")
    case .notDetermined:
        throw ScanError.message("Location Services authorization timed out.")
    @unknown default:
        throw ScanError.message("Unexpected Location Services status.")
    }
}

// ─────────────────────────── scan ──────────────────────────────────────────

struct ScannedNetwork {
    var ssid: String
    var bssid: Data
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

func mapBand(_ b: CWChannelBand) -> UInt8 {
    switch b.rawValue { case 1: return 1; case 2: return 2; case 3: return 3; default: return 0 }
}
func mapWidth(_ w: CWChannelWidth) -> UInt16 {
    switch w.rawValue { case 0: return 20; case 1: return 40; case 2: return 80; case 3: return 160; default: return 0 }
}
func detectSecurity(_ n: CWNetwork) -> UInt8 {
    let probes: [(CWSecurity, UInt8)] = [
        (.wpa3Enterprise, 7), (.wpa3Personal, 5), (.wpa3Transition, 5),
        (.wpa2Enterprise, 6), (.wpa2Personal, 4), (.personal, 4),
        (.wpaEnterprise, 6), (.wpaEnterpriseMixed, 6),
        (.wpaPersonal, 3), (.wpaPersonalMixed, 3),
        (.enterprise, 6), (.dynamicWEP, 2), (.WEP, 2), (.none, 1),
    ]
    for (mode, wire) in probes where n.supportsSecurity(mode) { return wire }
    return 0
}
func parseBSSID(_ s: String?) -> Data {
    guard let s = s else { return Data() }
    let parts = s.split(separator: ":")
    guard parts.count == 6 else { return Data() }
    var b = Data(capacity: 6)
    for p in parts { guard let v = UInt8(p, radix: 16) else { return Data() }; b.append(v) }
    return b
}
func preferredNetworkSSIDs() -> Set<String> {
    let p = Process()
    p.executableURL = URL(fileURLWithPath: "/usr/sbin/networksetup")
    p.arguments = ["-listpreferredwirelessnetworks", "en0"]
    let pipe = Pipe()
    p.standardOutput = pipe; p.standardError = Pipe()
    do { try p.run() } catch { return [] }
    p.waitUntilExit()
    guard let out = String(data: pipe.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) else { return [] }
    var set = Set<String>()
    for line in out.split(separator: "\n") {
        let s = String(line)
        if s.hasPrefix("\t") { set.insert(String(s.dropFirst())) }
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
            ssid: ssid, bssid: parseBSSID(n.bssid),
            rssi: Int16(clamping: n.rssiValue),
            noise: Int16(clamping: n.noiseMeasurement),
            channel: UInt16(clamping: ch?.channelNumber ?? 0),
            band: ch.map { mapBand($0.channelBand) } ?? 0,
            channelWidth: ch.map { mapWidth($0.channelWidth) } ?? 0,
            security: detectSecurity(n),
            phyMode: "", password: "",
            current: currentSSID == ssid, saved: saved.contains(ssid)
        ))
    }
    for ssid in saved where byName[ssid] == nil {
        out.append(ScannedNetwork(
            ssid: ssid, bssid: Data(),
            rssi: 0, noise: 0, channel: 0, band: 0, channelWidth: 0,
            security: 0, phyMode: "", password: "",
            current: currentSSID == ssid, saved: true
        ))
    }
    out.sort { ($0.rssi == 0 ? -999 : Int($0.rssi)) > ($1.rssi == 0 ? -999 : Int($1.rssi)) }
    return out
}

// ─────────────────────────── keychain ──────────────────────────────────────

func keychainPassword(ssid: String) -> (String, String) {
    let query: [String: Any] = [
        kSecClass as String:            kSecClassGenericPassword,
        kSecAttrDescription as String:  "AirPort network password",
        kSecAttrAccount as String:      ssid,
        kSecReturnData as String:       true,
        kSecMatchLimit as String:       kSecMatchLimitOne,
    ]
    var result: CFTypeRef?
    let status = SecItemCopyMatching(query as CFDictionary, &result)
    switch status {
    case errSecSuccess:
        if let data = result as? Data, let s = String(data: data, encoding: .utf8) { return (s, "") }
        return ("", "keychain returned non-utf8 data")
    case errSecItemNotFound:
        return ("", "")
    case errSecUserCanceled:
        return ("", "user declined keychain access")
    default:
        return ("", "SecItemCopyMatching status \(status)")
    }
}

// ─────────────────────────── encoders ──────────────────────────────────────

func encodeScanResponse(networks: [ScannedNetwork], error: String) -> Data {
    var w = BinaryWriter()
    w.putMagic("MWIF"); w.putU8(1); w.putU8(0x01)
    w.putString16(error)
    w.putU32LE(UInt32(networks.count))
    for n in networks {
        w.putString16(n.ssid); w.putBytes8(n.bssid)
        w.putI16LE(n.rssi);   w.putI16LE(n.noise)
        w.putU16LE(n.channel); w.putU8(n.band); w.putU16LE(n.channelWidth)
        w.putU8(n.security);   w.putString8(n.phyMode)
        w.putString16(n.password)
        var f: UInt8 = 0
        if n.current { f |= 0x01 }; if n.saved { f |= 0x02 }
        w.putU8(f)
    }
    return w.data
}

func encodePasswordResponse(password: String, error: String) -> Data {
    var w = BinaryWriter()
    w.putMagic("MWIF"); w.putU8(1); w.putU8(0x02)
    w.putString16(error)
    w.putString16(password)
    return w.data
}

// ─────────────────────────── connection + service loop ─────────────────────

func connectLoopback(port: UInt16) -> Int32? {
    let fd = Darwin.socket(AF_INET, SOCK_STREAM, 0)
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
    if rc != 0 { Darwin.close(fd); return nil }
    return fd
}

func handleScan(fd: Int32) {
    let nets: [ScannedNetwork]
    let err: String
    do {
        nets = try runScan()
        err = ""
    } catch ScanError.message(let m) {
        nets = []; err = m
    } catch {
        nets = []; err = "\(error)"
    }
    _ = writeAll(fd, encodeScanResponse(networks: nets, error: err))
}

func handlePassword(fd: Int32) -> Bool {
    guard let ssid = readString16(fd) else { return false }
    let (pw, err) = keychainPassword(ssid: ssid)
    return writeAll(fd, encodePasswordResponse(password: pw, error: err))
}

func runService(port: UInt16) {
    guard let fd = connectLoopback(port: port) else {
        FileHandle.standardError.write("could not connect to 127.0.0.1:\(port)\n".data(using: .utf8)!)
        exit(1)
    }
    defer { Darwin.close(fd) }

    loop: while true {
        // Expect a header: 4 magic + 1 version + 1 msgType
        guard let magic = readExact(fd, 4), String(data: magic, encoding: .ascii) == "MWIF" else { break }
        guard let version = readU8(fd), version == 1 else { break }
        guard let msgType = readU8(fd) else { break }

        switch msgType {
        case 0x10: // scan_request
            handleScan(fd: fd)
        case 0x11: // password_request
            if !handlePassword(fd: fd) { break loop }
        case 0x1F: // close_request
            break loop
        default:
            FileHandle.standardError.write("unknown msgType 0x\(String(msgType, radix: 16))\n".data(using: .utf8)!)
            break loop
        }
    }
}

// ─────────────────────────── parent-death watchdog ────────────────────────

/// Stored at top-level so ARC keeps the DispatchSource alive for the
/// whole process lifetime. A local variable would be released after
/// watchParentPID returns, which cancels the source.
var parentWatchdogSource: DispatchSourceProcess?

/// Watches pid via Dispatch (wraps kqueue EVFILT_PROC / NOTE_EXIT). Calls
/// exit(0) the instant the kernel reports the parent process has exited.
/// Fires regardless of death mode (clean, panic, SIGKILL). No-op if pid
/// is invalid or already dead at setup time.
func watchParentPID(_ pid: pid_t) {
    let source = DispatchSource.makeProcessSource(
        identifier: pid,
        eventMask: .exit,
        queue: .global(qos: .utility),
    )
    source.setEventHandler { exit(0) }
    source.resume()
    parentWatchdogSource = source

    // Race: parent could have died between its spawn and our registration.
    // Probe explicitly; Dispatch won't replay an event that already happened.
    if kill(pid, 0) != 0, errno == ESRCH { exit(0) }
}
