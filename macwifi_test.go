package macwifi

import "testing"

func TestBandString(t *testing.T) {
	tests := []struct {
		band Band
		want string
	}{
		{BandUnknown, "unknown"},
		{Band24GHz, "2.4GHz"},
		{Band5GHz, "5GHz"},
		{Band6GHz, "6GHz"},
		{Band(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.band.String(); got != tt.want {
			t.Fatalf("%v.String() = %q, want %q", uint8(tt.band), got, tt.want)
		}
	}
}

func TestSecurityString(t *testing.T) {
	tests := []struct {
		security Security
		want     string
	}{
		{SecurityUnknown, "unknown"},
		{SecurityNone, "none"},
		{SecurityWEP, "WEP"},
		{SecurityWPA, "WPA"},
		{SecurityWPA2Personal, "WPA2"},
		{SecurityWPA3Personal, "WPA3"},
		{SecurityWPA2Enterprise, "WPA2-Enterprise"},
		{SecurityWPA3Enterprise, "WPA3-Enterprise"},
		{SecurityOWE, "OWE"},
		{Security(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.security.String(); got != tt.want {
			t.Fatalf("%v.String() = %q, want %q", uint8(tt.security), got, tt.want)
		}
	}
}
