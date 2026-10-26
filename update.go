package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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
//
// It returns check, for Settings' "Check for updates": the newer version or "", without a
// notification since Settings shows the result. nil when there's no updater.
func startUpdater(app *application.App, notifier *notifications.NotificationService, muted func() bool, available func(version string)) (check func() (string, error)) {
	if version == "dev" || flatpakID != "" {
		return nil
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
		return nil
	}
	var checking sync.Mutex // the periodic check and the button take turns
	check = func() (string, error) {
		checking.Lock()
		defer checking.Unlock()
		switch app.Updater.State() {
		case updater.StateDownloading, updater.StateVerifying, updater.StateInstalling, updater.StateReady:
			return "", errUpdating // a Check now would clobber the install's state
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute) // a stalled request must not hang
		defer cancel()
		rel, err := app.Updater.Check(ctx)
		if err != nil || rel == nil {
			return "", err
		}
		offered.Store(rel.Version)
		available(rel.Version)
		return rel.Version, nil
	}
	go func() {
		for time.Sleep(time.Minute); ; time.Sleep(6 * time.Hour) {
			v, err := check()
			if err != nil {
				log.Printf("update check: %v", err)
				continue
			}
			if v == "" || muted() {
				continue
			}
			body := T("update.body_install")
			if !canSelfUpdate() {
				body = T("update.body_download")
				if installedBy() != "" {
					body = T("update.body_package")
				}
			}
			if err := notifier.SendNotificationWithActions(notifications.NotificationOptions{
				ID:         "update",
				Title:      T("update.title", "version", v),
				Body:       body,
				CategoryID: "update",
				Data:       map[string]any{"kind": "update"},
			}); err != nil {
				log.Printf("update notification: %v", err)
			}
		}
	}()
	return check
}

// errUpdating: an update is being installed, so there's nothing to check.
var errUpdating = errors.New("an update is being installed")

// checkResult is how a check from Settings ended, as the page shows it: found (the Update
// button appears), latest, updating or failed.
func checkResult(newer string, err error) string {
	switch {
	case errors.Is(err, errUpdating):
		return "updating"
	case err != nil:
		return "failed"
	case newer != "":
		return "found"
	}
	return "latest"
}

var installing sync.Mutex

// installUpdate runs when the update notification's button is pressed.
func installUpdate(app *application.App, notifier *notifications.NotificationService) {
	if !installing.TryLock() {
		return // a second click while the first install is still running
	}
	defer installing.Unlock()
	ctx := context.Background()
	var err error
	v, _ := offered.Load().(string)
	switch kind := installedBy(); {
	case canSelfUpdate():
		err = app.Updater.DownloadAndInstall(ctx)
		if err == nil {
			err = app.Updater.Restart(ctx) // exits this process on success
		}
	case kind != "" && v != "":
		// A .deb or .rpm: the package manager installs it, after an admin password.
		var path string
		base := "https://github.com/" + repo + "/releases/download/v" + strings.TrimPrefix(v, "v") + "/"
		path, err = downloadPackage(ctx, &http.Client{Timeout: 10 * time.Minute}, base, kind, v)
		if err == nil {
			err = installPackage(ctx, kind, path)
			_ = os.Remove(path)
		}
		if errors.Is(err, errCancelled) {
			return
		}
		if err == nil {
			err = relaunch()
		}
		if err == nil {
			app.Quit()
			return
		}
	default:
		// Installed some other way (e.g. AUR): send them to the release page.
		_ = app.Browser.OpenURL(releasesURL)
		return
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

// offered is the version the last check found; installUpdate needs it for a package.
var offered atomic.Value

// errCancelled is a pkexec password prompt the user dismissed: nothing to report.
var errCancelled = errors.New("update cancelled")

// packageAsset is the name scripts/dist.sh gives the release's .deb or .rpm for goarch.
func packageAsset(kind, version, goarch string) string {
	version = strings.ReplaceAll(strings.TrimPrefix(version, "v"), "-", "~") // Debian/RPM prerelease order
	if kind == "rpm" {
		goarch = map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	}
	return fmt.Sprintf("do-not-forget_%s_%s.%s", version, goarch, kind)
}

// downloadPackage fetches the release's package into a temporary file and checks it like an
// archive: its <asset>.manifest.sig must cover this version, platform, name and SHA-256, so
// only a signed package of exactly that release reaches the root installer.
func downloadPackage(ctx context.Context, client *http.Client, base, kind, version string) (string, error) {
	asset := packageAsset(kind, version, runtime.GOARCH)
	sig, err := signedProvider{client: client}.fetchSignature(ctx, base+asset+".manifest.sig")
	if err != nil {
		return "", fmt.Errorf("package manifest signature: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+asset, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", asset, resp.Status)
	}
	f, err := os.CreateTemp("", "do-not-forget-*."+kind)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = verifyPackage(version, asset, h.Sum(nil), sig)
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func verifyPackage(version, asset string, sum, sig []byte) error {
	key, err := publicKey()
	if err != nil {
		return err
	}
	if !ed25519.Verify(key, updateManifest(strings.TrimPrefix(version, "v"), runtime.GOOS, runtime.GOARCH, asset, sum), sig) {
		return fmt.Errorf("package manifest signature doesn't match version %s of %s", version, asset)
	}
	return nil
}
