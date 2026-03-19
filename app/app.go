// plugin-androidtv connects to Android TV devices via Google Cast protocol
// and provides media control capabilities through SlideBolt.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	contract "github.com/slidebolt/sb-contract"
	domain "github.com/slidebolt/sb-domain"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	storage "github.com/slidebolt/sb-storage-sdk"

	translate "github.com/slidebolt/plugin-androidtv/internal/translate"
)

const PluginID = "plugin-androidtv"
const minDiscoveryTimeout = 3 * time.Second

type MediaPlayerState struct {
	Playing   bool   `json:"playing"`
	Volume    int    `json:"volume"`
	Muted     bool   `json:"muted"`
	Title     string `json:"title,omitempty"`
	AppName   string `json:"app_name,omitempty"`
	Connected bool   `json:"connected"`
	LastError string `json:"last_error,omitempty"`
}

type MediaPlay struct{}

func (MediaPlay) ActionName() string { return "media_play" }

type MediaPause struct{}

func (MediaPause) ActionName() string { return "media_pause" }

type MediaStop struct{}

func (MediaStop) ActionName() string { return "media_stop" }

type MediaSetVolume struct {
	Volume int `json:"volume"`
}

func (MediaSetVolume) ActionName() string { return "media_set_volume" }

type MediaMute struct{}

func (MediaMute) ActionName() string { return "media_mute" }

type MediaUnmute struct{}

func (MediaUnmute) ActionName() string { return "media_unmute" }

type MediaLaunchApp struct {
	AppName string `json:"app_name"`
}

func (MediaLaunchApp) ActionName() string { return "media_launch_app" }

func init() {
	domain.Register("media_player", MediaPlayerState{})
	domain.RegisterCommand("media_play", MediaPlay{})
	domain.RegisterCommand("media_pause", MediaPause{})
	domain.RegisterCommand("media_stop", MediaStop{})
	domain.RegisterCommand("media_set_volume", MediaSetVolume{})
	domain.RegisterCommand("media_mute", MediaMute{})
	domain.RegisterCommand("media_unmute", MediaUnmute{})
	domain.RegisterCommand("media_launch_app", MediaLaunchApp{})
}

type App struct {
	msg         messenger.Messenger
	store       storage.Storage
	cmd         *messenger.Commands
	subs        []messenger.Subscription
	discovery   *translate.Discovery
	ctx         context.Context
	cancel      context.CancelFunc
	seen        sync.Map
	discoveries []translate.AndroidTV
}

func New() *App { return &App{} }

func (a *App) Hello() contract.HelloResponse {
	return contract.HelloResponse{
		ID:              PluginID,
		Kind:            contract.KindPlugin,
		ContractVersion: contract.ContractVersion,
		DependsOn:       []string{"messenger", "storage"},
	}
}

func (a *App) OnStart(deps map[string]json.RawMessage) (json.RawMessage, error) {
	msg, err := messenger.Connect(deps)
	if err != nil {
		return nil, fmt.Errorf("connect messenger: %w", err)
	}
	a.msg = msg

	store, err := storage.Connect(deps)
	if err != nil {
		return nil, fmt.Errorf("connect storage: %w", err)
	}
	a.store = store

	a.cmd = messenger.NewCommands(msg, domain.LookupCommand)
	sub, err := a.cmd.Receive(PluginID+".>", a.handleCommand)
	if err != nil {
		return nil, fmt.Errorf("subscribe commands: %w", err)
	}
	a.subs = append(a.subs, sub)

	timeout := translate.DefaultTimeout
	if t := os.Getenv("ANDROIDTV_DISCOVERY_TIMEOUT_MS"); t != "" {
		if ms, err := strconv.Atoi(t); err == nil {
			timeout = time.Duration(ms) * time.Millisecond
		}
	}
	if timeout < minDiscoveryTimeout {
		timeout = minDiscoveryTimeout
	}
	disc, err := translate.NewDiscovery(timeout)
	if err != nil {
		return nil, fmt.Errorf("create discovery: %w", err)
	}
	a.discovery = disc

	ctx, cancel := context.WithCancel(context.Background())
	a.ctx = ctx
	a.cancel = cancel

	go func() {
		devices, err := translate.Discover(timeout)
		if err != nil {
			log.Printf("plugin-androidtv: discovery error: %v", err)
		} else {
			log.Printf("plugin-androidtv: initial probe found %d device(s)", len(devices))
			for _, dev := range devices {
				a.onDeviceFound(dev)
			}
		}
		disc.Listen(ctx, a.onDeviceFound)
	}()

	log.Println("plugin-androidtv: started")
	return nil, nil
}

