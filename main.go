package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

	entityswitch "github.com/slidebolt/sdk-entities/switch"
	runner "github.com/slidebolt/sdk-runner"
	"github.com/slidebolt/sdk-types"
)

type PluginAndroidtvPlugin struct {
	dataDir   string
	commander tvCommander
	eventSink runner.EventSink

	mu       sync.RWMutex
	devices  map[string]discoveredTV
	deviceIP map[string]string
}

type discoveryOverrides struct {
	Entities map[string][]types.Entity `json:"entities"`
}

func (p *PluginAndroidtvPlugin) OnInitialize(config runner.Config, state types.Storage) (types.Manifest, types.Storage) {
	p.dataDir = config.DataDir
	p.eventSink = config.EventSink
	if p.commander == nil {
		p.commander = shellTVCommander{}
	}
	if p.devices == nil {
		p.devices = map[string]discoveredTV{}
	}
	if p.deviceIP == nil {
		p.deviceIP = map[string]string{}
	}
	schemas := append([]types.DomainDescriptor{}, types.CoreDomains()...)
	schemas = append(schemas, mediaCastDomainDescriptor())
	return types.Manifest{ID: "plugin-androidtv", Name: "Android TV Plugin", Version: "1.0.0", Schemas: schemas}, state
}

func (p *PluginAndroidtvPlugin) OnReady() {}
func (p *PluginAndroidtvPlugin) WaitReady(ctx context.Context) error {
	return nil
}

func (p *PluginAndroidtvPlugin) OnShutdown()                    {}
func (p *PluginAndroidtvPlugin) OnHealthCheck() (string, error) { return "perfect", nil }
func (p *PluginAndroidtvPlugin) OnStorageUpdate(current types.Storage) (types.Storage, error) {
	return current, nil
}

func (p *PluginAndroidtvPlugin) OnDeviceCreate(dev types.Device) (types.Device, error) {
	return dev, nil
}
func (p *PluginAndroidtvPlugin) OnDeviceUpdate(dev types.Device) (types.Device, error) {
	return dev, nil
}
func (p *PluginAndroidtvPlugin) OnDeviceDelete(id string) error { return nil }
func (p *PluginAndroidtvPlugin) OnDevicesList(current []types.Device) ([]types.Device, error) {
	existing := map[string]types.Device{}
	for _, d := range current {
		existing[d.ID] = d
	}

	discovered, err := discoverAndroidTVDevices(context.Background())
	if err != nil {
		log.Printf("plugin-androidtv discovery failed: %v", err)
	} else {
		latest := p.updateDiscoveredCache(discovered)
		for _, d := range latest {
			if existingDev, ok := existing[d.Device.ID]; ok {
				existing[d.Device.ID] = runner.ReconcileDevice(existingDev, d.Device)
			} else {
				existing[d.Device.ID] = runner.ReconcileDevice(types.Device{}, d.Device)
			}
		}
	}

	out := make([]types.Device, 0, len(existing))
	for _, d := range existing {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return runner.EnsureCoreDevice("plugin-androidtv", out), nil
}
func (p *PluginAndroidtvPlugin) OnDeviceSearch(q types.SearchQuery, res []types.Device) ([]types.Device, error) {
	return res, nil
}

func (p *PluginAndroidtvPlugin) OnEntityCreate(e types.Entity) (types.Entity, error) { return e, nil }
func (p *PluginAndroidtvPlugin) OnEntityUpdate(e types.Entity) (types.Entity, error) { return e, nil }
func (p *PluginAndroidtvPlugin) OnEntityDelete(d, e string) error                    { return nil }
func (p *PluginAndroidtvPlugin) OnEntitiesList(d string, c []types.Entity) ([]types.Entity, error) {
	if ov, ok := p.loadEntityOverride(d); ok {
		for i := range ov {
			if ov[i].DeviceID == "" {
				ov[i].DeviceID = d
			}
		}
		return ov, nil
	}
	c = runner.EnsureCoreEntities("plugin-androidtv", d, c)
	if d == "plugin-androidtv" {
		return c, nil
	}
	if !p.isKnownDevice(d) {
		return c, nil
	}

	c = upsertEntity(c, types.Entity{
		ID:        "power",
		DeviceID:  d,
		Domain:    entityswitch.Type,
		LocalName: "Power",
		Actions:   entityswitch.SupportedActions(),
	})
	c = upsertEntity(c, types.Entity{
		ID:        "media",
		DeviceID:  d,
		Domain:    domainMediaCast,
		LocalName: "Media",
		Actions:   []string{actionPlayURL, actionStop},
	})
	sort.Slice(c, func(i, j int) bool { return c[i].ID < c[j].ID })
	return c, nil
}

func (p *PluginAndroidtvPlugin) loadEntityOverride(deviceID string) ([]types.Entity, bool) {
	path := os.Getenv("PLUGIN_ANDROIDTV_DISCOVERY_FILE")
	if path == "" && p.dataDir != "" {
		path = filepath.Join(p.dataDir, "discovery_overrides.json")
	}
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cfg discoveryOverrides
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, false
	}
	entities, ok := cfg.Entities[deviceID]
	if !ok {
		return nil, false
	}
	out := make([]types.Entity, len(entities))
	copy(out, entities)
	return out, true
}

func (p *PluginAndroidtvPlugin) OnCommand(req types.Command, entity types.Entity) (types.Entity, error) {
	switch {
	case entity.ID == "power" && entity.Domain == entityswitch.Type:
		return p.handlePowerCommand(req, entity)
	case entity.ID == "media" && entity.Domain == domainMediaCast:
		return p.handleMediaCommand(req, entity)
	default:
		return entity, fmt.Errorf("unsupported entity command: %s (%s)", entity.ID, entity.Domain)
	}
}

