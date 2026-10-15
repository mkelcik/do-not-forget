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
	wevents "github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

//go:embed frontend
var assets embed.FS

func main() {
	dummyPath := flag.String("dummy", "dummy.json", "JSON file with fake events for development")
	previewAlert := flag.Bool("preview-alert", false, "show the fullscreen meeting alert for the next event right after start")
	flag.Parse()

	// Wayland ignores client window positioning, so the popup would land mid-screen.
	// XWayland honours it. ponytail: drop when Wails supports layer-shell on Wayland.
	if runtime.GOOS == "linux" && os.Getenv("GDK_BACKEND") == "" {
		os.Setenv("GDK_BACKEND", "x11")
	}
	frontend, _ := fs.Sub(assets, "frontend")
	notifier := notifications.New()
	app := application.New(application.Options{
		Name:     "do-not-forget",
		Services: []application.Service{application.NewService(notifier)},
		Assets:   application.AssetOptions{Handler: application.BundledAssetFileServer(frontend)},
		Mac:      application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	cfg := loadConfig()
	var (
		mu         sync.Mutex // guards cfg and events
		events     []Event
		showWindow func()
	)
	// Change and write under one lock so concurrent saves can't interleave or lose a field.
	saveConfig := func(change func(*Config)) {
		mu.Lock()
		defer mu.Unlock()
		change(&cfg)
		if err := cfg.save(); err != nil {
			log.Printf("save config: %v", err)
		}
	}
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
	window.OnWindowEvent(wevents.Common.WindowDidResize, func(*application.WindowEvent) {
		if saveSize != nil {
			saveSize.Stop()
		}
		saveSize = time.AfterFunc(500*time.Millisecond, func() {
			w, h := window.Size()
			saveConfig(func(c *Config) { c.Width, c.Height = w, h })
		})
	})

	tray := app.SystemTray.New()
	// ponytail: dummy is the only source until the Google connector lands.
	sources := []Source{&dummySource{path: *dummyPath, base: time.Now()}}

	updateTray := func() {
		mu.Lock()
		next := nextEvent(events, time.Now())
		muted := muteLabel(cfg.MutedUntil, time.Now())
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
		if muted != "" {
			sub += "<br>🔕 " + muted
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
	// Reminders: "meeting" carries a Join button, plain clicks open the popup.
	_ = notifier.RegisterNotificationCategory(notifications.NotificationCategory{
		ID:      "meeting",
		Actions: []notifications.NotificationAction{{ID: "join", Title: "Pripojiť sa"}},
	})
	notifier.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		// Only explicit buttons act: on Linux closing a notification with X arrives as the
		// default action too, so body clicks and dismissals can't be told apart.
		switch r.Response.ActionIdentifier {
		case "update":
			go installUpdate(app, notifier)
		case "join":
			if link, _ := r.Response.UserInfo["link"].(string); link != "" {
				_ = app.Browser.OpenURL(link)
			}
		}
	})
	sent := map[string]bool{}
	// Meeting-start alert: a separate frameless window, maximised over everything else.
	alertWin := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:         "alert",
		URL:          "/alert.html",
		Frameless:    true,
		AlwaysOnTop:  true,
		Hidden:       true,
		HideOnEscape: true,
		Windows:      application.WindowsWindow{HiddenOnTaskbar: true},
	})
	showAlert := func(evs []Event) {
		app.Event.Emit("alert", evs)
		alertWin.Show().Focus()
		alertWin.Maximise() // maximised, not fullscreen: the panel and clock stay visible
		skipTaskbar(alertWin)
	}
	alerted := map[string]bool{}
	alert := func() {
		mu.Lock()
		if !cfg.FullscreenAlert || cfg.mutedAt(time.Now()) {
			mu.Unlock()
			return
		}
		// The ticker runs every 10s, so "starting within 15s" fires once, right at the start;
		// the 3 min grace still catches it after waking from sleep.
		due := dueNotifications(events, time.Now(), 15*time.Second, 3*time.Minute, alerted)
		mu.Unlock()
		if len(due) > 0 {
			showAlert(due)
		}
	}
	if *previewAlert {
		go func() {
			time.Sleep(3 * time.Second) // let the first fetch land and the alert page load
			mu.Lock()
			next := nextEvent(events, time.Now())
			mu.Unlock()
			if next != nil {
				showAlert([]Event{*next})
			}
		}()
	}

	notify := func() {
		now := time.Now()
		mu.Lock()
		if cfg.NotifyMinutes <= 0 || cfg.mutedAt(now) {
			mu.Unlock()
			return
		}
		due := dueNotifications(events, now, time.Duration(cfg.NotifyMinutes)*time.Minute, 0, sent)
		mu.Unlock()
		for _, e := range due {
			title, body := notificationText(e, now)
			opts := notifications.NotificationOptions{ID: e.ID, Title: title, Body: body, Data: map[string]any{"link": e.Link}}
			var err error
			if e.Link != "" {
				opts.CategoryID = "meeting"
				err = notifier.SendNotificationWithActions(opts)
			} else {
				err = notifier.SendNotification(opts)
			}
			if err != nil {
				log.Printf("notify %s: %v", e.ID, err)
				mu.Lock()
				delete(sent, sentKey(e)) // retry on the next tick
				mu.Unlock()
			}
		}
	}

	// Mute lives in the right-click menu; the icon gets a red slash while muted.
	var applyMute func()
	mute := func(until time.Time) {
		saveConfig(func(c *Config) { c.MutedUntil = until })
		applyMute()
	}
	wasMuted := false
	applyMute = func() {
		mu.Lock()
		now := time.Now()
		muted, label := cfg.mutedAt(now), muteLabel(cfg.MutedUntil, now)
		mu.Unlock()
		wasMuted = muted
		if runtime.GOOS == "darwin" && !muted {
			tray.SetTemplateIcon(icons.SystrayMacTemplate)
		} else {
			tray.SetIcon(trayIcon(muted))
		}
		menu := app.NewMenu()
		if muted {
			menu.Add("🔕 " + label).SetEnabled(false)
			menu.Add("🔔 Zapnúť notifikácie").OnClick(func(*application.Context) { mute(time.Time{}) })
		} else {
			// Flat items: Plasma's tray submenus clip the label under the arrow.
			menu.Add("🔕 Stlmiť na 1 hodinu").OnClick(func(*application.Context) { mute(time.Now().Add(time.Hour)) })
			menu.Add("🔕 Stlmiť do konca dňa").OnClick(func(*application.Context) {
				y, m, d := time.Now().Date()
				mute(time.Date(y, m, d+1, 0, 0, 0, 0, time.Local))
			})
			menu.Add("🔕 Stlmiť, kým nezapnem").OnClick(func(*application.Context) { mute(muteForever) })
		}
		menu.AddSeparator()
		menu.Add("Koniec").OnClick(func(*application.Context) { app.Quit() })
		tray.SetMenu(menu)
		updateTray()
	}
	applyMute()
	startUpdater(app, notifier, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return cfg.mutedAt(time.Now())
	})

	go func() { // tray countdown and reminders tick faster than the fetch
		for ; ; time.Sleep(10 * time.Second) {
			mu.Lock()
			expired := wasMuted && !cfg.mutedAt(time.Now())
			mu.Unlock()
			if expired {
				applyMute()
			}
			updateTray()
			notify()
			alert()
		}
	}()

	// Wails positions before showing; an unmapped X11 window then gets placed by the WM.
	// Show first, then move.
	showWindow = func() {
		window.Show().Focus()
		skipTaskbar(window)
		_ = tray.PositionWindow(window, 5)
	}

	tray.AttachWindow(window)
	tray.OnClick(func() {
		if window.IsVisible() {
			window.Hide()
			return
		}
		showWindow()
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
