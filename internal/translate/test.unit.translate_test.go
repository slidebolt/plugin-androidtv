package translate

import (
	"net"
	"testing"

	"github.com/grandcat/zeroconf"
)

func mustParseIP(t *testing.T, raw string) net.IP {
	t.Helper()
	ip := net.ParseIP(raw)
	if ip == nil {
		t.Fatalf("parse ip %q", raw)
	}
	return ip
}

func TestDiscoveryUpsert_PrefersIPv4(t *testing.T) {
	d := &Discovery{
		devices: make(map[string]AndroidTV),
		stopCh:  make(chan struct{}),
	}

	firstChanged := d.upsert(AndroidTV{
		DeviceID:   "androidtv-abcd",
		SourceID:   "abcd",
		SourceName: "Living Room TV",
		Address:    "fe80::1",
		IsTV:       true,
	})
	secondChanged := d.upsert(AndroidTV{
		DeviceID:   "androidtv-abcd",
		SourceID:   "abcd",
		SourceName: "Living Room TV",
		Address:    "192.168.88.48",
		IsTV:       true,
	})
	got := d.snapshot()

	if !firstChanged || !secondChanged {
		t.Fatalf("expected both upserts to report changes: first=%v second=%v", firstChanged, secondChanged)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Address != "192.168.88.48" {
		t.Fatalf("address = %q, want IPv4", got[0].Address)
	}
}

func TestDiscoveryEntryToAndroidTV(t *testing.T) {
	d := &Discovery{
		devices: make(map[string]AndroidTV),
		stopCh:  make(chan struct{}),
	}
	entry := &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: "SONY XR-77A80J",
		},
		HostName: "sony.local.",
		Port:     8009,
		Text:     []string{"id=abc123", "fn=SONY XR-77A80J", "rr=AndroidNativeApp"},
	}
	entry.AddrIPv4 = append(entry.AddrIPv4, mustParseIP(t, "192.168.88.48"))

	tv, ok := d.entryToAndroidTV(entry)
	if !ok {
		t.Fatal("expected android tv entry")
	}
	if tv.DeviceID != "androidtv-abc123" {
		t.Fatalf("deviceID = %q", tv.DeviceID)
	}
	if tv.Address != "192.168.88.48" {
		t.Fatalf("address = %q", tv.Address)
	}
}

func TestDevicesFromServices_PrefersIPv4AndFiltersToTVs(t *testing.T) {
	got := devicesFromServices([]CastService{
		{
			ServiceName: "Living Room TV",
			HostName:    "living-room.local",
			Address:     "fe80::1",
			Port:        8009,
			TXT:         map[string]string{"id": "abcd", "fn": "Living Room TV"},
		},
		{
			ServiceName: "Living Room TV",
			HostName:    "living-room.local",
			Address:     "192.168.88.48",
			Port:        8009,
			TXT:         map[string]string{"id": "abcd", "fn": "Living Room TV"},
		},
		{
			ServiceName: "Kitchen Speaker",
			HostName:    "speaker.local",
			Address:     "192.168.88.49",
			Port:        8009,
			TXT:         map[string]string{"id": "speaker", "fn": "Kitchen Speaker"},
		},
	})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Address != "192.168.88.48" {
		t.Fatalf("address = %q, want IPv4", got[0].Address)
	}
	if got[0].DeviceID != "androidtv-abcd" {
		t.Fatalf("deviceID = %q", got[0].DeviceID)
	}
}

func TestIsLikelyAndroidTV_ReceiverHint(t *testing.T) {
	if !isLikelyAndroidTV(CastService{TXT: map[string]string{"rr": "AndroidNativeApp"}}) {
		t.Fatal("expected rr androidnativeapp hint to be accepted")
	}
}

func TestMakeDeviceID(t *testing.T) {
	tests := []struct{ input, expected string }{
		{"Test TV 123", "androidtv-test-tv-123"},
		{"Samsung_Smart_TV", "androidtv-samsung-smart-tv"},
		{"  Spaces  ", "androidtv-spaces"},
		{"UPPERCASE", "androidtv-uppercase"},
	}
	for _, tc := range tests {
		got := makeDeviceID(tc.input)
		if got != tc.expected {
			t.Errorf("makeDeviceID(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		values   []string
		expected string
	}{
		{[]string{"", "", "value"}, "value"},
		{[]string{"first", "", "second"}, "first"},
		{[]string{"  ", "trimmed"}, "trimmed"},
		{[]string{"", ""}, ""},
	}
	for _, tc := range tests {
		got := firstNonEmpty(tc.values...)
		if got != tc.expected {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", tc.values, got, tc.expected)
		}
	}
}
