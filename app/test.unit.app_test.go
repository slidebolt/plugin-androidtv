package app

import (
	"encoding/json"
	"testing"
	"time"

	domain "github.com/slidebolt/sb-domain"
	managersdk "github.com/slidebolt/sb-manager-sdk"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	storage "github.com/slidebolt/sb-storage-sdk"
)

func env(t *testing.T) (*managersdk.TestEnv, storage.Storage, *messenger.Commands) {
	t.Helper()
	e := managersdk.NewTestEnv(t)
	e.Start("messenger")
	e.Start("storage")
	cmds := messenger.NewCommands(e.Messenger(), domain.LookupCommand)
	return e, e.Storage(), cmds
}

func saveEntity(t *testing.T, store storage.Storage, plugin, device, id, typ, name string, state any) domain.Entity {
	t.Helper()
	e := domain.Entity{ID: id, Plugin: plugin, DeviceID: device, Type: typ, Name: name, State: state}
	if err := store.Save(e); err != nil {
		t.Fatalf("save %s: %v", id, err)
	}
	return e
}

func getEntity(t *testing.T, store storage.Storage, plugin, device, id string) domain.Entity {
	t.Helper()
	raw, err := store.Get(domain.EntityKey{Plugin: plugin, DeviceID: device, ID: id})
	if err != nil {
		t.Fatalf("get %s.%s.%s: %v", plugin, device, id, err)
	}
	var entity domain.Entity
	if err := json.Unmarshal(raw, &entity); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return entity
}

func queryByType(t *testing.T, store storage.Storage, typ string) []storage.Entry {
	t.Helper()
	entries, err := store.Query(storage.Query{Where: []storage.Filter{{Field: "type", Op: storage.Eq, Value: typ}}})
	if err != nil {
		t.Fatalf("query type=%s: %v", typ, err)
	}
	return entries
}

func sendAndReceive(t *testing.T, cmds *messenger.Commands, entity domain.Entity, cmd any, pattern string) any {
	t.Helper()
	done := make(chan any, 1)
	cmds.Receive(pattern, func(addr messenger.Address, c any) { done <- c })
	if err := cmds.Send(entity, cmd.(messenger.Action)); err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case got := <-done:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for command")
		return nil
	}
}

func TestCustomMediaPlayerSaveGetHydrate(t *testing.T) {
	_, store, _ := env(t)
	saveEntity(t, store, PluginID, "androidtv-test-123", "androidtv-test-123", "media_player", "Living Room TV",
		MediaPlayerState{Playing: false, Volume: 50, Muted: false, AppName: "Netflix"})
	got := getEntity(t, store, PluginID, "androidtv-test-123", "androidtv-test-123")
	state, ok := got.State.(MediaPlayerState)
	if !ok {
		t.Fatalf("state type: got %T, want MediaPlayerState", got.State)
	}
	if state.Playing || state.Volume != 50 || state.Muted || state.AppName != "Netflix" {
		t.Errorf("state: %+v", state)
	}
}

func TestCustomMediaPlayerQueryByType(t *testing.T) {
	_, store, _ := env(t)
	saveEntity(t, store, PluginID, "tv1", "tv1", "media_player", "TV 1", MediaPlayerState{Volume: 30})
	saveEntity(t, store, PluginID, "tv2", "tv2", "media_player", "TV 2", MediaPlayerState{Volume: 70})
	saveEntity(t, store, "test", "dev1", "light001", "light", "Light", domain.Light{Power: true})
	entries := queryByType(t, store, "media_player")
	if len(entries) != 2 {
		t.Fatalf("media_players: got %d, want 2", len(entries))
	}
}

func TestMediaSetVolume(t *testing.T) {
	_, store, cmds := env(t)
	saveEntity(t, store, PluginID, "tv1", "tv1", "media_player", "Test TV", MediaPlayerState{Volume: 50})
	entity := domain.Entity{ID: "tv1", Plugin: PluginID, DeviceID: "tv1", Type: "media_player"}
	got := sendAndReceive(t, cmds, entity, MediaSetVolume{Volume: 75}, PluginID+".>")
	cmd, ok := got.(MediaSetVolume)
	if !ok {
		t.Fatalf("type: got %T, want MediaSetVolume", got)
	}
	if cmd.Volume != 75 {
		t.Errorf("volume: got %d, want 75", cmd.Volume)
	}
}

func TestMediaLaunchApp(t *testing.T) {
	_, store, cmds := env(t)
	saveEntity(t, store, PluginID, "tv1", "tv1", "media_player", "Test TV", MediaPlayerState{AppName: ""})
	entity := domain.Entity{ID: "tv1", Plugin: PluginID, DeviceID: "tv1", Type: "media_player"}
	got := sendAndReceive(t, cmds, entity, MediaLaunchApp{AppName: "YouTube"}, PluginID+".>")
	cmd, ok := got.(MediaLaunchApp)
	if !ok {
		t.Fatalf("type: got %T, want MediaLaunchApp", got)
	}
	if cmd.AppName != "YouTube" {
		t.Errorf("app_name: got %s, want YouTube", cmd.AppName)
	}
}

func TestPluginHello(t *testing.T) {
	hello := New().Hello()
	if hello.ID != PluginID {
		t.Errorf("ID: got %q, want %q", hello.ID, PluginID)
	}
}
