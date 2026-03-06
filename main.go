package main

import (
	"encoding/json"
	"log"

	runner "github.com/slidebolt/sdk-runner"
	"github.com/slidebolt/sdk-types"
)

func main() {
	r, err := runner.NewRunner(&PluginAdapter{})
	if err != nil {
		log.Fatal(err)
	}
	if err := r.Run(); err != nil {
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
