package app_test

import (
	"encoding/json"
	"testing"

	atvapp "github.com/slidebolt/plugin-androidtv/app"
	domain "github.com/slidebolt/sb-domain"
	managersdk "github.com/slidebolt/sb-manager-sdk"
)

func TestStorageContract_MediaPlayerEntityRoundTrips(t *testing.T) {
	env := managersdk.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	entity := domain.Entity{
		ID:       "androidtv-123",
		Plugin:   atvapp.PluginID,
		DeviceID: "androidtv-123",
		Type:     "media_player",
		Name:     "Living Room TV",
		Commands: []string{"media_play", "media_pause", "media_set_volume"},
		State: domain.MediaPlayer{
			State:       "playing",
			VolumeLevel: 35,
			Source:      "Netflix",
		},
		Meta: map[string]json.RawMessage{
			"connected": json.RawMessage(`true`),
		},
	}
	if err := env.Storage().Save(entity); err != nil {
		t.Fatalf("save entity: %v", err)
	}

	raw, err := env.Storage().Get(domain.EntityKey{Plugin: atvapp.PluginID, DeviceID: "androidtv-123", ID: "androidtv-123"})
	if err != nil {
		t.Fatalf("get entity: %v", err)
	}
	var got domain.Entity
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	state, ok := got.State.(domain.MediaPlayer)
	if !ok {
		t.Fatalf("state type = %T", got.State)
	}
	if state.State != "playing" || state.VolumeLevel != 35 || state.Source != "Netflix" {
		t.Fatalf("state = %+v", state)
	}
}
