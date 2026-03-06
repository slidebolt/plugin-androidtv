package main

import "testing"

func TestDevicesFromCastServices_FilterAndDedup(t *testing.T) {
	services := []castService{
		{
			ServiceName: "BRAVIA-4K-VH2-70c3d996b7937b5b5ab80f788cacd65a",
			HostName:    "70c3d996-b793-7b5b-5ab8-0f788cacd65a.local",
			Address:     "192.168.88.48",
			Port:        8009,
			TXT: map[string]string{
				"fn": "SONY XR-77A80J",
				"md": "BRAVIA 4K VH2",
				"id": "70c3d996b7937b5b5ab80f788cacd65a",
			},
		},
		{
			ServiceName: "Smart-TV-Pro-c0e7cbe63b8b4f5ef4560cbef723d33d",
			HostName:    "c0e7cbe6-3b8b-4f5e-f456-0cbef723d33d.local",
			Address:     "192.168.88.54",
			Port:        8009,
			TXT: map[string]string{
				"rr": "AndroidNativeApp",
				"fn": "55R646",
				"md": "Smart TV Pro",
				"id": "c0e7cbe63b8b4f5ef4560cbef723d33d",
			},
		},
		{
			ServiceName: "Google-Home-1234",
			HostName:    "google-home-1234.local",
			Address:     "192.168.88.70",
			Port:        8009,
			TXT: map[string]string{
				"fn": "Kitchen speaker",
				"md": "Google Home",
				"id": "googlehome1234",
			},
		},
		{
			ServiceName: "Smart-TV-Pro-c0e7cbe63b8b4f5ef4560cbef723d33d-v6",
			HostName:    "c0e7cbe6-3b8b-4f5e-f456-0cbef723d33d.local",
			Address:     "",
			Port:        8009,
			TXT: map[string]string{
				"rr": "AndroidNativeApp",
				"fn": "55R646",
				"md": "Smart TV Pro",
				"id": "c0e7cbe63b8b4f5ef4560cbef723d33d",
			},
		},
	}

	devices := devicesFromCastServices(services)
	if len(devices) != 2 {
		t.Fatalf("devices=%d want=2", len(devices))
	}

	if devices[0].Device.ID != "androidtv-70c3d996b7937b5b5ab80f788cacd65a" {
		t.Fatalf("first device id=%q", devices[0].Device.ID)
	}
	if devices[1].Device.ID != "androidtv-c0e7cbe63b8b4f5ef4560cbef723d33d" {
		t.Fatalf("second device id=%q", devices[1].Device.ID)
	}
	if devices[1].Address != "192.168.88.54" {
		t.Fatalf("expected deduped device to keep ipv4 address, got=%q", devices[1].Address)
	}
}

func TestIsLikelyAndroidTV(t *testing.T) {
	services := []castService{
		{
			ServiceName: "BRAVIA-4K-VH2",
			HostName:    "bravia.local",
			TXT:         map[string]string{"md": "BRAVIA 4K VH2"},
		},
		{
			ServiceName: "Smart-TV-Pro",
			HostName:    "smarttv.local",
			TXT:         map[string]string{"rr": "AndroidNativeApp", "md": "Smart TV Pro"},
		},
		{
			ServiceName: "Google-Home-1234",
			HostName:    "speaker.local",
			TXT:         map[string]string{"md": "Google Home", "fn": "Kitchen speaker"},
		},
	}

	if !isLikelyAndroidTV(services[0]) {
		t.Fatalf("expected BRAVIA to be detected as TV")
	}
	if !isLikelyAndroidTV(services[1]) {
		t.Fatalf("expected Smart-TV-Pro to be detected as TV")
	}
	if isLikelyAndroidTV(services[2]) {
		t.Fatalf("expected Google Home speaker to not be detected as TV")
	}
}

func TestMakeDeviceID(t *testing.T) {
	got := makeDeviceID("c0e7cbe6:3b8b:4f5e:f456:0cbef723d33d")
	want := "androidtv-c0e7cbe6-3b8b-4f5e-f456-0cbef723d33d"
	if got != want {
		t.Fatalf("makeDeviceID=%q want=%q", got, want)
	}
}
