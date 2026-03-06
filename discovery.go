package main

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/slidebolt/sdk-types"
)

const (
	googleCastService = "_googlecast._tcp"
	devicePrefix      = "androidtv-"
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
	out, err := exec.CommandContext(ctx, "avahi-browse", "-rt", googleCastService).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("avahi-browse failed: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	services := parseGoogleCastServices(string(out))
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
	return devices, nil
}

func parseGoogleCastServices(raw string) []castService {
	lines := strings.Split(raw, "\n")
	services := make([]castService, 0)
	var cur *castService

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "= ") {
			if cur != nil {
				services = append(services, *cur)
			}
			cur = parseServiceHeader(trimmed)
			continue
		}
		if cur == nil {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "hostname = ["):
			cur.HostName = parseBracketValue(trimmed, "hostname = [")
		case strings.HasPrefix(trimmed, "address = ["):
			addr := parseBracketValue(trimmed, "address = [")
			if addr != "" && !strings.Contains(addr, ":") {
				cur.Address = addr
			}
		case strings.HasPrefix(trimmed, "port = ["):
			portStr := parseBracketValue(trimmed, "port = [")
			port, err := strconv.Atoi(portStr)
			if err == nil {
				cur.Port = port
			}
		case strings.HasPrefix(trimmed, "txt = ["):
			cur.TXT = parseTXTRecordLine(trimmed)
		}
	}
	if cur != nil {
		services = append(services, *cur)
	}

	return services
}

func parseServiceHeader(line string) *castService {
	fields := strings.Fields(line)
	for i := 0; i < len(fields); i++ {
		if fields[i] != googleCastService {
			continue
		}
		if i < 1 {
			return nil
		}
		return &castService{
			ServiceName: fields[i-1],
			TXT:         map[string]string{},
		}
	}
	return nil
}

func parseBracketValue(line, prefix string) string {
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	out := strings.TrimPrefix(line, prefix)
	out = strings.TrimSuffix(out, "]")
	return strings.TrimSpace(out)
}

func parseTXTRecordLine(line string) map[string]string {
	records := map[string]string{}
	// Example: txt = ["k=v" "k2=v2" "flag"]
	parts := strings.Split(line, "\"")
	for i := 1; i < len(parts); i += 2 {
		token := strings.TrimSpace(parts[i])
		if token == "" {
			continue
		}
		key, val, ok := strings.Cut(token, "=")
		if !ok {
			records[token] = ""
			continue
		}
		records[key] = val
	}
	return records
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
