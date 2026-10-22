package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// version is stamped by release builds: go build -ldflags "-X main.version=1.2.3".
var version = "dev"

const (
	repo        = "mkelcik/do-not-forget"
	releasesURL = "https://github.com/" + repo + "/releases/latest"
)

// startUpdater checks GitHub Releases a minute after start and every 6 hours. A new version
// goes to available (Settings › About offers it, so it doesn't hinge on notifications being
// allowed) and is announced with a notification whose button downloads, verifies (SHA256SUMS
// + Ed25519 signature, see signing.go) and restarts. The notification repeats every check
// until updated, and waits out a mute. Dev builds never check, so `go run` can't get replaced
// by a release binary. Flatpak updates the Flatpak.
func startUpdater(app *application.App, notifier *notifications.NotificationService, muted func() bool, available func(version string)) {
	if version == "dev" || flatpakID != "" {
		return
	}
	gh, err := github.New(github.Config{Repository: repo, ChecksumAsset: "SHA256SUMS"})
	if err == nil {
		err = app.Updater.Init(updater.Config{
			CurrentVersion: version,
			Providers:      []updater.Provider{signedProvider{Provider: gh, client: &http.Client{Timeout: 30 * time.Second}}},
			PublicKey:      updatePublicKey,
		})
	}
	if err != nil {
		log.Printf("updater: %v", err)
		return
	}
	go func() {
		for time.Sleep(time.Minute); ; time.Sleep(6 * time.Hour) {
			switch app.Updater.State() {
			case updater.StateDownloading, updater.StateVerifying, updater.StateInstalling, updater.StateReady:
				continue // an install is in flight; a Check now would clobber its state
			default:
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute) // a stalled request must not stop the loop
			rel, err := app.Updater.Check(ctx)
			cancel()
			if err != nil {
				log.Printf("update check: %v", err)
				continue
			}
			if rel == nil {
				continue
			}
			available(rel.Version)
			if muted() {
				continue
			}
			body := T("update.body_install")
			if !canSelfUpdate() {
				body = T("update.body_download")
			}
			if err := notifier.SendNotificationWithActions(notifications.NotificationOptions{
				ID:         "update",
				Title:      T("update.title", "version", rel.Version),
				Body:       body,
				CategoryID: "update",
				Data:       map[string]any{"kind": "update"},
			}); err != nil {
				log.Printf("update notification: %v", err)
			}
		}
	}()
}

var installing sync.Mutex

// installUpdate runs when the update notification's button is pressed.
func installUpdate(app *application.App, notifier *notifications.NotificationService) {
	if !installing.TryLock() {
		return // a second click while the first install is still running
	}
	defer installing.Unlock()
	if !canSelfUpdate() {
		// Installed by a package manager (e.g. .deb in /usr/bin): send them to the release page.
		_ = app.Browser.OpenURL(releasesURL)
		return
	}
	ctx := context.Background()
	err := app.Updater.DownloadAndInstall(ctx)
	if err == nil {
		err = app.Updater.Restart(ctx) // exits this process on success
	}
	if err != nil && !errors.Is(err, updater.ErrDownloadInProgress) {
		log.Printf("update: %v", err) // the full chain is for the log, not the toast
		_ = notifier.SendNotification(notifications.NotificationOptions{
			ID: "update-failed", Title: T("update.failed_title"),
			Body: T("update.failed_body"),
		})
	}
}

// canSelfUpdate reports whether we may replace our own binary (not owned by root/a package).
// Inside an AppImage the binary sits on a read-only mount; the updater swaps $APPIMAGE itself.
func canSelfUpdate() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if img, mount := os.Getenv("APPIMAGE"), os.Getenv("APPDIR"); img != "" && mount != "" &&
		strings.HasPrefix(exe, filepath.Clean(mount)+string(filepath.Separator)) {
		exe = img
	}
	f, err := os.CreateTemp(filepath.Dir(exe), ".update-probe-*")
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(f.Name()) //nolint:gosec // G703 false positive: the name comes from os.CreateTemp
	return true
}
