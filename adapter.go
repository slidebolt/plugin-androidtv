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
	pctx      runner.PluginContext
}

func (p *PluginAdapter) Initialize(ctx runner.PluginContext) (types.Manifest, error) {
	p.pctx = ctx
	p.dataDir = os.Getenv("PLUGIN_DATA_DIR")
	if p.dataDir == "" {
		p.dataDir = "."
	}
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
	}, nil
}

func (p *PluginAdapter) Start(ctx context.Context) error { return nil }
func (p *PluginAdapter) Stop() error                     { return nil }

func (p *PluginAdapter) OnReset() error {
	if p.pctx.Registry == nil {
		return nil
	}
	for _, dev := range p.pctx.Registry.LoadDevices() {
		_ = p.pctx.Registry.DeleteDevice(dev.ID)
	}
	return p.pctx.Registry.DeleteState()
}
func (p *PluginAdapter) OnHealthCheck() (string, error) { return "perfect", nil }

func (p *PluginAdapter) OnDeviceCreate(dev types.Device) (types.Device, error) {
	return dev, nil
}
func (p *PluginAdapter) OnDeviceUpdate(dev types.Device) (types.Device, error) {
	return dev, nil
}
func (p *PluginAdapter) OnDeviceDelete(id string) error { return nil }
func (p *PluginAdapter) OnDeviceDiscover(current []types.Device) ([]types.Device, error) {
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
				existing[d.Device.ID] = reconcileDevice(existingDev, d.Device)
			} else {
				existing[d.Device.ID] = reconcileDevice(types.Device{}, d.Device)
			}
			p.storeDeviceIP(d.Device.ID, d.Address)
		}
	}

	out := make([]types.Device, 0, len(existing))
	for _, d := range existing {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return ensureCoreDevice("plugin-androidtv", out), nil
}
func (p *PluginAdapter) OnDeviceSearch(q types.SearchQuery, res []types.Device) ([]types.Device, error) {
	return res, nil
}

func (p *PluginAdapter) OnEntityCreate(e types.Entity) (types.Entity, error) { return e, nil }
func (p *PluginAdapter) OnEntityUpdate(e types.Entity) (types.Entity, error) { return e, nil }
func (p *PluginAdapter) OnEntityDelete(d, e string) error                    { return nil }
func (p *PluginAdapter) OnEntityDiscover(d string, c []types.Entity) ([]types.Entity, error) {
	if ov, ok := p.loadEntityOverride(d); ok {
		for i := range ov {
			if ov[i].DeviceID == "" {
				ov[i].DeviceID = d
			}
		}
		return ov, nil
	}
	c = ensureCoreEntities("plugin-androidtv", d, c)
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

func (p *PluginAdapter) OnCommand(req types.Command, entity types.Entity) error {
	var (
		updated types.Entity
		err     error
	)
	switch {
	case entity.ID == "power" && entity.Domain == entityswitch.Type:
		updated, err = p.handlePowerCommand(req, entity)
	case entity.ID == "media" && entity.Domain == domainMediaCast:
		updated, err = p.handleMediaCommand(req, entity)
	default:
		return fmt.Errorf("unsupported entity command: %s (%s)", entity.ID, entity.Domain)
	}
	if err != nil {
		return err
	}
	if p.pctx.Registry != nil && updated.ID != "" && updated.DeviceID != "" {
		_ = p.pctx.Registry.SaveEntity(updated)
	}
	return nil
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

// storeDeviceIP stores the device IP in local plugin state for protocol-specific persistence.
func (p *PluginAdapter) storeDeviceIP(deviceID string, ip string) {
	if p.pctx.Registry == nil || ip == "" {
		return
	}
	s := p.loadPersistentState()
	if s.DeviceIPs == nil {
		s.DeviceIPs = map[string]string{}
	}
	if s.DeviceIPs[deviceID] == ip {
		return
	}
	s.DeviceIPs[deviceID] = ip
	p.savePersistentState(s)
}

// lookupDeviceIP retrieves the device IP from local plugin state.
func (p *PluginAdapter) lookupDeviceIP(deviceID string) (string, bool) {
	if p.pctx.Registry == nil {
		return "", false
	}
	s := p.loadPersistentState()
	ip, ok := s.DeviceIPs[deviceID]
	return ip, ok && ip != ""
}

// hasDeviceIP checks if a device has an IP in local plugin state.
func (p *PluginAdapter) hasDeviceIP(deviceID string) bool {
	ip, ok := p.lookupDeviceIP(deviceID)
	return ok && ip != ""
}

func (p *PluginAdapter) emitCommandAck(req types.Command, entity types.Entity, payload map[string]any) {
	if p.pctx.Events == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = p.pctx.Events.PublishEvent(types.InboundEvent{
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
	if p.pctx.Events == nil {
		return
	}
	raw, _ := json.Marshal(map[string]any{
		"type":   "async_error",
		"entity": entityID,
		"error":  err.Error(),
	})
	_ = p.pctx.Events.PublishEvent(types.InboundEvent{
		DeviceID:      deviceID,
		EntityID:      entityID,
		CorrelationID: correlationID,
		Payload:       raw,
	})
}

type pluginState struct {
	DeviceIPs map[string]string `json:"device_ips,omitempty"`
}

func (p *PluginAdapter) loadPersistentState() pluginState {
	if p.pctx.Registry == nil {
		return pluginState{}
	}
	raw, ok := p.pctx.Registry.LoadState()
	if !ok || len(raw.Data) == 0 {
		return pluginState{DeviceIPs: map[string]string{}}
	}
	var s pluginState
	if err := json.Unmarshal(raw.Data, &s); err != nil {
		return pluginState{DeviceIPs: map[string]string{}}
	}
	if s.DeviceIPs == nil {
		s.DeviceIPs = map[string]string{}
	}
	return s
}

func (p *PluginAdapter) savePersistentState(s pluginState) {
	if p.pctx.Registry == nil {
		return
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = p.pctx.Registry.SaveState(types.Storage{Data: raw})
}
