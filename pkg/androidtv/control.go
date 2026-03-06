package androidtv

import (
	"context"
	"fmt"
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
	return withCastApp(ip, func(app *application.Application) error {
		if err := app.Load(url, 0, contentType, false, true, true); err != nil {
			return fmt.Errorf("cast load failed: %w", err)
		}
		time.Sleep(2 * time.Second)
		if err := app.Update(); err != nil {
			return fmt.Errorf("cast update failed: %w", err)
		}
		_, media, _ := app.Status()
		if media == nil {
			return fmt.Errorf("cast media status missing after load")
		}
		if strings.EqualFold(media.PlayerState, "IDLE") && strings.EqualFold(media.IdleReason, "ERROR") {
			return fmt.Errorf("cast media load rejected by receiver")
		}
		return nil
	})
}

func (c ShellTVCommander) Stop(ctx context.Context, ip string) error {
	return withCastApp(ip, func(app *application.Application) error {
		if err := app.Update(); err != nil {
			return err
		}
		_, media, _ := app.Status()
		if media == nil {
			return nil
		}
		return app.StopMedia()
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

func withCastApp(ip string, fn func(app *application.Application) error) error {
	app := application.NewApplication(
		application.WithCacheDisabled(true),
		application.WithConnectionRetries(2),
	)
	if err := app.Start(ip, 8009); err != nil {
		return fmt.Errorf("connect to cast receiver failed: %w", err)
	}
	defer app.Close(false)
	return fn(app)
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
