package translate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

type Discovery struct {
	timeout  time.Duration
	resolver *zeroconf.Resolver

	mu      sync.RWMutex
	devices map[string]AndroidTV
	stopCh  chan struct{}
}

func NewDiscovery(timeout time.Duration) (*Discovery, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("create mDNS resolver: %w", err)
	}
	return &Discovery{
		timeout:  timeout,
		resolver: resolver,
		devices:  make(map[string]AndroidTV),
		stopCh:   make(chan struct{}),
	}, nil
}

func (d *Discovery) Stop() {
	select {
	case <-d.stopCh:
	default:
		close(d.stopCh)
	}
}

func (d *Discovery) Listen(ctx context.Context, onDevice func(AndroidTV)) {
	go d.listenLoop(ctx, onDevice)
}

func (d *Discovery) Discover(ctx context.Context) ([]AndroidTV, error) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		if err := d.resolver.Browse(ctx, GoogleCastService, "local", entries); err != nil {
			// Browse delivers errors via closed context patterns; the caller
			// observes them through the timed discovery result.
		}
	}()

	const idleWindow = 600 * time.Millisecond
	idle := time.NewTimer(idleWindow)
	defer idle.Stop()

	for {
		select {
		case <-ctx.Done():
			return d.snapshot(), nil
		case <-d.stopCh:
			return d.snapshot(), nil
		case entry, ok := <-entries:
			if !ok {
				return d.snapshot(), nil
			}
			if tv, ok := d.entryToAndroidTV(entry); ok {
				d.upsert(tv)
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idleWindow)
		case <-idle.C:
			return d.snapshot(), nil
		}
	}
}

func (d *Discovery) listenLoop(ctx context.Context, onDevice func(AndroidTV)) {
	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		_ = d.resolver.Browse(ctx, GoogleCastService, "local", entries)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case entry, ok := <-entries:
			if !ok {
				return
			}
			tv, ok := d.entryToAndroidTV(entry)
			if !ok {
				continue
			}
			if changed := d.upsert(tv); changed && onDevice != nil {
				go onDevice(tv)
			}
		}
	}
}

func (d *Discovery) snapshot() []AndroidTV {
	d.mu.RLock()
	defer d.mu.RUnlock()

	out := make([]AndroidTV, 0, len(d.devices))
	for _, tv := range d.devices {
		out = append(out, tv)
	}
	sortDevices(out)
	return out
}

func (d *Discovery) upsert(tv AndroidTV) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	current, ok := d.devices[tv.DeviceID]
	if !ok {
		d.devices[tv.DeviceID] = tv
		return true
	}

	changed := false
	if current.Address == "" && tv.Address != "" {
		current.Address = tv.Address
		changed = true
	}
	if strings.Contains(current.Address, ":") && tv.Address != "" && !strings.Contains(tv.Address, ":") {
		current.Address = tv.Address
		changed = true
	}
	if current.SourceName == "" && tv.SourceName != "" {
		current.SourceName = tv.SourceName
		changed = true
	}
	if !current.IsTV && tv.IsTV {
		current.IsTV = true
		changed = true
	}
	if changed {
		d.devices[tv.DeviceID] = current
	}
	return changed
}

func (d *Discovery) entryToAndroidTV(entry *zeroconf.ServiceEntry) (AndroidTV, bool) {
	txt := make(map[string]string)
	for _, item := range entry.Text {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			txt[parts[0]] = parts[1]
		}
	}

	address := ""
	if len(entry.AddrIPv4) > 0 {
		address = entry.AddrIPv4[0].String()
	} else if len(entry.AddrIPv6) > 0 {
		address = entry.AddrIPv6[0].String()
	}

	service := CastService{
		ServiceName: entry.Instance,
		HostName:    entry.HostName,
		Address:     address,
		Port:        entry.Port,
		TXT:         txt,
	}
	if !isLikelyAndroidTV(service) {
		return AndroidTV{}, false
	}

	sourceID := firstNonEmpty(service.TXT["id"], service.Address, service.ServiceName, service.HostName)
	if sourceID == "" {
		return AndroidTV{}, false
	}

	return AndroidTV{
		DeviceID:   makeDeviceID(sourceID),
		SourceID:   sourceID,
		SourceName: firstNonEmpty(service.TXT["fn"], service.TXT["md"], service.ServiceName),
		Address:    service.Address,
		Port:       service.Port,
		TXT:        service.TXT,
		IsTV:       true,
	}, true
}
