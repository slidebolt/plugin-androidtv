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
			"media_set_volume", "media_mute",
			"media_select_source",
		},
		State: domain.MediaPlayer{
			State:  "idle",
			Source: dev.TXT["fn"],
		},
		Meta: map[string]json.RawMessage{
			"address":   json.RawMessage(fmt.Sprintf(`"%s"`, dev.Address)),
			"port":      json.RawMessage(fmt.Sprintf(`%d`, dev.Port)),
			"connected": json.RawMessage(`true`),
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
	case domain.MediaPlay:
		a.updateMediaState(entity, func(s *domain.MediaPlayer) { s.State = "playing" })
	case domain.MediaPause:
		a.updateMediaState(entity, func(s *domain.MediaPlayer) { s.State = "paused" })
	case domain.MediaStop:
		a.updateMediaState(entity, func(s *domain.MediaPlayer) { s.State = "idle" })
	case domain.MediaSetVolume:
		a.updateMediaState(entity, func(s *domain.MediaPlayer) { s.VolumeLevel = c.VolumeLevel })
	case domain.MediaMute:
		a.updateMediaState(entity, func(s *domain.MediaPlayer) { s.IsVolumeMuted = c.Mute })
	case domain.MediaSelectSource:
		a.updateMediaState(entity, func(s *domain.MediaPlayer) { s.Source = c.Source })
	default:
		log.Printf("plugin-androidtv: unknown command %T for %s", cmd, addr.Key())
	}
}

func (a *App) updateMediaState(entity domain.Entity, mutate func(*domain.MediaPlayer)) {
	state, _ := entity.State.(domain.MediaPlayer)
	mutate(&state)
	entity.State = state
	if err := a.store.Save(entity); err != nil {
		log.Printf("plugin-androidtv: failed to update state for %s: %v", entity.ID, err)
	}
}