func (p *PluginAndroidtvPlugin) handlePowerCommand(req types.Command, entity types.Entity) (types.Entity, error) {
	cmd, err := entityswitch.ParseCommand(req)
	if err != nil {
		return entity, err
	}
	ip, ok := p.lookupDeviceIP(entity.DeviceID)
	if !ok {
		if err := p.refreshDiscoveryCache(); err != nil {
			log.Printf("plugin-androidtv refresh before command failed: %v", err)
		}
		ip, ok = p.lookupDeviceIP(entity.DeviceID)
		if !ok {
			log.Printf("plugin-androidtv no device ip for %s; command accepted as best-effort no-op", entity.DeviceID)
		}
	}
	turnOn := cmd.Type == entityswitch.ActionTurnOn
	if ok {
		if err := p.commander.Power(context.Background(), ip, turnOn); err != nil {
			log.Printf("plugin-androidtv power command failed for %s (%s): %v", entity.DeviceID, ip, err)
		}
	} else {
		log.Printf("plugin-androidtv power command skipped for %s (no known ip)", entity.DeviceID)
	}
	store := entityswitch.Bind(&entity)
	if err := store.SetDesiredFromCommand(cmd); err != nil {
		return entity, err
	}
	entity.Data.SyncStatus = "pending"
	p.emitCommandAck(req, entity, map[string]any{"type": "power_ack", "power": cmd.Type})
	return entity, nil
}

func (p *PluginAndroidtvPlugin) handleMediaCommand(req types.Command, entity types.Entity) (types.Entity, error) {
	var cmd mediaCastCommand
	if err := json.Unmarshal(req.Payload, &cmd); err != nil {
		return entity, fmt.Errorf("invalid media command payload: %w", err)
	}
	if err := cmd.Validate(); err != nil {
		return entity, err
	}

	ip, ok := p.lookupDeviceIP(entity.DeviceID)
	if !ok {
		if err := p.refreshDiscoveryCache(); err != nil {
			log.Printf("plugin-androidtv refresh before media command failed: %v", err)
		}
		ip, ok = p.lookupDeviceIP(entity.DeviceID)
	}
	if !ok {
		return entity, fmt.Errorf("no known ip for device %s", entity.DeviceID)
	}

	switch cmd.Type {
	case actionPlayURL:
		go func(deviceID string, command mediaCastCommand) {
			if err := p.commander.PlayURL(context.Background(), ip, command.URL, command.ContentType); err != nil {
				log.Printf("plugin-androidtv async play_url failed for %s (%s): %v", deviceID, ip, err)
			}
		}(entity.DeviceID, cmd)
		entity.Data.Desired = mustJSON(map[string]any{
			"state":        "playing",
			"url":          cmd.URL,
			"content_type": cmd.ContentType,
		})
	case actionStop:
		go func(deviceID string) {
			if err := p.commander.Stop(context.Background(), ip); err != nil {
				log.Printf("plugin-androidtv async stop failed for %s (%s): %v", deviceID, ip, err)
			}
		}(entity.DeviceID)
		entity.Data.Desired = mustJSON(map[string]any{"state": "stopped"})
	}
	entity.Data.SyncStatus = "pending"
	p.emitCommandAck(req, entity, map[string]any{
		"type":   "media_ack",
		"action": cmd.Type,
		"url":    cmd.URL,
	})
	return entity, nil
}

func (p *PluginAndroidtvPlugin) OnEvent(evt types.Event, entity types.Entity) (types.Entity, error) {
	return entity, nil
}

func main() {
	r, err := runner.NewRunner(&PluginAndroidtvPlugin{})
	if err != nil {
		log.Fatal(err)
	}
	if err := r.Run(); err != nil {
		log.Fatal(err)
	}
}

func (p *PluginAndroidtvPlugin) isKnownDevice(deviceID string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.devices[deviceID]
	return ok
}

func (p *PluginAndroidtvPlugin) lookupDeviceIP(deviceID string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	ip, ok := p.deviceIP[deviceID]
	return ip, ok && ip != ""
}

func (p *PluginAndroidtvPlugin) refreshDiscoveryCache() error {
	discovered, err := discoverAndroidTVDevices(context.Background())
	if err != nil {
		return err
	}
	p.updateDiscoveredCache(discovered)
	return nil
}

func (p *PluginAndroidtvPlugin) updateDiscoveredCache(discovered []discoveredTV) map[string]discoveredTV {
	latest := make(map[string]discoveredTV, len(discovered))
	for _, d := range discovered {
		latest[d.Device.ID] = d
	}
	p.mu.Lock()
	p.devices = latest
	p.deviceIP = map[string]string{}
	for id, d := range latest {
		p.deviceIP[id] = d.Address
	}
	p.mu.Unlock()
	return latest
}

func upsertEntity(current []types.Entity, want types.Entity) []types.Entity {
	for i := range current {
		if current[i].ID == want.ID {
			existing := current[i]
			if want.DeviceID != "" {
				existing.DeviceID = want.DeviceID
			}
			if want.Domain != "" {
				existing.Domain = want.Domain
			}
			existing.Actions = append([]string(nil), want.Actions...)
			current[i] = existing
			return current
		}
	}
	return append(current, want)
}

func (p *PluginAndroidtvPlugin) emitCommandAck(req types.Command, entity types.Entity, payload map[string]any) {
	if p.eventSink == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = p.eventSink.EmitEvent(types.InboundEvent{
		DeviceID:      entity.DeviceID,
		EntityID:      entity.ID,
		CorrelationID: req.ID,
		Payload:       raw,
	})
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
