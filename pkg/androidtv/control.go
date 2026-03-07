package androidtv

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/vishen/go-chromecast/application"
)

// TVCommander defines the interface for controlling Android TV devices
type TVCommander interface {
	Power(ctx context.Context, ip string, on bool) error
	PlayURL(ctx context.Context, ip, url, contentType string) error
	Stop(ctx context.Context, ip string) error
}

type ShellTVCommander struct{}

func (c ShellTVCommander) Power(ctx context.Context, ip string, on bool) error {
	var errs []error
	if err := TryADBPower(ctx, ip, on); err == nil {
		return nil
	} else if err != ErrToolMissing {
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs[0]
}

func (c ShellTVCommander) PlayURL(ctx context.Context, ip, url, contentType string) error {
	if strings.TrimSpace(contentType) == "" {
		contentType = inferContentType(url)
	}
	log.Printf("plugin-androidtv: PlayURL start ip=%s url=%s content_type=%s", ip, url, contentType)

	// Prefer Android intent playback when adb is available; this tends to bring
	// playback to foreground on TVs more reliably than cast-only handoff.
	log.Printf("plugin-androidtv: PlayURL trying adb intent ip=%s", ip)
	if err := TryADBPlayURL(ctx, ip, url, contentType); err == nil {
		log.Printf("plugin-androidtv: PlayURL adb intent succeeded ip=%s", ip)
		return nil
	} else if err != ErrToolMissing {
		// If adb is present but intent launch fails, continue to cast fallback.
		log.Printf("plugin-androidtv: PlayURL adb intent failed ip=%s err=%v", ip, err)
	} else {
		log.Printf("plugin-androidtv: PlayURL adb not available, falling back to cast ip=%s", ip)
	}

	log.Printf("plugin-androidtv: PlayURL trying cast fallback ip=%s", ip)
	app, err := connectCastApp(ip)
	if err != nil {
		return err
	}
	// Chrome "Stop casting" semantics: explicitly close receiver/media session
	// before reconnecting and loading a new stream.
	if err := app.Close(true); err != nil {
		log.Printf("plugin-androidtv: PlayURL cast reset close(true) failed ip=%s err=%v", ip, err)
	} else {
		log.Printf("plugin-androidtv: PlayURL cast reset close(true) sent ip=%s", ip)
	}
	time.Sleep(250 * time.Millisecond)

	app, err = connectCastApp(ip)
	if err != nil {
		return err
	}
	defer app.Close(false)
	log.Printf("plugin-androidtv: PlayURL cast reconnect after reset ok ip=%s", ip)

	// Clear any stale/background media session before loading a new stream.
	// This helps when TV UI navigates away (e.g. HOME) but receiver session
	// remains attached and prevents visible takeover on subsequent loads.
	if err := app.Update(); err == nil {
		_, media, _ := app.Status()
		if media != nil {
			log.Printf("plugin-androidtv: PlayURL preflight stopping existing media ip=%s player_state=%s idle_reason=%s", ip, media.PlayerState, media.IdleReason)
			if err := app.StopMedia(); err != nil {
				log.Printf("plugin-androidtv: PlayURL preflight stop failed ip=%s err=%v", ip, err)
			} else {
				log.Printf("plugin-androidtv: PlayURL preflight stop succeeded ip=%s", ip)
				time.Sleep(200 * time.Millisecond)
			}
		}
	}

	if err := app.Load(url, 0, contentType, false, true, true); err != nil {
		log.Printf("plugin-androidtv: PlayURL cast load failed ip=%s err=%v", ip, err)
		return fmt.Errorf("cast load failed: %w", err)
	}
	// Keep post-load validation bounded.
	deadline := time.Now().Add(700 * time.Millisecond)
	for {
		if err := app.Update(); err == nil {
			_, media, _ := app.Status()
			if media != nil {
				if strings.EqualFold(media.PlayerState, "IDLE") && strings.EqualFold(media.IdleReason, "ERROR") {
					log.Printf("plugin-androidtv: PlayURL cast rejected ip=%s player_state=%s idle_reason=%s", ip, media.PlayerState, media.IdleReason)
					return fmt.Errorf("cast media load rejected by receiver")
				}
				log.Printf("plugin-androidtv: PlayURL cast fallback succeeded ip=%s player_state=%s idle_reason=%s", ip, media.PlayerState, media.IdleReason)
				return nil
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Printf("plugin-androidtv: PlayURL cast status unavailable after load ip=%s", ip)
	return fmt.Errorf("cast media status unavailable after load")
}

func (c ShellTVCommander) Stop(ctx context.Context, ip string) error {
	log.Printf("plugin-androidtv: Stop start ip=%s", ip)
	return withCastApp(ip, func(app *application.Application) error {
		if err := app.Update(); err != nil {
			log.Printf("plugin-androidtv: Stop update failed ip=%s err=%v", ip, err)
			return err
		}
		_, media, _ := app.Status()
		if media == nil {
			log.Printf("plugin-androidtv: Stop no active media ip=%s", ip)
			return nil
		}
		if err := app.StopMedia(); err != nil {
			log.Printf("plugin-androidtv: Stop stopMedia failed ip=%s err=%v", ip, err)
			return err
		}
		log.Printf("plugin-androidtv: Stop succeeded ip=%s", ip)
		return nil
	})
}

var ErrToolMissing = fmt.Errorf("tool missing")

func TryADBPower(ctx context.Context, ip string, on bool) error {
	if _, err := exec.LookPath("adb"); err != nil {
		return ErrToolMissing
	}
	target := ip + ":5555"
	if out, err := exec.CommandContext(ctx, "adb", "connect", target).CombinedOutput(); err != nil {
		return fmt.Errorf("adb connect failed: %w (%s)", err, string(out))
	}
	keycode := "KEYCODE_WAKEUP"
	if !on {
		keycode = "KEYCODE_SLEEP"
	}
	if out, err := exec.CommandContext(ctx, "adb", "-s", target, "shell", "input", "keyevent", keycode).CombinedOutput(); err != nil {
		return fmt.Errorf("adb keyevent failed: %w (%s)", err, string(out))
	}
	return nil
}

func TryADBPlayURL(ctx context.Context, ip, url, contentType string) error {
	if _, err := exec.LookPath("adb"); err != nil {
		return ErrToolMissing
	}
	target := ip + ":5555"
	if out, err := exec.CommandContext(ctx, "adb", "connect", target).CombinedOutput(); err != nil {
		return fmt.Errorf("adb connect failed: %w (%s)", err, string(out))
	}
	args := []string{
		"-s", target, "shell", "am", "start",
		"-a", "android.intent.action.VIEW",
		"-d", url,
	}
	if strings.TrimSpace(contentType) != "" {
		args = append(args, "-t", contentType)
	}
	if out, err := exec.CommandContext(ctx, "adb", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("adb am start failed: %w (%s)", err, string(out))
	}
	return nil
}

func withCastApp(ip string, fn func(app *application.Application) error) error {
	app, err := connectCastApp(ip)
	if err != nil {
		return err
	}
	defer app.Close(false)
	return fn(app)
}

func connectCastApp(ip string) (*application.Application, error) {
	app := application.NewApplication(
		application.WithCacheDisabled(true),
		application.WithConnectionRetries(2),
	)
	if err := app.Start(ip, 8009); err != nil {
		return nil, fmt.Errorf("connect to cast receiver failed: %w", err)
	}
	return app, nil
}

func inferContentType(url string) string {
	l := strings.ToLower(url)
	if strings.Contains(l, ".m3u8") {
		return "application/x-mpegURL"
	}
	switch strings.ToLower(path.Ext(strings.Split(url, "?")[0])) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mp3"
	}
	return "video/mp4"
}
