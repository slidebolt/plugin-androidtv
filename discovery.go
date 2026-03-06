package main

import (
	"context"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/slidebolt/sdk-types"
	castdns "github.com/vishen/go-chromecast/dns"
)

const (
	googleCastService = "_googlecast._tcp"
	devicePrefix      = "androidtv-"
	discoveryTimeout  = 1 * time.Second
)

var idCleaner = regexp.MustCompile(`[^a-z0-9]+`)

type castService struct {
	ServiceName string
	HostName    string
	Address     string
	Port        int
	TXT         map[string]string
}

type discoveredTV struct {
	Device  types.Device
	Address string
}

func discoverAndroidTVDevices(ctx context.Context) ([]discoveredTV, error) {
	log.Printf("plugin-androidtv: starting mDNS discovery (timeout %v)", discoveryTimeout)
	scanCtx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	log.Printf("plugin-androidtv: calling castdns.DiscoverCastDNSEntries")
	entries, err := castdns.DiscoverCastDNSEntries(scanCtx, nil)
	if err != nil {
		log.Printf("plugin-androidtv: castdns error: %v", err)
		return nil, err
	}

	log.Printf("plugin-androidtv: reading entries from channel")
	services := make([]castService, 0)
	for entry := range entries {
		services = append(services, castServiceFromDNSEntry(entry))
	}
	log.Printf("plugin-androidtv: finished reading entries, found %d services", len(services))

	return devicesFromCastServices(services), nil
}

func castServiceFromDNSEntry(entry castdns.CastEntry) castService {
	address := ""
	if entry.AddrV4 != nil {
		address = entry.AddrV4.String()
	}
	return castService{
		ServiceName: entry.Name,
		HostName:    entry.Host,
		Address:     address,
		Port:        entry.Port,
		TXT:         entry.InfoFields,
	}
}

func devicesFromCastServices(services []castService) []discoveredTV {
	devicesByID := map[string]discoveredTV{}

	for _, s := range services {
		if !isLikelyAndroidTV(s) {
			continue
		}
		sourceID := firstNonEmpty(s.TXT["id"], s.Address, s.ServiceName)
		if sourceID == "" {
			continue
		}
		deviceID := makeDeviceID(sourceID)
		name := firstNonEmpty(s.TXT["fn"], s.TXT["md"], s.ServiceName)
		next := discoveredTV{
			Address: s.Address,
			Device: types.Device{
				ID:         deviceID,
				SourceID:   sourceID,
				SourceName: name,
				Labels: map[string][]string{
					"protocol": {"googlecast"},
				},
			},
		}
		if cur, ok := devicesByID[deviceID]; ok {
			// Keep the first discovered device metadata, but upgrade address when we
			// later see an IPv4 record after an IPv6-only record.
			if cur.Address == "" && next.Address != "" {
				cur.Address = next.Address
				devicesByID[deviceID] = cur
			}
			continue
		}
		devicesByID[deviceID] = next
	}

	devices := make([]discoveredTV, 0, len(devicesByID))
	for _, d := range devicesByID {
		devices = append(devices, d)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Device.ID < devices[j].Device.ID })
	return devices
}

func isLikelyAndroidTV(s castService) bool {
	if strings.Contains(strings.ToLower(s.TXT["rr"]), "androidnativeapp") {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		s.ServiceName, s.TXT["md"], s.TXT["fn"], s.HostName,
	}, " "))
	indicators := []string{
		"android", "google tv", "smart tv", "bravia", "sony", "shield", "tcl", "hisense", "tv",
	}
	for _, marker := range indicators {
		if strings.Contains(haystack, marker) {
			return true
		}
	}
	return false
}

func makeDeviceID(sourceID string) string {
	cleaned := strings.ToLower(strings.TrimSpace(sourceID))
	cleaned = idCleaner.ReplaceAllString(cleaned, "-")
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		cleaned = "unknown"
	}
	return devicePrefix + cleaned
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}
