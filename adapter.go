package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/slidebolt/plugin-androidtv/pkg/androidtv"
	entityswitch "github.com/slidebolt/sdk-entities/switch"
	runner "github.com/slidebolt/sdk-runner"
	"github.com/slidebolt/sdk-types"
)

type discoveryOverrides struct {
	Entities map[string][]types.Entity `json:"entities"`
}

// PluginAdapter implements the Slidebolt SDK Plugin interface for Android TV
type PluginAdapter struct {
	dataDir   string
	commander androidtv.TVCommander
	eventSink runner.EventSink
	rawStore  runner.RawStore
}

func (p *PluginAdapter) OnInitialize(config runner.Config, state types.Storage) (types.Manifest, types.Storage) {
	p.dataDir = config.DataDir
	p.eventSink = config.EventSink
	p.rawStore = config.RawStore
	if p.commander == nil {
		p.commander = androidtv.ShellTVCommander{}
	}
	schemas := append([]types.DomainDescriptor{}, types.CoreDomains()...)
	schemas = append(schemas, mediaCastDomainDescriptor())
	return types.Manifest{
		ID:      "plugin-androidtv",
		Name:    "Android TV Plugin",
		Version: "1.0.0",
		Schemas: schemas,
	}, state
}

func (p *PluginAdapter) OnReady() {}
func (p *PluginAdapter) WaitReady(ctx context.Context) error {
	return nil
}

func (p *PluginAdapter) OnShutdown()                    {}
func (p *PluginAdapter) OnHealthCheck() (string, error) { return "perfect", nil }
func (p *PluginAdapter) OnStorageUpdate(current types.Storage) (types.Storage, error) {
	return current, nil
}