func (a *App) OnShutdown() error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.discovery != nil {
		a.discovery.Stop()
	}
	for _, sub := range a.subs {
		sub.Unsubscribe()
	}
	if a.store != nil {
		a.store.Close()
	}
	if a.msg != nil {
		a.msg.Close()
	}
	return nil
}

func (a *App) onDeviceFound(dev translate.AndroidTV) {
	if !dev.IsTV {
		return
	}
	if _, already := a.seen.LoadOrStore(dev.DeviceID, struct{}{}); already {
		return
	}
	a.discoveries = append(a.discoveries, dev)
	entity := domain.Entity{
		ID:       dev.DeviceID,
		Plugin:   PluginID,
		DeviceID: dev.DeviceID,
		Type:     "media_player",
		Name:     dev.SourceName,
		Commands: []string{
			"media_play", "media_pause", "media_stop",
			"media_set_volume", "media_mute", "media_unmute",
			"media_launch_app",
		},
		State: MediaPlayerState{
			Connected: true,
			AppName:   dev.TXT["fn"],
		},
		Meta: map[string]json.RawMessage{
			"address": json.RawMessage(fmt.Sprintf(`"%s"`, dev.Address)),
			"port":    json.RawMessage(fmt.Sprintf(`%d`, dev.Port)),
		},
	}
	if err := a.store.Save(entity); err != nil {
		log.Printf("plugin-androidtv: failed to save entity %s: %v", dev.DeviceID, err)
		return
	}
	log.Printf("plugin-androidtv: registered %s at %s:%d", dev.SourceName, dev.Address, dev.Port)
}

func (a *App) handleCommand(addr messenger.Address, cmd any) {
	entityKey := domain.EntityKey{Plugin: addr.Plugin, DeviceID: addr.DeviceID, ID: addr.EntityID}
	raw, err := a.store.Get(entityKey)
	if err != nil {
		log.Printf("plugin-androidtv: command for unknown entity %s: %v", addr.Key(), err)
		return
	}

	var entity domain.Entity
	if err := json.Unmarshal(raw, &entity); err != nil {
		log.Printf("plugin-androidtv: failed to parse entity %s: %v", addr.Key(), err)
		return
	}

	switch c := cmd.(type) {
	case MediaPlay:
		a.updateMediaState(entity, func(s *MediaPlayerState) { s.Playing = true })
	case MediaPause:
		a.updateMediaState(entity, func(s *MediaPlayerState) { s.Playing = false })
	case MediaStop:
		a.updateMediaState(entity, func(s *MediaPlayerState) { s.Playing = false })
	case MediaSetVolume:
		a.updateMediaState(entity, func(s *MediaPlayerState) { s.Volume = c.Volume })
	case MediaMute:
		a.updateMediaState(entity, func(s *MediaPlayerState) { s.Muted = true })
	case MediaUnmute:
		a.updateMediaState(entity, func(s *MediaPlayerState) { s.Muted = false })
	case MediaLaunchApp:
		a.updateMediaState(entity, func(s *MediaPlayerState) { s.AppName = c.AppName })
	default:
		log.Printf("plugin-androidtv: unknown command %T for %s", cmd, addr.Key())
	}
}

func (a *App) updateMediaState(entity domain.Entity, mutate func(*MediaPlayerState)) {
	state, ok := entity.State.(MediaPlayerState)
	if !ok {
		if stateMap, ok := entity.State.(map[string]interface{}); ok {
			if playing, ok := stateMap["playing"].(bool); ok {
				state.Playing = playing
			}
			if volume, ok := stateMap["volume"].(float64); ok {
				state.Volume = int(volume)
			}
			if muted, ok := stateMap["muted"].(bool); ok {
				state.Muted = muted
			}
			if appName, ok := stateMap["app_name"].(string); ok {
				state.AppName = appName
			}
		}
	}
	mutate(&state)
	entity.State = state
	if err := a.store.Save(entity); err != nil {
		log.Printf("plugin-androidtv: failed to update state for %s: %v", entity.ID, err)
	}
}
