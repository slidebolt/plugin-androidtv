//go:build integration

package main

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	translate "github.com/slidebolt/plugin-androidtv/internal/translate"
)

func loadEnvLocal(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile("../../.env.local")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			t.Setenv(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
}

// TestDiscovery_FindAndroidTVs scans the network via mDNS (_googlecast._tcp)
// for Android TV / Google TV devices and fails if none are found.
//
// Run: go test -tags integration -v -run TestDiscovery_FindAndroidTVs ./cmd/plugin-androidtv/
func TestDiscovery_FindAndroidTVs(t *testing.T) {
	loadEnvLocal(t)

	timeoutMs := 3000
	if v := os.Getenv("ANDROIDTV_DISCOVERY_TIMEOUT_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			timeoutMs = n
		}
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	t.Logf("scanning for Android TV devices via mDNS (timeout %v)...", timeout)

	devices, err := translate.Discover(timeout)
	if err != nil {
		t.Fatalf("discovery error: %v", err)
	}
	if len(devices) == 0 {
		t.Fatal("no Android TV devices found on network")
	}

	t.Logf("found %d device(s):", len(devices))
	for _, d := range devices {
		t.Logf("  %s — %s @ %s", d.DeviceID, d.SourceName, d.Address)
	}
}
