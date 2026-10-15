package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io/fs"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

//go:embed frontend
var assets embed.FS

func main() {
	dummyPath := flag.String("dummy", "dummy.json", "JSON file with fake events for development")
	flag.Parse()

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

	cfg := loadConfig()
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Width:           cfg.Width,
		Height:          cfg.Height,
		MinWidth:        320,
		MinHeight:       400,
		Frameless:       true, // still resizable by dragging the edges
		AlwaysOnTop:     true,
		Hidden:          true,
		HideOnEscape:    true,
		HideOnFocusLost: true,
		Windows:         application.WindowsWindow{HiddenOnTaskbar: true},
	})

	// Remember the size; debounced because resize events stream while dragging.
	var saveSize *time.Timer
	window.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		if saveSize != nil {
			saveSize.Stop()
		}
		saveSize = time.AfterFunc(500*time.Millisecond, func() {
			cfg.Width, cfg.Height = window.Size()
			if err := cfg.save(); err != nil {
				log.Printf("save config: %v", err)
			}
		})
	})

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.SystrayMacTemplate)
	} else {
		tray.SetIcon(trayIcon())
	}
	// ponytail: dummy is the only source until the Google connector lands.
	sources := []Source{&dummySource{path: *dummyPath, base: time.Now()}}

	var (
		mu     sync.Mutex
		events []Event
	)
	updateTray := func() {
		mu.Lock()
		next := nextEvent(events, time.Now())
		mu.Unlock()
		title, sub := "Žiadne ďalšie meetingy", ""
		if next != nil {
			title = next.Title
			sub = fmt.Sprintf(`%s · %s – %s · <span style="color:%s">●</span> %s`,
				untilText(next, time.Now()), next.Start.Format("15:04"), next.End.Format("15:04"),
				next.Color, html.EscapeString(next.Calendar))
			if runtime.GOOS == "darwin" {
				tray.SetLabel(next.Title + " · " + untilText(next, time.Now()))
			}
		}
		setTrayTooltip(tray, html.EscapeString(title), sub)
	}
	refresh := func() {
		now := time.Now()
		y, m, d := now.Date()
		today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		evs := fetchAll(context.Background(), sources, today, today.AddDate(0, 0, 2))
		mu.Lock()
		events = evs
		mu.Unlock()
		app.Event.Emit("events", map[string]any{"events": evs, "synced": now})
		updateTray()
	}
	app.Event.On("refresh", func(*application.CustomEvent) { go refresh() })
	app.Event.On("respond", func(ev *application.CustomEvent) {
		var req struct{ ID, Response string }
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil {
			return
		}
		mu.Lock()
		var src Source
		for _, e := range events {
			if e.ID == req.ID {
				src = e.src
			}
		}
		mu.Unlock()
		if src == nil {
			return
		}
		go func() {
			if err := src.Respond(context.Background(), req.ID, req.Response); err != nil {
				log.Printf("respond %s: %v", req.ID, err)
			}
			refresh()
		}()
	})
	go func() {
		for ; ; time.Sleep(time.Minute) {
			refresh()
		}
	}()
	go func() { // tray countdown ticks faster than the fetch
		for ; ; time.Sleep(10 * time.Second) {
			updateTray()
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

// untilText is the countdown shown in the tray: "o 12 min", "o 1 h 5 min", "prebieha do 17:57".
func untilText(e *Event, now time.Time) string {
	if !e.Start.After(now) {
		return "prebieha do " + e.End.Format("15:04")
	}
	m := int(e.Start.Sub(now).Round(time.Minute).Minutes())
	if m < 60 {
		return fmt.Sprintf("o %d min", m)
	}
	return fmt.Sprintf("o %d h %d min", m/60, m%60)
}