func (p *PluginAdapter) OnDeviceCreate(dev types.Device) (types.Device, error) {
	return dev, nil
}
func (p *PluginAdapter) OnDeviceUpdate(dev types.Device) (types.Device, error) {
	return dev, nil
}
func (p *PluginAdapter) OnDeviceDelete(id string) error { return nil }
func (p *PluginAdapter) OnDevicesList(current []types.Device) ([]types.Device, error) {
	existing := map[string]types.Device{}
	for _, d := range current {
		existing[d.ID] = d
	}

	discovered, err := androidtv.DiscoverAndroidTVDevices(context.Background())
	if err != nil {
		log.Printf("plugin-androidtv discovery failed: %v", err)
	} else {
		for _, d := range discovered {
			if existingDev, ok := existing[d.Device.ID]; ok {
				existing[d.Device.ID] = runner.ReconcileDevice(existingDev, d.Device)
			} else {
				existing[d.Device.ID] = runner.ReconcileDevice(types.Device{}, d.Device)
			}
			p.storeDeviceIP(d.Device.ID, d.Address)
		}
	}

	out := make([]types.Device, 0, len(existing))
	for _, d := range existing {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return runner.EnsureCoreDevice("plugin-androidtv", out), nil
}
func (p *PluginAdapter) OnDeviceSearch(q types.SearchQuery, res []types.Device) ([]types.Device, error) {
	return res, nil
}

func (p *PluginAdapter) OnEntityCreate(e types.Entity) (types.Entity, error) { return e, nil }
func (p *PluginAdapter) OnEntityUpdate(e types.Entity) (types.Entity, error) { return e, nil }
func (p *PluginAdapter) OnEntityDelete(d, e string) error                    { return nil }
func (p *PluginAdapter) OnEntitiesList(d string, c []types.Entity) ([]types.Entity, error) {
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

	// Check if this is a physical device (has IP in store) or is in current device list
	hasIP := p.hasDeviceIP(d)
	if !hasIP {
		// Check in current entities list
		for _, ent := range c {
			if ent.DeviceID == d {
				hasIP = true
				break
			}
		}
	}
	if !hasIP {
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
	// Add availability entity for device status monitoring
	c = upsertEntity(c, types.Entity{
		ID:        "availability",
		DeviceID:  d,
		Domain:    "binary_sensor",
		LocalName: "Availability",
	})
	sort.Slice(c, func(i, j int) bool { return c[i].ID < c[j].ID })
	return c, nil
}

func (p *PluginAdapter) loadEntityOverride(deviceID string) ([]types.Entity, bool) {
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

func (p *PluginAdapter) OnCommand(req types.Command, entity types.Entity) (types.Entity, error) {
	switch {
	case entity.ID == "power" && entity.Domain == entityswitch.Type:
		return p.handlePowerCommand(req, entity)
	case entity.ID == "media" && entity.Domain == domainMediaCast:
		return p.handleMediaCommand(req, entity)
	default:
		return entity, fmt.Errorf("unsupported entity command: %s (%s)", entity.ID, entity.Domain)
	}
}

func (p *PluginAdapter) handlePowerCommand(req types.Command, entity types.Entity) (types.Entity, error) {
	cmd, err := entityswitch.ParseCommand(req)
	if err != nil {
		return p.setEntityError(entity, req, err)
	}

	ip, ok := p.lookupDeviceIP(entity.DeviceID)
	if !ok {
		return p.setEntityError(entity, req, fmt.Errorf("device offline: no IP address known"))
	}

	turnOn := cmd.Type == entityswitch.ActionTurnOn
	if err := p.commander.Power(context.Background(), ip, turnOn); err != nil {
		return p.setEntityError(entity, req, fmt.Errorf("power command failed: %w", err))
	}

	store := entityswitch.Bind(&entity)
	if err := store.SetDesiredFromCommand(cmd); err != nil {
		return entity, err
	}
	entity.Data.SyncStatus = types.SyncStatusSynced
	entity.Data.Reported = mustJSON(map[string]any{"power": turnOn})
	p.emitCommandAck(req, entity, map[string]any{"type": "power_ack", "power": cmd.Type})
	return entity, nil
}

func (p *PluginAdapter) handleMediaCommand(req types.Command, entity types.Entity) (types.Entity, error) {
	var cmd mediaCastCommand
	if err := json.Unmarshal(req.Payload, &cmd); err != nil {
		return p.setEntityError(entity, req, fmt.Errorf("invalid media command payload: %w", err))
	}
	if err := cmd.Validate(); err != nil {
		return p.setEntityError(entity, req, err)
	}

	ip, ok := p.lookupDeviceIP(entity.DeviceID)
	if !ok {
		return p.setEntityError(entity, req, fmt.Errorf("device offline: no IP address known"))
	}

	switch cmd.Type {
	case actionPlayURL:
		entity.Data.Desired = mustJSON(map[string]any{
			"state":        "playing",
			"url":          cmd.URL,
			"content_type": cmd.ContentType,
		})
		entity.Data.SyncStatus = types.SyncStatusPending
		p.emitCommandAck(req, entity, map[string]any{
			"type":   "media_ack",
			"action": cmd.Type,
			"url":    cmd.URL,
			"mode":   "async",
		})
		go func(deviceID, correlationID string, command mediaCastCommand) {
			if err := p.commander.PlayURL(context.Background(), ip, command.URL, command.ContentType); err != nil {
				p.emitAsyncError(deviceID, "media", correlationID, fmt.Errorf("play_url failed: %w", err))
				return
			}
			p.emitCommandAck(types.Command{ID: correlationID}, types.Entity{DeviceID: deviceID, ID: "media"}, map[string]any{
				"type":   "media_play_started",
				"action": command.Type,
				"url":    command.URL,
			})
		}(entity.DeviceID, req.ID, cmd)
		return entity, nil
	case actionStop:
		if err := p.commander.Stop(context.Background(), ip); err != nil {
			return p.setEntityError(entity, req, fmt.Errorf("stop failed: %w", err))
		}
		entity.Data.Desired = mustJSON(map[string]any{"state": "stopped"})
		entity.Data.Reported = mustJSON(map[string]any{"state": "stopped"})
		entity.Data.SyncStatus = types.SyncStatusSynced
		p.emitCommandAck(req, entity, map[string]any{
			"type":   "media_ack",
			"action": cmd.Type,
			"url":    cmd.URL,
		})
		return entity, nil
	}
	return p.setEntityError(entity, req, fmt.Errorf("unsupported media action: %s", cmd.Type))
}

func (p *PluginAdapter) OnEvent(evt types.Event, entity types.Entity) (types.Entity, error) {
	return entity, nil
}

// storeDeviceIP stores the device IP in RawStore for protocol-specific persistence
func (p *PluginAdapter) storeDeviceIP(deviceID string, ip string) {
	if p.rawStore == nil {
		return
	}
	data := map[string]string{"ip": ip}
	raw, _ := json.Marshal(data)
	_ = p.rawStore.WriteRawDevice(deviceID, raw)
}

// lookupDeviceIP retrieves the device IP from RawStore
func (p *PluginAdapter) lookupDeviceIP(deviceID string) (string, bool) {
	if p.rawStore == nil {
		return "", false
	}
	raw, err := p.rawStore.ReadRawDevice(deviceID)
	if err != nil {
		return "", false
	}
	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", false
	}
	ip, ok := data["ip"]
	return ip, ok && ip != ""
}

// hasDeviceIP checks if a device has an IP stored in RawStore
func (p *PluginAdapter) hasDeviceIP(deviceID string) bool {
	ip, ok := p.lookupDeviceIP(deviceID)
	return ok && ip != ""
}

func (p *PluginAdapter) emitCommandAck(req types.Command, entity types.Entity, payload map[string]any) {
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

// setEntityError sets the entity state to failed with error information
func (p *PluginAdapter) setEntityError(entity types.Entity, req types.Command, err error) (types.Entity, error) {
	entity.Data.SyncStatus = types.SyncStatusFailed
	entity.Data.Reported = mustJSON(map[string]any{
		"error": err.Error(),
	})
	p.emitCommandAck(req, entity, map[string]any{
		"type":  "error",
		"error": err.Error(),
	})
	return entity, err
}

// emitAsyncError emits an error event for async command failures.
func (p *PluginAdapter) emitAsyncError(deviceID, entityID, correlationID string, err error) {
	if p.eventSink == nil {
		return
	}
	raw, _ := json.Marshal(map[string]any{
		"type":   "async_error",
		"entity": entityID,
		"error":  err.Error(),
	})
	_ = p.eventSink.EmitEvent(types.InboundEvent{
		DeviceID:      deviceID,
		EntityID:      entityID,
		CorrelationID: correlationID,
		Payload:       raw,
	})
}
