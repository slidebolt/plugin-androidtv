// plugin-androidtv discovers and controls Android TV devices via Google Cast protocol
package translate

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	castdns "github.com/vishen/go-chromecast/dns"
)

const (
	GoogleCastService = "_googlecast._tcp"
	DevicePrefix      = "androidtv-"
	DefaultTimeout    = 3 * time.Second
)

var idCleaner = regexp.MustCompile(`[^a-z0-9]+`)

type CastService struct {
	ServiceName string
	HostName    string
	Address     string
	Port        int
	Interface   string
	TXT         map[string]string
}

type AndroidTV struct {
	DeviceID   string
	SourceID   string
	SourceName string
	Address    string
	Port       int
	TXT        map[string]string
	IsTV       bool
}

func Discover(timeout time.Duration) ([]AndroidTV, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	scanCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	entries, err := castdns.DiscoverCastDNSEntries(scanCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("mDNS discover failed: %w", err)
	}

	services := make([]CastService, 0)
	for entry := range entries {
		services = append(services, castServiceFromDNSEntry(entry))
	}
	return devicesFromServices(services), nil
}

func castServiceFromDNSEntry(entry castdns.CastEntry) CastService {
	address := ""
	switch {
	case entry.AddrV4 != nil:
		address = entry.AddrV4.String()
	case entry.AddrV6 != nil:
		address = entry.AddrV6.String()
	}
	return CastService{
		ServiceName: entry.Name,
		HostName:    entry.Host,
		Address:     address,
		Port:        entry.Port,
		TXT:         entry.InfoFields,
	}
}

func devicesFromServices(services []CastService) []AndroidTV {
	devicesByID := map[string]AndroidTV{}

	for _, s := range services {
		isTV := isLikelyAndroidTV(s)
		if !isTV {
			continue
		}
		sourceID := firstNonEmpty(s.TXT["id"], s.Address, s.ServiceName)
		if sourceID == "" {
			sourceID = s.HostName
		}
		if sourceID == "" {
			continue
		}

		deviceID := makeDeviceID(sourceID)
		name := firstNonEmpty(s.TXT["fn"], s.TXT["md"], s.ServiceName)

		tv := AndroidTV{
			DeviceID:   deviceID,
			SourceID:   sourceID,
			SourceName: name,
			Address:    s.Address,
			Port:       s.Port,
			TXT:        s.TXT,
			IsTV:       isTV,
		}

		if cur, ok := devicesByID[deviceID]; ok {
			if cur.Address == "" && tv.Address != "" {
				cur.Address = tv.Address
				devicesByID[deviceID] = cur
				continue
			}
			if strings.Contains(cur.Address, ":") && tv.Address != "" && !strings.Contains(tv.Address, ":") {
				cur.Address = tv.Address
				devicesByID[deviceID] = cur
			}
			continue
		}
		devicesByID[deviceID] = tv
	}

	devices := make([]AndroidTV, 0, len(devicesByID))
	for _, d := range devicesByID {
		devices = append(devices, d)
	}
	sortDevices(devices)
	return devices
}

func sortDevices(devices []AndroidTV) {
	sort.Slice(devices, func(i, j int) bool { return devices[i].DeviceID < devices[j].DeviceID })
}

func isLikelyAndroidTV(s CastService) bool {
	if strings.Contains(strings.ToLower(s.TXT["rr"]), "androidnativeapp") {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		s.ServiceName, s.TXT["md"], s.TXT["fn"], s.HostName,
	}, " "))
	indicators := []string{
		"android", "google tv", "smart tv", "bravia", "sony", "shield",
		"tcl", "hisense", "tv", "chromecast",
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
	return DevicePrefix + cleaned
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
