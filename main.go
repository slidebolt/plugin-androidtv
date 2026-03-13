package main

import (
	"encoding/json"
	"log"

	runner "github.com/slidebolt/sdk-runner"
	"github.com/slidebolt/sdk-types"
)

func main() {
	if err := runner.RunCLI(func() runner.Plugin { return &PluginAdapter{} }); err != nil {
		log.Fatal(err)
	}
}

// Helper functions shared between adapter and tests
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

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

func reconcileDevice(existing types.Device, discovered types.Device) types.Device {
	if discovered.SourceID == "" {
		discovered.SourceID = discovered.ID
	}
	if existing.ID == "" {
		discovered.LocalName = ""
		if discovered.Labels == nil {
			discovered.Labels = make(map[string][]string)
		}
		return discovered
	}

	result := existing
	result.SourceID = discovered.SourceID
	result.SourceName = discovered.SourceName
	if result.Labels == nil {
		result.Labels = make(map[string][]string)
	}
	for k, v := range discovered.Labels {
		if _, ok := result.Labels[k]; !ok {
			result.Labels[k] = v
		}
	}
	return result
}

func ensureCoreDevice(pluginID string, current []types.Device) []types.Device {
	coreID := types.CoreDeviceID(pluginID)
	for _, d := range current {
		if d.ID == coreID {
			return current
		}
	}
	return append(current, reconcileDevice(types.Device{}, types.Device{
		ID:         coreID,
		SourceID:   coreID,
		SourceName: pluginID,
	}))
}

func ensureCoreEntities(pluginID, deviceID string, current []types.Entity) []types.Entity {
	if deviceID != types.CoreDeviceID(pluginID) {
		return current
	}
	for _, need := range types.CoreEntities(pluginID) {
		found := false
		for _, e := range current {
			if e.ID == need.ID {
				found = true
				break
			}
		}
		if !found {
			current = append(current, need)
		}
	}
	return current
}
