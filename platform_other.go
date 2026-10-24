//go:build !linux

package main

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Windows uses WindowsWindow.HiddenOnTaskbar; macOS accessory apps have no Dock entry.
func skipTaskbar(application.Window) {}

func setTrayTooltip(tray *application.SystemTray, title, sub string) {
	tray.SetTooltip(title + "\n" + sub)
}

func flatpakAutostart(bool) error { return errors.New("flatpak is Linux only") }

// Packages (.deb, .rpm) are Linux only; elsewhere the in-app updater swaps the binary.
func installedBy() string { return "" }
func installPackage(context.Context, string, string) error {
	return errors.New("packages are Linux only")
}
func relaunch() error { return errors.New("packages are Linux only") }
