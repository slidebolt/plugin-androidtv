package main

import "testing"

func TestParseGoogleCastServices(t *testing.T) {
	raw := `
= enp2s0 IPv4 BRAVIA-4K-VH2-70c3d996b7937b5b5ab80f788cacd65a _googlecast._tcp local
   hostname = [70c3d996-b793-7b5b-5ab8-0f788cacd65a.local]
   address = [192.168.88.48]
   port = [8009]
   txt = ["fn=SONY XR-77A80J" "md=BRAVIA 4K VH2" "id=70c3d996b7937b5b5ab80f788cacd65a"]
= enp2s0 IPv4 Smart-TV-Pro-c0e7cbe63b8b4f5ef4560cbef723d33d _googlecast._tcp local
   hostname = [c0e7cbe6-3b8b-4f5e-f456-0cbef723d33d.local]
   address = [192.168.88.54]
   port = [8009]
   txt = ["rr=AndroidNativeApp" "fn=55R646" "md=Smart TV Pro" "id=c0e7cbe63b8b4f5ef4560cbef723d33d"]
= enp2s0 IPv4 Google-Home-1234 _googlecast._tcp local
   hostname = [google-home-1234.local]
   address = [192.168.88.70]
   port = [8009]
   txt = ["fn=Kitchen speaker" "md=Google Home" "id=googlehome1234"]
`

	services := parseGoogleCastServices(raw)
	if len(services) != 3 {
		t.Fatalf("services=%d want=3", len(services))
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
