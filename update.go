package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
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

// startUpdater checks GitHub Releases at startup and every 6 hours and announces a new
// version with a notification; clicking it downloads, verifies (SHA256SUMS) and restarts.
// Dev builds never check, so `go run` can't get replaced by a release binary.
// ponytail: no release signing yet; set updater.Config.PublicKey once CI signs releases.
func startUpdater(app *application.App, notifier *notifications.NotificationService) {
	if version == "dev" {
		return
	}
	gh, err := github.New(github.Config{Repository: repo, ChecksumAsset: "SHA256SUMS"})
	if err == nil {
		err = app.Updater.Init(updater.Config{CurrentVersion: version, Providers: []updater.Provider{gh}})
	}
	if err != nil {
		log.Printf("updater: %v", err)
		return
	}
	_ = notifier.RegisterNotificationCategory(notifications.NotificationCategory{
		ID:      "update",
		Actions: []notifications.NotificationAction{{ID: "update", Title: "Aktualizovať"}},
	})

	announced := ""
	go func() {
		for time.Sleep(time.Minute); ; time.Sleep(6 * time.Hour) {
			rel, err := app.Updater.Check(context.Background())
			if err != nil {
				log.Printf("update check: %v", err)
				continue
			}
			if rel == nil || rel.Version == announced {
				continue
			}
			announced = rel.Version
			body := "Klikni pre aktualizáciu a reštart."
			if !canSelfUpdate() {
				body = "Klikni pre stiahnutie novej verzie."
			}
			_ = notifier.SendNotificationWithActions(notifications.NotificationOptions{
				ID:         "update",
				Title:      "Je dostupná nová verzia " + rel.Version,
				Body:       body,
				CategoryID: "update",
				Data:       map[string]any{"kind": "update"},
			})
		}
	}()
}

// installUpdate runs when the update notification is clicked.
func installUpdate(app *application.App, notifier *notifications.NotificationService) {
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
	if err != nil {
		log.Printf("update: %v", err)
		_ = notifier.SendNotification(notifications.NotificationOptions{
			ID: "update-failed", Title: "Aktualizácia zlyhala", Body: err.Error(),
		})
	}
}

// canSelfUpdate reports whether we may replace our own binary (not owned by root/a package).
func canSelfUpdate() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	f, err := os.CreateTemp(filepath.Dir(exe), ".update-probe-*")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}
