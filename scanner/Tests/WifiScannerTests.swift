import Darwin
import Foundation

struct TestFailure: Error, CustomStringConvertible {
    let message: String

    var description: String { message }
}

typealias TestCase = (name: String, run: () throws -> Void)

func expectTrue(_ actual: Bool, _ message: String) throws {
    if !actual {
        throw TestFailure(message: message)
    }
}

func expectNil<T>(_ actual: T?, _ message: String) throws {
    if actual != nil {
        throw TestFailure(message: message)
    }
}

func expectEqual<T: Equatable>(_ actual: T, _ expected: T, _ message: String) throws {
    if actual != expected {
        throw TestFailure(message: "\(message)\nactual:   \(actual)\nexpected: \(expected)")
    }
}

func testBinaryWriterUsesLittleEndianAndLengthPrefixes() throws {
    var writer = BinaryWriter()
    writer.putMagic("MWIF")
    writer.putU8(0x01)
    writer.putU16LE(0x1234)
    writer.putU32LE(0x89ABCDEF)
    writer.putI16LE(-2)
    writer.putString8("hi")
    writer.putString16("wifi")
    writer.putBytes8(Data([0xAA, 0xBB]))

    try expectEqual(
        Array(writer.data),
        [
            0x4D, 0x57, 0x49, 0x46,
            0x01,
            0x34, 0x12,
            0xEF, 0xCD, 0xAB, 0x89,
            0xFE, 0xFF,
            0x02, 0x68, 0x69,
            0x04, 0x00, 0x77, 0x69, 0x66, 0x69,
            0x02, 0xAA, 0xBB,
        ],
        "BinaryWriter should encode values in the helper wire format"
    )
}

func testReadExactReadU16AndReadString16() throws {
    let fds = try makePipe()
    var writeFD: Int32? = fds.write
    defer {
        Darwin.close(fds.read)
        if let fd = writeFD { Darwin.close(fd) }
    }

    var payload = Data()
    payload.append(contentsOf: [0x34, 0x12])
    payload.append(contentsOf: [0x04, 0x00])
    payload.append(contentsOf: Array("wifi".utf8))

    try expectTrue(writeAll(fds.write, payload), "writeAll should write the payload")
    Darwin.close(fds.write)
    writeFD = nil

    try expectEqual(readU16LE(fds.read), 0x1234, "readU16LE should decode little-endian UInt16")
    try expectEqual(readString16(fds.read), "wifi", "readString16 should decode UTF-8 strings")
    try expectNil(readU8(fds.read), "readU8 should return nil at EOF")
}

func testWriteAllWritesFullPayload() throws {
    let fds = try makePipe()
    var writeFD: Int32? = fds.write
    defer {
        Darwin.close(fds.read)
        if let fd = writeFD { Darwin.close(fd) }
    }

    let payload = Data((0..<4096).map { UInt8($0 % 251) })
    try expectTrue(writeAll(fds.write, payload), "writeAll should write the full payload")
    Darwin.close(fds.write)
    writeFD = nil

    try expectEqual(readExact(fds.read, payload.count), payload, "readExact should read the full payload")
    try expectNil(readExact(fds.read, 1), "readExact should return nil at EOF")
}

func testParseBSSIDAcceptsHexOctets() throws {
    try expectEqual(
        parseBSSID("01:23:45:ab:CD:ef"),
        Data([0x01, 0x23, 0x45, 0xAB, 0xCD, 0xEF]),
        "parseBSSID should accept six hex octets"
    )
}

func testParseBSSIDRejectsMissingInvalidOrShortValues() throws {
    try expectEqual(parseBSSID(nil), Data(), "nil BSSID should parse as empty data")
    try expectEqual(parseBSSID("01:23:45:67:89"), Data(), "short BSSID should parse as empty data")
    try expectEqual(parseBSSID("01:23:45:67:89:xx"), Data(), "invalid BSSID should parse as empty data")
}

func testEncodePasswordResponseMatchesWireFormat() throws {
    let response = encodePasswordResponse(password: "secret", error: "denied")

    try expectEqual(
        Array(response),
        [
            0x4D, 0x57, 0x49, 0x46,
            0x01,
            0x02,
            0x06, 0x00, 0x64, 0x65, 0x6E, 0x69, 0x65, 0x64,
            0x06, 0x00, 0x73, 0x65, 0x63, 0x72, 0x65, 0x74,
        ],
        "password response should match the Go protocol wire format"
    )
}

func testEncodeScanResponseMatchesWireFormat() throws {
    let network = ScannedNetwork(
        ssid: "Cafe",
        bssid: Data([0x00, 0x11, 0x22, 0x33, 0x44, 0x55]),
        rssi: -42,
        noise: -91,
        channel: 149,
        band: 2,
        channelWidth: 80,
        security: 4,
        phyMode: "ax",
        password: "pw",
        current: true,
        saved: true
    )

    let response = encodeScanResponse(networks: [network], error: "")

    try expectEqual(
        Array(response),
        [
            0x4D, 0x57, 0x49, 0x46,
            0x01,
            0x01,
            0x00, 0x00,
            0x01, 0x00, 0x00, 0x00,
            0x04, 0x00, 0x43, 0x61, 0x66, 0x65,
            0x06, 0x00, 0x11, 0x22, 0x33, 0x44, 0x55,
            0xD6, 0xFF,
            0xA5, 0xFF,
            0x95, 0x00,
            0x02,
            0x50, 0x00,
            0x04,
            0x02, 0x61, 0x78,
            0x02, 0x00, 0x70, 0x77,
            0x03,
        ],
        "scan response should match the Go protocol wire format"
    )
}

func makePipe() throws -> (read: Int32, write: Int32) {
    var fds: [Int32] = [0, 0]
    try expectEqual(Darwin.pipe(&fds), 0, "pipe() should succeed")
    return (fds[0], fds[1])
}

@main
struct WifiScannerTestRunner {
    static func main() {
        let tests: [TestCase] = [
            ("BinaryWriter encodes little-endian values", testBinaryWriterUsesLittleEndianAndLengthPrefixes),
            ("Binary readers decode pipe data", testReadExactReadU16AndReadString16),
            ("writeAll writes full payloads", testWriteAllWritesFullPayload),
            ("parseBSSID accepts valid hex octets", testParseBSSIDAcceptsHexOctets),
            ("parseBSSID rejects invalid values", testParseBSSIDRejectsMissingInvalidOrShortValues),
            ("encodePasswordResponse matches wire format", testEncodePasswordResponseMatchesWireFormat),
            ("encodeScanResponse matches wire format", testEncodeScanResponseMatchesWireFormat),
        ]

        var failures = 0
        for test in tests {
            do {
                try test.run()
                print("PASS \(test.name)")
            } catch {
                failures += 1
                fputs("FAIL \(test.name): \(error)\n", stderr)
            }
        }

        if failures > 0 {
            fputs("\(failures) Swift helper test(s) failed\n", stderr)
            exit(1)
        }
    }
}
