package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io/fs"
	"log"
	"maps"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	wevents "github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

//go:embed frontend
var assets embed.FS

// flatpakID is the app ID inside the Flatpak sandbox, "" elsewhere. Flatpak updates the app
// and the Background portal handles start at login.
var flatpakID = os.Getenv("FLATPAK_ID")

func main() {
	dummyPath := flag.String("dummy", "", "development: read fake events from this JSON file (e.g. dummy.json)")
	previewAlert := flag.Bool("preview-alert", false, "show the fullscreen meeting alert for the next event right after start")
	noAccounts := flag.Bool("no-accounts", false, "development: leave connected accounts out (e.g. for screenshots)")
	forceLang := flag.String("lang", "", "development: use this UI language (e.g. en) over the setting and the system, without saving it")
	flag.Parse()

	// Wayland ignores client window positioning, so the popup would land mid-screen.
	// XWayland honours it. ponytail: drop when Wails supports layer-shell on Wayland.
	if runtime.GOOS == "linux" && os.Getenv("GDK_BACKEND") == "" {
		_ = os.Setenv("GDK_BACKEND", "x11")
	}
	frontend, _ := fs.Sub(assets, "frontend")
	langs, err := loadCatalog(frontend)
	if err != nil {
		log.Fatalf("translations: %v", err)
	}
	notifier := notifications.New()
	app := application.New(application.Options{
		Name:     "do-not-forget", // technical: Linux derives the app ID from it; shown name is "Do Not Forget"
		Services: []application.Service{application.NewService(notifier)},
		Assets:   application.AssetOptions{Handler: application.BundledAssetFileServer(frontend)},
		Mac:      application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
		// The sandbox only lets WebKit own bus names under the Flatpak app ID.
		Linux: application.LinuxOptions{ApplicationID: flatpakID},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	cfg := loadConfig()
	webLangs := []string{} // navigator.languages, reported by the popup; the OS locale on macOS/Windows
	tr.Store(langs.translator(langs.resolve(append([]string{*forceLang, cfg.Language}, systemLanguages()...)...)))
	var (
		mu         sync.Mutex // guards cfg, events and accounts
		events     []Event    // fetched, with the event rules applied
		fetched    []Event    // fetched, before the rules, so a changed rule applies without a fetch
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

	// Links from event data (and the UI) go to the OS only as http(s).
	// A Zoom or Teams meeting opens in its desktop app when one is installed (and Settings allows
	// it), else in the browser. The native link is built here from the checked https one.
	openURL := func(link string) {
		if !externalURL(link) {
			log.Printf("not opening %q: only http(s) links", link)
			return
		}
		mu.Lock()
		nativeApps := cfg.NativeApps
		mu.Unlock()
		if native := nativeMeetingURL(link); nativeApps && native != "" && hasURLHandler(native) {
			link = native
		}
		_ = app.Browser.OpenURL(link)
	}
	app.Event.On("open", func(ev *application.CustomEvent) {
		link, _ := ev.Data.(string)
		openURL(link)
	})

	tray := app.SystemTray.New()
	var emitAccounts func()
	var sendLanguage func()                // settings and translations to every window; defined with the language below
	var checkUpdate func() (string, error) // Settings' "Check for updates"; nil without an updater
	var dummy Source
	if *dummyPath != "" {
		dummy = &dummySource{path: *dummyPath, base: time.Now()}
	}
	// The own Google OAuth client from Settings, if any (guarded by mu); otherwise the built-in one.
	var ownClient oauthClient
	if cfg.GoogleClientID != "" {
		if secret, err := loadClientSecret(); err != nil {
			log.Printf("own Google client: keychain: %v", err)
		} else {
			ownClient = oauthClient{cfg.GoogleClientID, secret}
		}
	}
	activeClient := func() oauthClient { // mu held
		if ownClient.ID != "" {
			return ownClient
		}
		return builtinClient()
	}
	clientFor := func(id string) (oauthClient, bool) { // mu held; "" is the built-in one
		switch id {
		case "", googleClientID:
			return builtinClient(), googleClientID != ""
		case ownClient.ID:
			return ownClient, true
		}
		return oauthClient{}, false
	}

	// Google accounts, guarded by mu. A token missing from the keychain, or a client that's no
	// longer set up, leaves the account listed with its error, so it can be connected again.
	var accounts []*googleSource
	for _, a := range cfg.Accounts {
		if *noAccounts {
			break
		}
		c, ok := clientFor(a.ClientID)
		tok, err := loadToken(a.Email)
		var g *googleSource
		switch {
		case !ok:
			g = brokenGoogleSource(a.Email, errClientGone)
		case err != nil:
			g = brokenGoogleSource(a.Email, fmt.Errorf("keychain: %w", err))
		default:
			g = newGoogleSource(a.Email, c, tok, a.Hidden)
		}
		g.label = a.Label
		g.setNames(a.Names)
		for _, id := range a.Quiet {
			g.setQuiet(id, true)
		}
		accounts = append(accounts, g)
	}
	adding, addErr := false, "" // Google sign-in in progress / its failure, guarded by mu
	updateVersion := ""         // a newer release found by the updater, guarded by mu
	autostartFailed := false    // the last start-at-login change failed, guarded by mu
	clientErr := ""             // saving the own client failed, guarded by mu
	mgr := &accountManager{mu: &mu, list: &accounts, removing: map[string]bool{}, saveConfig: saveConfig}

	// The tray icon: a template on macOS, so it follows a light or dark menu bar.
	// ponytail: the muted icon is drawn for dark panels only, see trayIcon.
	setTrayIcon := func(muted bool) {
		if runtime.GOOS == "darwin" && !muted {
			tray.SetTemplateIcon(trayIcon(false))
		} else {
			tray.SetIcon(trayIcon(muted))
		}
	}
	trayIconPos := application.IconPosition(application.NSImageLeading) // guarded by mu
	updateTray := func() {
		now := time.Now()
		mu.Lock()
		next := nextEvent(events, now)
		muted := muteLabel(cfg.MutedUntil, now)
		style, titleMax := cfg.TrayStyle, cfg.TrayTitleMax
		mu.Unlock()
		title, sub := T("tray.none"), ""
		if next != nil {
			title = next.Title
			sub = fmt.Sprintf(`%s · %s – %s · <span style="color:%s">●</span> %s`,
				untilText(next, now), next.Start.Format("15:04"), next.End.Format("15:04"),
				next.Color, html.EscapeString(next.Calendar))
		}
		if muted != "" {
			sub += "<br>🔕 " + muted
		}
		setTrayTooltip(tray, html.EscapeString(title), sub)

		if runtime.GOOS != "darwin" {
			return
		}
		// Menu bar text only for a meeting today (or still running); otherwise just the icon.
		label := ""
		if next != nil && style != "icon" && daysBetween(now, next.Start) <= 0 {
			label = truncateTitle(next.Title, titleMax) + " · " + untilText(next, now)
		}
		tray.SetLabel(label)
		// Text only hides the icon, unless there's no text or the icon has to show the mute.
		pos := application.IconPosition(application.NSImageLeading)
		if style == "text" && label != "" && muted == "" {
			pos = application.NSImageNone
		}
		mu.Lock()
		changed := pos != trayIconPos
		trayIconPos = pos
		mu.Unlock()
		if changed { // Wails applies the position with the next icon
			tray.SetIconPosition(pos)
			setTrayIcon(muted != "")
		}
	}
	// Settings › Accounts: each account with its calendars and last sync error.
	emitAccounts = func() {
		mu.Lock()
		gs := append([]*googleSource{}, accounts...)
		state := map[string]any{"google": activeClient().ID != "", "adding": adding, "error": addErr,
			"removing": slices.Collect(maps.Keys(mgr.removing)),
			// Settings › own client ID; the secret never goes back to the page.
			"client": map[string]any{"own": ownClient.ID, "builtin": googleClientID != "", "error": clientErr}}
		mu.Unlock()
		list := make([]map[string]any, 0, len(gs))
		for _, g := range gs {
			label, cals, errText := g.status()
			list = append(list, map[string]any{"email": g.email, "label": label, "calendars": cals, "error": errText,
				"revokeFailed": g.revokeFailed(), "signIn": g.needsSignIn()})
		}
		state["accounts"] = list
		app.Event.Emit("accounts", state)
	}
	var lastSync time.Time // guarded by mu; the ticker below refreshes every SyncMinutes
	refresh := func() {
		now := time.Now()
		y, m, d := now.Date()
		today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		mu.Lock()
		lastSync = now
		var sources []Source
		if dummy != nil {
			sources = append(sources, dummy)
		}
		for _, g := range accounts {
			sources = append(sources, g)
		}
		days := cfg.fetchDays()
		mu.Unlock()
		evs := fetchAll(context.Background(), sources, today, today.AddDate(0, 0, days))
		mu.Lock()
		for _, g := range accounts {
			g.applyQuiet(evs)
		}
		fetched = evs
		// Rules of events that are over go once a fetch shows they really are, also from the file.
		expired := cfg.expireRules(evs, now)
		if expired {
			if err := cfg.save(); err != nil {
				log.Printf("save config: %v", err)
			}
		}
		evs = cfg.applyRules(evs)
		events = evs
		mu.Unlock()
		if expired {
			sendLanguage() // Settings lists the rules
		}
		app.Event.Emit("events", map[string]any{"events": evs, "synced": now, "sources": len(sources)})
		updateTray()
		emitAccounts()
	}
	app.Event.On("refresh", func(*application.CustomEvent) { go refresh() })
	// reapplyRules puts a changed rule into effect on the events at hand, so a reminder due before
	// the next fetch already follows it.
	reapplyRules := func() {
		mu.Lock()
		events = cfg.applyRules(fetched)
		mu.Unlock()
	}
	app.Event.On("accounts:get", func(*application.CustomEvent) { emitAccounts() })

	// Connecting a Google account: browser sign-in, then the token into the keychain.
	// Connecting one that's already there replaces its token and keeps its calendar choice.
	// Connecting: browser sign-in first (no lock, it can take minutes), then the manager stores it.
	addAccount := func(email string) error { // email: the account to connect again, or ""
		if mgr.busy() {
			return errBusy
		}
		mu.Lock()
		c := activeClient()
		mu.Unlock()
		tok, err := googleLogin(context.Background(), c, app.Browser.OpenURL, email)
		if err != nil {
			return err
		}
		email, hidden, err := accountEmail(context.Background(), c, tok)
		if err != nil {
			return err
		}
		return mgr.connect(email, c, tok, hidden)
	}

	// The own OAuth client: set or cleared in Settings. Accounts signed in through a client
	// that's no longer set up stop syncing and ask to be connected again.
	setOwnClient := func(c oauthClient) error {
		if c.ID != "" {
			if !validClientID(c.ID) || c.Secret == "" {
				return errors.New("needs a Client ID ending in .apps.googleusercontent.com and its secret")
			}
			if err := saveClientSecret(c.Secret); err != nil {
				return fmt.Errorf("keychain: %w", err)
			}
		} else if err := deleteClientSecret(); err != nil {
			return fmt.Errorf("keychain: %w", err)
		}
		saveConfig(func(cfg *Config) { cfg.GoogleClientID = c.ID })
		mu.Lock()
		ownClient = c
		usable := map[string]bool{ownClient.ID: ownClient.ID != "", googleClientID: googleClientID != ""}
		mu.Unlock()
		mgr.dropClients(func(id string) bool { return usable[id] })
		return nil
	}
	app.Event.On("client:set", func(ev *application.CustomEvent) {
		var req struct{ ID, Secret string }
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil {
			return
		}
		go func() {
			err := setOwnClient(oauthClient{strings.TrimSpace(req.ID), strings.TrimSpace(req.Secret)})
			mu.Lock()
			clientErr = ""
			if err != nil {
				log.Printf("own Google client: %v", err)
				clientErr = err.Error()
			}
			mu.Unlock()
			refresh() // also re-emits the accounts
		}()
	})
	app.Event.On("account:add", func(ev *application.CustomEvent) {
		email, _ := ev.Data.(string) // set when connecting an account again
		mu.Lock()
		if adding {
			mu.Unlock()
			return
		}
		adding, addErr = true, ""
		mu.Unlock()
		emitAccounts()
		go func() {
			err := addAccount(email)
			mu.Lock()
			adding = false
			if err != nil {
				log.Printf("add account: %v", err)
				addErr = err.Error()
			}
			mu.Unlock()
			refresh() // also re-emits the accounts
		}()
	})
	// Disconnecting revokes the token at Google and deletes it from the keychain.
	app.Event.On("account:remove", func(ev *application.CustomEvent) {
		var req struct {
			Email string
			Local bool // after a failed revoke: only delete it here
		}
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil {
			return
		}
		email, mode := req.Email, revokeAndDelete
		if req.Local {
			mode = deleteOnly
		}
		mu.Lock()
		var g *googleSource
		if i := slices.IndexFunc(accounts, func(a *googleSource) bool { return a.email == email }); i >= 0 {
			g = accounts[i]
		}
		mu.Unlock()
		if g == nil {
			return
		}
		go func() {
			if err := mgr.disconnect(g, mode, emitAccounts); err != nil {
				log.Printf("remove %s: %v", email, err)
			}
			refresh() // also re-emits the accounts
		}()
	})
	// Naming an account ("Work", "Personal") shows that name instead of its email everywhere.
	app.Event.On("account:label", func(ev *application.CustomEvent) {
		var req struct{ Email, Label string }
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil {
			return
		}
		label := strings.TrimSpace(req.Label)
		if r := []rune(label); len(r) > 40 {
			label = string(r[:40])
		}
		saveConfig(func(c *Config) {
			for i, a := range c.Accounts {
				if a.Email == req.Email {
					c.Accounts[i].Label = label
				}
			}
		})
		mu.Lock()
		for _, g := range accounts {
			if g.email == req.Email {
				g.setLabel(label)
			}
		}
		mu.Unlock()
		go refresh()
	})
	// Naming a calendar (a shared one Google calls by its owner's email, say), only in this app.
	app.Event.On("calendar:name", func(ev *application.CustomEvent) {
		var req struct{ Email, ID, Name string }
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil {
			return
		}
		name := strings.TrimSpace(req.Name)
		if r := []rune(name); len(r) > 40 {
			name = string(r[:40])
		}
		var names map[string]string
		saveConfig(func(c *Config) {
			for i, a := range c.Accounts {
				if a.Email != req.Email {
					continue
				}
				if a.Names == nil {
					a.Names = map[string]string{}
				}
				if name == "" {
					delete(a.Names, req.ID)
				} else {
					a.Names[req.ID] = name
				}
				c.Accounts[i].Names = a.Names
				names = maps.Clone(a.Names) // the config's map is only touched under saveConfig's lock
			}
		})
		mu.Lock()
		for _, g := range accounts {
			if g.email == req.Email {
				g.setNames(names)
			}
		}
		mu.Unlock()
		go refresh()
	})
	// A calendar's tick (shown in the agenda) and bell (reminders for its events), from Settings.
	app.Event.On("calendar:toggle", func(ev *application.CustomEvent) {
		var req struct {
			Email, ID string
			On        bool
			Remind    bool // the bell rather than the tick
		}
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil {
			return
		}
		saveConfig(func(c *Config) {
			for i, a := range c.Accounts {
				if a.Email != req.Email {
					continue
				}
				list := &c.Accounts[i].Hidden
				if req.Remind {
					list = &c.Accounts[i].Quiet
				}
				*list = slices.DeleteFunc(*list, func(id string) bool { return id == req.ID })
				if !req.On {
					*list = append(*list, req.ID)
				}
			}
		})
		mu.Lock()
		for _, g := range accounts {
			switch {
			case g.email != req.Email:
			case req.Remind:
				g.setQuiet(req.ID, !req.On)
			default:
				g.setHidden(req.ID, !req.On)
			}
		}
		if req.Remind {
			for _, g := range accounts {
				g.applyQuiet(fetched)
			}
			events = cfg.applyRules(fetched)
		}
		mu.Unlock()
		go refresh()
	})
	// Muting or hiding an event or its series from the agenda, and undoing it from Settings.
	app.Event.On("event:rule", func(ev *application.CustomEvent) {
		var req struct {
			ID           string
			Series, Hide bool
		}
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil {
			return
		}
		mu.Lock()
		i := slices.IndexFunc(events, func(e Event) bool { return e.ID == req.ID })
		var e Event
		if i >= 0 {
			e = events[i]
		}
		mu.Unlock()
		if i < 0 {
			return
		}
		r := EventRule{ID: e.ID, Title: e.Title, Hide: req.Hide, Until: e.End}
		if req.Series && e.SeriesID != "" {
			r.ID, r.Until = e.SeriesID, time.Time{}
		}
		saveConfig(func(c *Config) { c.setRule(r) })
		reapplyRules()
		sendLanguage() // Settings lists the rules
		go refresh()
	})
	app.Event.On("event:unrule", func(ev *application.CustomEvent) {
		id, _ := ev.Data.(string)
		saveConfig(func(c *Config) { c.dropRule(id) })
		reapplyRules()
		sendLanguage()
		go refresh()
	})
	app.Event.On("respond", func(ev *application.CustomEvent) {
		var req struct {
			ID, Response string
			Series       bool // the whole recurring series, not just this occurrence
		}
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil {
			return
		}
		mu.Lock()
		var src Source
		series := ""
		for _, e := range events {
			if e.ID == req.ID {
				src = e.src
				if req.Series {
					series = e.SeriesID
				}
			}
		}
		mu.Unlock()
		if src == nil {
			return
		}
		go func() {
			ctx := context.Background()
			var err error
			if series != "" {
				y, m, d := time.Now().Date()
				today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
				err = answerSeries(ctx, src, series, req.Response, today, today.AddDate(0, 0, 7)) // what the agenda shows
			} else {
				err = src.Respond(ctx, req.ID, req.Response)
			}
			if err != nil {
				log.Printf("respond %s: %v", req.ID, err)
				app.Event.Emit("respond:failed", req.ID) // the refresh below shows the answer that stuck
			}
			refresh()
		}()
	})
	// Notification buttons; re-registered when the language changes.
	registerCategories := func() {
		snooze := notifications.NotificationAction{ID: "snooze", Title: T("snooze.notification")}
		_ = notifier.RegisterNotificationCategory(notifications.NotificationCategory{
			ID: "meeting", Actions: []notifications.NotificationAction{{ID: "join", Title: T("join")}, snooze},
		})
		_ = notifier.RegisterNotificationCategory(notifications.NotificationCategory{
			ID: "reminder", Actions: []notifications.NotificationAction{snooze},
		})
		_ = notifier.RegisterNotificationCategory(notifications.NotificationCategory{
			ID: "update", Actions: []notifications.NotificationAction{{ID: "update", Title: T("update.action")}},
		})
	}
	registerCategories()
	var snoozes []snoozed // guarded by mu
	snooze := func(ids []string, d time.Duration, alert bool) {
		mu.Lock()
		defer mu.Unlock()
		for _, id := range ids {
			snoozes = append(snoozes, snoozed{id, time.Now().Add(d), alert})
		}
	}
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
				openURL(link)
			}
		case "snooze":
			if id, _ := r.Response.UserInfo["id"].(string); id != "" {
				snooze([]string{id}, 5*time.Minute, false)
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
	// dueAlerts are the meetings whose start alert is due; the ticker shows them.
	dueAlerts := func() []Event {
		mu.Lock()
		defer mu.Unlock()
		if !cfg.FullscreenAlert || cfg.mutedAt(time.Now()) {
			return nil
		}
		// The ticker runs every 10s, so "starting within 15s" fires once, right at the start;
		// the 3 min grace still catches it after waking from sleep.
		return dueNotifications(cfg.remindable(events), time.Now(), 15*time.Second, 3*time.Minute, alerted)
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

	// sendReminder shows a meeting reminder with Join (when there's a link) and Snooze.
	sendReminder := func(e Event, id string, textAt time.Time) error {
		mu.Lock()
		silent := !cfg.Sound
		mu.Unlock()
		title, body := notificationText(e, textAt)
		opts := notifications.NotificationOptions{ID: id, Title: title, Body: body, CategoryID: "reminder",
			Data: map[string]any{"link": e.Link, "id": e.ID}}
		if e.Link != "" {
			opts.CategoryID = "meeting"
		}
		if silent {
			opts.Sound = &notifications.NotificationSound{Silent: true}
		}
		err := notifier.SendNotificationWithActions(opts)
		if err != nil {
			log.Printf("notify %s: %v", id, err)
		}
		return err
	}
	sentAtStart := map[string]bool{} // "also at the start" notifications, guarded by mu
	notify := func() {
		now := time.Now()
		mu.Lock()
		if !cfg.Notify || cfg.mutedAt(now) {
			mu.Unlock()
			return
		}
		evs := cfg.remindable(events)
		due := dueReminders(evs, now, time.Duration(cfg.NotifyMinutes)*time.Minute, cfg.CalendarReminders, cfg.NotifyAtStart, sent)
		var atStart []Event
		if cfg.NotifyAtStart { // like the alert: once, right at the start, or soon after waking
			atStart = dueNotifications(evs, now, 15*time.Second, 3*time.Minute, sentAtStart)
		}
		mu.Unlock()
		send := func(e Event, id string, textAt time.Time, sentMap map[string]bool) {
			if sendReminder(e, id, textAt) != nil {
				mu.Lock()
				forgetSent(sentMap, e) // retry on the next tick
				mu.Unlock()
			}
		}
		for _, e := range due {
			send(e, e.ID, now, sent)
		}
		for _, e := range atStart {
			send(e, e.ID+"@start", e.Start, sentAtStart) // reads "in progress until …"
		}
	}
	// wakeSnoozed sends the snoozed reminders that are due and returns the snoozed alerts, for
	// the ticker to show together with the start alerts due at the same time.
	wakeSnoozed := func() []Event {
		now := time.Now()
		mu.Lock()
		var alerts, reminders []Event
		snoozes, alerts, reminders = dueSnoozes(snoozes, cfg, events, now)
		mu.Unlock()
		for _, e := range reminders {
			if sendReminder(e, e.ID+"@snooze", now) != nil {
				snooze([]string{e.ID}, 0, false) // retry on the next tick
			}
		}
		return alerts
	}
	// The alert's "Remind me in …"
	app.Event.On("snooze", func(ev *application.CustomEvent) {
		var req struct {
			IDs     []string
			Minutes int
		}
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil || req.Minutes < 1 || req.Minutes > 60 {
			return
		}
		snooze(req.IDs, time.Duration(req.Minutes)*time.Minute, true)
	})

	// Mute lives in the right-click menu; the icon gets a red slash while muted.
	var applyMute func()
	mute := func(until time.Time) {
		saveConfig(func(c *Config) { c.MutedUntil = until })
		applyMute()
	}
	wasMuted := false // guarded by mu
	applyMute = func() {
		mu.Lock()
		now := time.Now()
		muted, label := cfg.mutedAt(now), muteLabel(cfg.MutedUntil, now)
		wasMuted = muted
		mu.Unlock()
		setTrayIcon(muted)
		menu := app.NewMenu()
		if muted {
			menu.Add("🔕 " + label).SetEnabled(false)
			menu.Add(T("menu.unmute")).OnClick(func(*application.Context) { mute(time.Time{}) })
		} else {
			// Flat items: Plasma's tray submenus clip the label under the arrow.
			menu.Add(T("menu.mute_hour")).OnClick(func(*application.Context) { mute(time.Now().Add(time.Hour)) })
			menu.Add(T("menu.mute_day")).OnClick(func(*application.Context) {
				y, m, d := time.Now().Date()
				mute(time.Date(y, m, d+1, 0, 0, 0, 0, time.Local))
			})
			menu.Add(T("menu.mute_forever")).OnClick(func(*application.Context) { mute(muteForever) })
		}
		menu.AddSeparator()
		menu.Add(T("menu.quit")).OnClick(func(*application.Context) { app.Quit() })
		tray.SetMenu(menu)
		updateTray()
	}
	applyMute()

	// Language: the popup says hello with navigator.languages and can pick one in Settings.
	// Switching swaps the translator and redraws everything that has text, no restart.
	sendLanguage = func() {
		mu.Lock()
		setting, trayStyle, trayTitleMax, update := cfg.Language, cfg.TrayStyle, cfg.TrayTitleMax, updateVersion
		settings, failed, autostart := cfg.settings(), autostartFailed, cfg.FlatpakAutostart
		mu.Unlock()
		autostartErr := ""
		if failed {
			autostartErr = T("settings.autostart_error") // the cause is in the log
		}
		if flatpakID == "" {
			var err error
			if autostart, err = app.Autostart.IsEnabled(); err != nil {
				log.Printf("autostart status: %v", err)
			}
		}
		t := tr.Load()
		app.Event.Emit("i18n", map[string]any{
			"lang": t.code, "strings": t.strings, "languages": langs.languages(), "setting": setting,
			"version": version, "update": update, "canCheck": checkUpdate != nil, // for Settings › About
			// Settings › Menu bar, offered on macOS only.
			"trayStyle": trayStyle, "trayTitleMax": trayTitleMax, "platform": runtime.GOOS,
			"settings": settings, "autostart": autostart, "autostartError": autostartErr,
			// Inside Flatpak xdg-mime can't see the host's apps, so meetings always open in the browser.
			"flatpak": flatpakID != "",
		})
	}
	applyLanguage := func() {
		mu.Lock()
		prefs := append(append([]string{*forceLang, cfg.Language}, webLangs...), systemLanguages()...)
		mu.Unlock()
		// Every window says hello on load; only a real change rebuilds the menu and categories.
		if code := langs.resolve(prefs...); code != tr.Load().code {
			tr.Store(langs.translator(code))
			registerCategories()
			applyMute() // rebuilds the tray menu and tooltip
		}
		sendLanguage()
	}
	app.Event.On("i18n:hello", func(ev *application.CustomEvent) {
		var l []string
		if b, err := json.Marshal(ev.Data); err == nil {
			_ = json.Unmarshal(b, &l)
		}
		mu.Lock()
		webLangs = l
		mu.Unlock()
		applyLanguage()
	})
	app.Event.On("i18n:set", func(ev *application.CustomEvent) {
		code, _ := ev.Data.(string)
		saveConfig(func(c *Config) { c.Language = code })
		applyLanguage()
	})
	// Settings: one event for the plain options (see Config.set); every window gets the result.
	app.Event.On("settings:set", func(ev *application.CustomEvent) {
		var req struct {
			Key   string
			Value json.RawMessage
		}
		if b, err := json.Marshal(ev.Data); err != nil || json.Unmarshal(b, &req) != nil {
			return
		}
		var err error
		saveConfig(func(c *Config) { err = c.set(req.Key, req.Value) })
		if err != nil {
			log.Printf("settings: %v", err)
		}
		sendLanguage()
	})
	// Start at login is the system's registration (login item, Run key, autostart .desktop), not
	// a config field, so Settings always shows what will really happen.
	// Changes run one at a time with the latest wish, so a quick on-off can't end with a slower
	// Enable finishing after Disable.
	var autostartChange latestWins[bool]
	app.Event.On("autostart:set", func(ev *application.CustomEvent) {
		on, _ := ev.Data.(bool)
		autostartChange.set(on, func(on bool) {
			var err error
			switch {
			case flatpakID != "":
				if err = flatpakAutostart(on); err == nil {
					saveConfig(func(c *Config) { c.FlatpakAutostart = on })
				}
			case on:
				err = app.Autostart.Enable()
			default:
				err = app.Autostart.Disable()
			}
			mu.Lock()
			autostartFailed = err != nil
			if err != nil {
				log.Printf("autostart: %v", err)
			}
			mu.Unlock()
			sendLanguage() // shows the registration as the system has it now
		})
	})
	app.Event.On("tray:length", func(ev *application.CustomEvent) {
		if n, _ := ev.Data.(float64); validTrayTitleMax(int(n)) { // JSON numbers arrive as float64
			saveConfig(func(c *Config) { c.TrayTitleMax = int(n) })
			updateTray()
		}
	})
	app.Event.On("tray:style", func(ev *application.CustomEvent) {
		if style, _ := ev.Data.(string); validTrayStyle(style) {
			saveConfig(func(c *Config) { c.TrayStyle = style })
			updateTray()
		}
	})

	checkUpdate = startUpdater(app, notifier, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return cfg.mutedAt(time.Now())
	}, func(v string) {
		mu.Lock()
		changed := updateVersion != v
		updateVersion = v
		mu.Unlock()
		if changed {
			sendLanguage() // carries the version for Settings › About
		}
	})
	app.Event.On("update:install", func(*application.CustomEvent) { go installUpdate(app, notifier) })
	// Settings › About › Check for updates. A newer version shows up through available (above)
	// as the Update button; this only tells the page how the check ended.
	app.Event.On("update:check", func(*application.CustomEvent) {
		if checkUpdate == nil {
			return
		}
		go func() {
			v, err := checkUpdate()
			if err != nil {
				log.Printf("update check: %v", err)
			}
			app.Event.Emit("update:checked", checkResult(v, err))
		}()
	})

	// macOS drops notifications from an app that never asked; ask once it's running (the
	// system prompts only the first time).
	if runtime.GOOS == "darwin" {
		app.Event.OnApplicationEvent(wevents.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			go func() {
				if ok, _ := notifier.CheckNotificationAuthorization(); ok {
					return
				}
				if ok, err := notifier.RequestNotificationAuthorization(); !ok {
					log.Printf("notifications not allowed: %v", err)
				}
			}()
		})
	}

	go func() { // tray countdown and reminders tick faster than the fetch
		for ; ; time.Sleep(10 * time.Second) {
			mu.Lock()
			expired := wasMuted && !cfg.mutedAt(time.Now())
			mu.Unlock()
			if expired {
				applyMute()
			}
			mu.Lock()
			syncDue := time.Since(lastSync) >= time.Duration(cfg.SyncMinutes)*time.Minute
			mu.Unlock()
			if syncDue {
				go refresh()
			}
			updateTray()
			notify()
			// One alert window: what's due now and what was snoozed until now show together.
			if due := mergeAlerts(dueAlerts(), wakeSnoozed()); len(due) > 0 {
				showAlert(due)
			}
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

// untilText is the countdown shown in the tray: "in 12 min", "in 1 h 5 min", "in progress until 17:57",
// and for later days "tomorrow" or "in 3 days".
func untilText(e *Event, now time.Time) string {
	if !e.Start.After(now) {
		return T("tray.running", "time", e.End.Format("15:04"))
	}
	switch days := daysBetween(now, e.Start); {
	case days == 1:
		return T("day.tomorrow")
	case days > 1:
		return T("in.days", "count", days)
	}
	m := int(e.Start.Sub(now).Round(time.Minute).Minutes())
	if m < 60 {
		return T("in.min", "m", m)
	}
	return T("in.hmin", "h", m/60, "m", m%60)
}

// truncateTitle shortens a title to at most max characters, ending in "…".
func truncateTitle(s string, maxLen int) string {
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return strings.TrimRight(string(r[:maxLen-1]), " -–·,:") + "…"
}

// daysBetween counts calendar days from a to b (0 = same day), DST-safe.
func daysBetween(a, b time.Time) int {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.In(a.Location()).Date()
	return int(time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC).Sub(time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)).Hours() / 24)
}
