//go:build !linux

package main

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Windows uses WindowsWindow.HiddenOnTaskbar; macOS accessory apps have no Dock entry.
func skipTaskbar(application.Window) {}

func setTrayTooltip(tray *application.SystemTray, title, sub string) {
	tray.SetTooltip(title + "\n" + sub)
}

func flatpakAutostart(bool) error { return errors.New("flatpak is Linux only") }
