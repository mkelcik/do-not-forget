package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

//go:embed frontend
var assets embed.FS

func main() {
	// Wayland ignores client window positioning, so the popup would land mid-screen.
	// XWayland honours it. ponytail: drop when Wails supports layer-shell on Wayland.
	if runtime.GOOS == "linux" && os.Getenv("GDK_BACKEND") == "" {
		os.Setenv("GDK_BACKEND", "x11")
	}
	frontend, _ := fs.Sub(assets, "frontend")
	app := application.New(application.Options{
		Name:   "do-not-forget",
		Assets: application.AssetOptions{Handler: application.BundledAssetFileServer(frontend)},
		Mac:    application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Width:           380,
		Height:          560,
		Frameless:       true,
		AlwaysOnTop:     true,
		Hidden:          true,
		DisableResize:   true,
		HideOnEscape:    true,
		HideOnFocusLost: true,
		Windows:         application.WindowsWindow{HiddenOnTaskbar: true},
	})

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.SystrayMacTemplate)
	} else {
		tray.SetIcon(trayIcon())
	}
	// ponytail: dummy next meeting, replaced by the calendar fetch
	next := struct {
		title, cal, color string
		start             time.Time
	}{"Standup", "Práca", "#0a84ff", time.Now().Add(12 * time.Minute)}
	go func() {
		for ; ; time.Sleep(10 * time.Second) {
			in := fmt.Sprintf("o %d min", int(time.Until(next.start).Minutes())+1)
			if runtime.GOOS == "darwin" {
				tray.SetLabel(next.title + " · " + in)
			}
			setTrayTooltip(tray, next.title, fmt.Sprintf(`%s · <span style="color:%s">●</span> %s`, in, next.color, next.cal))
		}
	}()

	menu := app.NewMenu()
	menu.Add("Koniec").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	tray.AttachWindow(window)
	// Wails positions before showing; an unmapped X11 window then gets placed by the WM.
	// Show first, then move.
	tray.OnClick(func() {
		if window.IsVisible() {
			window.Hide()
			return
		}
		window.Show().Focus()
		skipTaskbar(window)
		_ = tray.PositionWindow(window, 5)
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
