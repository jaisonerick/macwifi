package macwifi

// Wire protocol between the Go client and the signed Swift helper.
//
// Connections are bidirectional: once the helper connects back to the Go
// listener, either side may send frames. In practice, Go sends request
// frames and the helper replies with response frames — one response per
// request, in order.
//
// Frame header (every message):
//
//	"MWIF" | u8 version=1 | u8 msgType | body
//
// Body layout depends on msgType:
//
//	0x10 scan_request      (empty)
//	0x11 password_request  u16 ssidLen | ssid
//	0x1F close_request     (empty)
//	0x01 scan_response     u16 errLen | errMsg | u32 count | networks
//	0x02 password_response u16 errLen | errMsg | u16 pwdLen | pwd
//
// All multi-byte integers little-endian. Strings are UTF-8.

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
	msgTypeScanRequest      = 0x10
	msgTypePasswordRequest  = 0x11
	msgTypeCloseRequest     = 0x1F

	flagCurrent = 1 << 0
	flagSaved   = 1 << 1
)

var byteOrder = binary.LittleEndian

// writeRequest writes a request frame to the helper.
func writeRequest(w io.Writer, msgType uint8, body []byte) error {
	buf := make([]byte, 0, 6+len(body))
	buf = append(buf, protocolMagic...)
	buf = append(buf, protocolVersion, msgType)
	buf = append(buf, body...)
	_, err := w.Write(buf)
	return err
}

func writeScanRequest(w io.Writer) error {
	return writeRequest(w, msgTypeScanRequest, nil)
}

func writePasswordRequest(w io.Writer, ssid string) error {
	body := make([]byte, 2+len(ssid))
	byteOrder.PutUint16(body[:2], uint16(len(ssid)))
	copy(body[2:], ssid)
	return writeRequest(w, msgTypePasswordRequest, body)
}

func writeCloseRequest(w io.Writer) error {
	return writeRequest(w, msgTypeCloseRequest, nil)
}

// readHeader reads the "MWIF"|version|msgType prefix. The caller then
// dispatches on msgType to parse the body.
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
	return msgType, nil
}

// readError reads the leading u16 errLen | errMsg from a response body.
// Returns a non-nil error if the server reported one.
func readError(r io.Reader) error {
	var n uint16
	if err := readInt(r, &n); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return fmt.Errorf("read error message: %w", err)
	}
	return errors.New(string(buf))
}

// decodeScanResponse reads a scan-response body (header already consumed).
func decodeScanResponse(r io.Reader) ([]Network, error) {
	if err := readError(r); err != nil {
		return nil, err
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

// decodePasswordResponse reads a password-response body (header already consumed).
func decodePasswordResponse(r io.Reader) (string, error) {
	if err := readError(r); err != nil {
		return "", err
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

func readInt(r io.Reader, v any) error { return binary.Read(r, byteOrder, v) }

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
