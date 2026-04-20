package macwifi

// Binary wire protocol between the Go library and the signed Swift helper.
//
//   header  := "MWIF" | u8 version | u8 msgType | u16 errLen | errLen·utf8 | u32 count
//   network := u16 ssidLen | ssid | u8 bssidLen | bssid | i16 rssi | i16 noise |
//              u16 channel | u8 band | u16 width | u8 security |
//              u8 phyLen | phy | u16 pwdLen | pwd | u8 flags
//
// All multi-byte integers are little-endian. Strings are UTF-8. `pwd` is empty
// if the Keychain lookup declined or the network has no saved password.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	protocolMagic   = "MWIF"
	protocolVersion = 1

	msgTypeScanResponse     = 0x01
	msgTypePasswordResponse = 0x02

	flagCurrent = 1 << 0
	flagSaved   = 1 << 1
)

var byteOrder = binary.LittleEndian

// readHeader reads the MWIF magic/version/msgType/errLen/errMsg prefix from
// r. Returns the message type (for the caller to dispatch body parsing) or
// the server-reported error.
func readHeader(r io.Reader) (msgType uint8, err error) {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return 0, fmt.Errorf("read magic: %w", err)
	}
	if string(magic[:]) != protocolMagic {
		return 0, fmt.Errorf("bad magic: got %q, want %q", magic, protocolMagic)
	}
	var version uint8
	if err := readInt(r, &version); err != nil {
		return 0, err
	}
	if version != protocolVersion {
		return 0, fmt.Errorf("unsupported protocol version %d (want %d)", version, protocolVersion)
	}
	if err := readInt(r, &msgType); err != nil {
		return 0, err
	}
	var errLen uint16
	if err := readInt(r, &errLen); err != nil {
		return 0, err
	}
	if errLen > 0 {
		buf := make([]byte, errLen)
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, fmt.Errorf("read error message: %w", err)
		}
		return msgType, errors.New(string(buf))
	}
	return msgType, nil
}

// decodeScanResponse reads a scan-response message and returns the networks.
func decodeScanResponse(r io.Reader) ([]Network, error) {
	msgType, err := readHeader(r)
	if err != nil {
		return nil, err
	}
	if msgType != msgTypeScanResponse {
		return nil, fmt.Errorf("expected scan_response (0x01), got 0x%02x", msgType)
	}
	var count uint32
	if err := readInt(r, &count); err != nil {
		return nil, err
	}
	nets := make([]Network, 0, count)
	for i := uint32(0); i < count; i++ {
		n, err := decodeNetwork(r)
		if err != nil {
			return nil, fmt.Errorf("network %d: %w", i, err)
		}
		nets = append(nets, n)
	}
	return nets, nil
}

// decodePasswordResponse reads a password-response message.
func decodePasswordResponse(r io.Reader) (string, error) {
	msgType, err := readHeader(r)
	if err != nil {
		return "", err
	}
	if msgType != msgTypePasswordResponse {
		return "", fmt.Errorf("expected password_response (0x02), got 0x%02x", msgType)
	}
	return readString16(r)
}

func decodeNetwork(r io.Reader) (Network, error) {
	var n Network

	ssid, err := readString16(r)
	if err != nil {
		return n, err
	}
	n.SSID = ssid

	bssid, err := readBytes8(r)
	if err != nil {
		return n, err
	}
	if len(bssid) > 0 {
		n.BSSID = formatMAC(bssid)
	}

	var rssi, noise int16
	if err := readInt(r, &rssi); err != nil {
		return n, err
	}
	n.RSSI = int(rssi)
	if err := readInt(r, &noise); err != nil {
		return n, err
	}
	n.Noise = int(noise)

	var channel uint16
	if err := readInt(r, &channel); err != nil {
		return n, err
	}
	n.Channel = int(channel)

	var band uint8
	if err := readInt(r, &band); err != nil {
		return n, err
	}
	n.ChannelBand = Band(band)

	var width uint16
	if err := readInt(r, &width); err != nil {
		return n, err
	}
	n.ChannelWidth = int(width)

	var sec uint8
	if err := readInt(r, &sec); err != nil {
		return n, err
	}
	n.Security = Security(sec)

	phy, err := readString8(r)
	if err != nil {
		return n, err
	}
	n.PHYMode = phy

	pwd, err := readString16(r)
	if err != nil {
		return n, err
	}
	n.Password = pwd

	var flags uint8
	if err := readInt(r, &flags); err != nil {
		return n, err
	}
	n.Current = flags&flagCurrent != 0
	n.Saved = flags&flagSaved != 0

	return n, nil
}

func readInt(r io.Reader, v any) error {
	return binary.Read(r, byteOrder, v)
}

func readBytes8(r io.Reader) ([]byte, error) {
	var n uint8
	if err := readInt(r, &n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(r, buf)
	return buf, err
}

func readString8(r io.Reader) (string, error) {
	b, err := readBytes8(r)
	return string(b), err
}

func readString16(r io.Reader) (string, error) {
	var n uint16
	if err := readInt(r, &n); err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func formatMAC(b []byte) string {
	if len(b) != 6 {
		return ""
	}
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5])
}
