package macwifi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestWriteRequests(t *testing.T) {
	t.Run("scan", func(t *testing.T) {
		var got bytes.Buffer
		if err := writeScanRequest(&got); err != nil {
			t.Fatal(err)
		}
		want := []byte{'M', 'W', 'I', 'F', protocolVersion, msgTypeScanRequest}
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("writeScanRequest() = %x, want %x", got.Bytes(), want)
		}
	})

	t.Run("password", func(t *testing.T) {
		var got bytes.Buffer
		if err := writePasswordRequest(&got, "Office WiFi"); err != nil {
			t.Fatal(err)
		}
		want := []byte{'M', 'W', 'I', 'F', protocolVersion, msgTypePasswordRequest}
		want = binary.LittleEndian.AppendUint16(want, uint16(len("Office WiFi")))
		want = append(want, "Office WiFi"...)
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("writePasswordRequest() = %x, want %x", got.Bytes(), want)
		}
	})

	t.Run("close", func(t *testing.T) {
		var got bytes.Buffer
		if err := writeCloseRequest(&got); err != nil {
			t.Fatal(err)
		}
		want := []byte{'M', 'W', 'I', 'F', protocolVersion, msgTypeCloseRequest}
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("writeCloseRequest() = %x, want %x", got.Bytes(), want)
		}
	})
}

func TestReadHeader(t *testing.T) {
	tests := []struct {
		name    string
		frame   []byte
		want    uint8
		wantErr string
	}{
		{
			name:  "valid",
			frame: []byte{'M', 'W', 'I', 'F', protocolVersion, msgTypeScanResponse},
			want:  msgTypeScanResponse,
		},
		{
			name:    "bad magic",
			frame:   []byte{'N', 'O', 'P', 'E', protocolVersion, msgTypeScanResponse},
			wantErr: "bad magic",
		},
		{
			name:    "bad version",
			frame:   []byte{'M', 'W', 'I', 'F', protocolVersion + 1, msgTypeScanResponse},
			wantErr: "unsupported protocol version",
		},
		{
			name:    "short",
			frame:   []byte{'M', 'W'},
			wantErr: "read magic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readHeader(bytes.NewReader(tt.frame))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("readHeader() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("readHeader() = 0x%02x, want 0x%02x", got, tt.want)
			}
		})
	}
}

func TestDecodeScanResponse(t *testing.T) {
	var body bytes.Buffer
	writeString16(&body, "")
	writeUint32(&body, 1)
	writeNetwork(&body, Network{
		SSID:         "Office WiFi",
		BSSID:        "aa:bb:cc:dd:ee:ff",
		RSSI:         -52,
		Noise:        -91,
		Channel:      149,
		ChannelBand:  Band5GHz,
		ChannelWidth: 80,
		Security:     SecurityWPA2Personal,
		PHYMode:      "802.11ax",
		Password:     "",
		Current:      true,
		Saved:        true,
	})

	got, err := decodeScanResponse(&body)
	if err != nil {
		t.Fatal(err)
	}
	want := []Network{{
		SSID:         "Office WiFi",
		BSSID:        "aa:bb:cc:dd:ee:ff",
		RSSI:         -52,
		Noise:        -91,
		Channel:      149,
		ChannelBand:  Band5GHz,
		ChannelWidth: 80,
		Security:     SecurityWPA2Personal,
		PHYMode:      "802.11ax",
		Password:     "",
		Current:      true,
		Saved:        true,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decodeScanResponse() = %#v, want %#v", got, want)
	}
}

func TestDecodeScanResponseReturnsRemoteError(t *testing.T) {
	var body bytes.Buffer
	writeString16(&body, "Location Services denied.")

	got, err := decodeScanResponse(&body)
	if err == nil || err.Error() != "Location Services denied." {
		t.Fatalf("decodeScanResponse() error = %v, want remote error", err)
	}
	if got != nil {
		t.Fatalf("decodeScanResponse() networks = %#v, want nil", got)
	}
}

func TestDecodeScanResponseIdentifiesTruncatedNetwork(t *testing.T) {
	var body bytes.Buffer
	writeString16(&body, "")
	writeUint32(&body, 1)
	writeString16(&body, "Office WiFi")

	_, err := decodeScanResponse(&body)
	if err == nil || !strings.Contains(err.Error(), "network 0") {
		t.Fatalf("decodeScanResponse() error = %v, want network index", err)
	}
}

func TestDecodePasswordResponse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var body bytes.Buffer
		writeString16(&body, "")
		writeString16(&body, "correct horse")

		got, err := decodePasswordResponse(&body)
		if err != nil {
			t.Fatal(err)
		}
		if got != "correct horse" {
			t.Fatalf("decodePasswordResponse() = %q, want %q", got, "correct horse")
		}
	})

	t.Run("remote error", func(t *testing.T) {
		var body bytes.Buffer
		writeString16(&body, "user declined keychain access")

		got, err := decodePasswordResponse(&body)
		if err == nil || err.Error() != "user declined keychain access" {
			t.Fatalf("decodePasswordResponse() error = %v, want remote error", err)
		}
		if got != "" {
			t.Fatalf("decodePasswordResponse() = %q, want empty password", got)
		}
	})
}

func TestReadHelpersRejectShortInput(t *testing.T) {
	if _, err := readBytes8(bytes.NewReader([]byte{2, 1})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("readBytes8() error = %v, want ErrUnexpectedEOF", err)
	}
	if _, err := readString16(bytes.NewReader([]byte{2, 0, 1})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("readString16() error = %v, want ErrUnexpectedEOF", err)
	}
}

func TestFormatMAC(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{name: "valid", in: []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}, want: "aa:bb:cc:dd:ee:ff"},
		{name: "empty"},
		{name: "short", in: []byte{0xaa, 0xbb}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMAC(tt.in); got != tt.want {
				t.Fatalf("formatMAC(%x) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func writeNetwork(w *bytes.Buffer, n Network) {
	writeString16(w, n.SSID)
	writeBytes8(w, []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	writeInt16(w, int16(n.RSSI))
	writeInt16(w, int16(n.Noise))
	writeUint16(w, uint16(n.Channel))
	w.WriteByte(byte(n.ChannelBand))
	writeUint16(w, uint16(n.ChannelWidth))
	w.WriteByte(byte(n.Security))
	writeString8(w, n.PHYMode)
	writeString16(w, n.Password)
	var flags uint8
	if n.Current {
		flags |= flagCurrent
	}
	if n.Saved {
		flags |= flagSaved
	}
	w.WriteByte(flags)
}

func writeBytes8(w *bytes.Buffer, b []byte) {
	w.WriteByte(byte(len(b)))
	w.Write(b)
}

func writeString8(w *bytes.Buffer, s string) {
	writeBytes8(w, []byte(s))
}

func writeString16(w *bytes.Buffer, s string) {
	writeUint16(w, uint16(len(s)))
	w.WriteString(s)
}

func writeInt16(w *bytes.Buffer, v int16) {
	_ = binary.Write(w, byteOrder, v)
}

func writeUint16(w *bytes.Buffer, v uint16) {
	_ = binary.Write(w, byteOrder, v)
}

func writeUint32(w *bytes.Buffer, v uint32) {
	_ = binary.Write(w, byteOrder, v)
}
