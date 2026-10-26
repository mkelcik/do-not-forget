package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Config is the user's persisted settings, in <UserConfigDir>/do-not-forget/config.json.
type Config struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	// Notify sends a reminder NotifyMinutes before each meeting (1, 5, 10 or 15).
	Notify        bool `json:"notify"`
	NotifyMinutes int  `json:"notifyMinutes"`
	// NotifyAtStart sends a second notification when the meeting starts.
	NotifyAtStart bool `json:"notifyAtStart"`
	// FullscreenAlert covers the screen when a meeting starts, for when a notification gets missed.
	FullscreenAlert bool `json:"fullscreenAlert"`
	// OnlyWithLink limits reminders and the alert to meetings with a Meet, Zoom or Teams link.
	OnlyWithLink bool `json:"onlyWithLink"`
	// OnlyAccepted skips invitations I haven't answered or answered "maybe".
	OnlyAccepted bool `json:"onlyAccepted"`
	// OnlyWithOthers skips events with no one but me, such as blocked time.
	OnlyWithOthers bool `json:"onlyWithOthers"`
	// CalendarReminders reminds at the times each event has in its calendar instead of
	// NotifyMinutes, which stays for events whose calendar doesn't say.
	CalendarReminders bool `json:"calendarReminders"`
	// Sound plays the system sound with notifications.
	Sound bool `json:"sound"`
	// NativeApps opens Zoom and Teams meetings in their desktop apps when installed.
	NativeApps bool `json:"nativeApps"`
	// Theme is "", "light" or "dark"; "" follows the system.
	Theme string `json:"theme"`
	// WeekTab shows the 7-day tab; ShowDeclined lists declined events (struck through).
	WeekTab      bool `json:"weekTab"`
	ShowDeclined bool `json:"showDeclined"`
	// SyncMinutes is how often calendars are fetched (1, 5 or 15).
	SyncMinutes int `json:"syncMinutes"`
	// Language is a BCP 47 tag from frontend/i18n, or "" to follow the system.
	Language string `json:"language"`
	// MutedUntil silences reminders and alerts until then; zero means not muted.
	MutedUntil time.Time `json:"mutedUntil"`
	// FlatpakAutostart remembers start at login inside the Flatpak, where the Background portal
	// sets it but can't be asked for it. Elsewhere the system's registration is the truth.
	FlatpakAutostart bool `json:"flatpakAutostart"`
	// TrayStyle is what the macOS menu bar shows for today's next meeting: "icon", "text" or
	// "both" (icon + title and countdown). Other systems' trays only take an icon.
	TrayStyle string `json:"trayStyle"`
	// TrayTitleMax caps the meeting title in the macOS menu bar, in characters; the countdown
	// after it always shows in full.
	TrayTitleMax int `json:"trayTitleMax"`
	// Accounts are the connected calendar accounts; their tokens are in the system keychain.
	Accounts []Account `json:"accounts"`
	// GoogleClientID is the user's own OAuth client, used instead of the built-in one; its
	// secret is in the keychain.
	GoogleClientID string `json:"googleClientId,omitempty"`
	// EventRules are events and series the user muted or hid.
	EventRules []EventRule `json:"eventRules,omitempty"`
}

// EventRule mutes an event or a whole series (no reminders, no alert) or hides it from the
// agenda as well.
type EventRule struct {
	ID    string `json:"id"`    // an event's ID, or a series' (Event.SeriesID)
	Title string `json:"title"` // for Settings, where rules can be undone
	Hide  bool   `json:"hide,omitempty"`
	// Until is a one-off event's end, after which the rule has nothing left to do and is
	// dropped; zero for a series.
	Until time.Time `json:"until,omitzero"`
}

// Account is a connected Google account.
type Account struct {
	Email string `json:"email"`
	// Label is the user's name for the account ("Work", "Personal"); it replaces the email that
	// Google uses as the primary calendar's name. Empty shows the email.
	Label string `json:"label,omitempty"`
	// Names are the user's names for calendars, by calendar ID (a shared calendar Google names
	// after its owner's email, say). They win over the label and Google's names.
	Names map[string]string `json:"names,omitempty"`
	// ClientID is the OAuth client the account signed in with; empty means the built-in one.
	ClientID string `json:"clientId,omitempty"`
	// Hidden lists the IDs of calendars unticked in Settings; new calendars show up ticked.
	Hidden []string `json:"hidden,omitempty"`
	// Quiet lists the IDs of calendars whose events get no reminders or alert.
	Quiet []string `json:"quiet,omitempty"`
}

func validTrayStyle(s string) bool { return s == "icon" || s == "text" || s == "both" }

func validTrayTitleMax(n int) bool { return n >= 10 && n <= 100 }

// muteForever is "until I turn it back on".
var muteForever = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

func (c Config) mutedAt(now time.Time) bool { return now.Before(c.MutedUntil) }

// fetchDays is how far ahead events are fetched: the 7-day tab, or with reminder times from the
// calendar as far as Google lets a reminder be set (4 weeks), so those reminders come on time.
func (c Config) fetchDays() int {
	if c.CalendarReminders {
		return 29
	}
	return 7
}

// applyRules drops hidden events and marks muted ones. An event can match a rule of its own and
// one of its series; hiding wins, whichever came first.
func (c Config) applyRules(events []Event) []Event {
	out := make([]Event, 0, len(events))
	for _, e := range events {
		matched, hide := c.ruleFor(e)
		e.Muted = e.Muted || matched
		if !hide {
			out = append(out, e)
		}
	}
	return out
}

// hiddenStarts are the starts of the events the rules hide, so the agenda can say how many a
// day leaves out.
func (c Config) hiddenStarts(events []Event) []time.Time {
	out := []time.Time{}
	for _, e := range events {
		if _, hide := c.ruleFor(e); hide {
			out = append(out, e.Start)
		}
	}
	return out
}

// ruleFor reports whether a rule covers e, as itself or its series, and whether one hides it.
func (c Config) ruleFor(e Event) (matched, hide bool) {
	for _, r := range c.EventRules {
		if r.ID == e.ID || (e.SeriesID != "" && r.ID == e.SeriesID) {
			matched, hide = true, hide || r.Hide
		}
	}
	return matched, hide
}

// setRule adds or replaces the rule for r.ID.
func (c *Config) setRule(r EventRule) {
	c.dropRule(r.ID)
	c.EventRules = append(c.EventRules, r)
}

// expireRules follows one-off rules' events in a fresh fetch: a rescheduled event's rule moves to
// its new end, and rules of events that are over are dropped, so their titles don't stay in the
// config. A rule whose event isn't in the fetch keeps its last known end. True if any changed.
// ponytail: an event moved beyond the fetched 7 days, or in an account that failed to sync, isn't
// seen, so its rule still expires at the old end.
func (c *Config) expireRules(fetched []Event, now time.Time) bool {
	changed := false
	kept := c.EventRules[:0]
	for _, r := range c.EventRules {
		if !r.Until.IsZero() {
			if j := slices.IndexFunc(fetched, func(e Event) bool { return e.ID == r.ID }); j >= 0 && !fetched[j].End.Equal(r.Until) {
				r.Until, changed = fetched[j].End, true
			}
			if r.Until.Before(now) {
				changed = true
				continue
			}
		}
		kept = append(kept, r)
	}
	c.EventRules = kept
	return changed
}

func (c *Config) dropRule(id string) {
	c.EventRules = slices.DeleteFunc(c.EventRules, func(r EventRule) bool { return r.ID == id })
}

// remindable are the events reminders and the alert are for, after the filters in Settings.
func (c Config) remindable(events []Event) []Event {
	return slices.DeleteFunc(slices.Clone(events), func(e Event) bool {
		return e.Muted || (c.OnlyWithLink && e.Link == "") ||
			(c.OnlyAccepted && (e.Response == "pending" || e.Response == "maybe")) ||
			(c.OnlyWithOthers && !slices.ContainsFunc(e.Attendees, func(a Attendee) bool { return !a.Self }))
	})
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "do-not-forget", "config.json")
}

// loadConfig never fails: a missing or broken file just means defaults.
// It also locks the config dir to the user (0700): Wails' notifier creates it 0755 on Linux,
// and MkdirAll leaves an existing dir's mode alone, so tightening it first sticks.
func loadConfig() Config {
	dir := filepath.Dir(configPath())
	_ = os.MkdirAll(dir, 0o700)
	_ = os.Chmod(dir, 0o700) //nolint:gosec // G302: a directory needs the x bit; 0700 is owner-only

	c := defaultConfig()
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	c.normalise()
	return c
}

func defaultConfig() Config {
	return Config{Width: 380, Height: 560, Notify: true, NotifyMinutes: 5, FullscreenAlert: true, Sound: true, NativeApps: true,
		WeekTab: true, ShowDeclined: true, SyncMinutes: 1, TrayStyle: "both", TrayTitleMax: 25}
}

// normalise replaces values the UI doesn't offer with defaults. Configs from before the
// Notify switch turned reminders off with notifyMinutes 0.
func (c *Config) normalise() {
	if c.NotifyMinutes <= 0 {
		c.Notify, c.NotifyMinutes = false, 5
	}
	if !slices.Contains(notifyMinuteChoices, c.NotifyMinutes) {
		c.NotifyMinutes = 5
	}
	if !slices.Contains(syncMinuteChoices, c.SyncMinutes) {
		c.SyncMinutes = 1
	}
	if !slices.Contains(themeChoices, c.Theme) {
		c.Theme = ""
	}
	if !validTrayStyle(c.TrayStyle) {
		c.TrayStyle = "both"
	}
	if !validTrayTitleMax(c.TrayTitleMax) {
		c.TrayTitleMax = 25
	}
}

var (
	notifyMinuteChoices = []int{1, 5, 10, 15}
	syncMinuteChoices   = []int{1, 5, 15}
	themeChoices        = []string{"", "light", "dark"}
)

// settings are the options Settings shows, by the same names as in config.json.
func (c Config) settings() map[string]any {
	return map[string]any{
		"notify": c.Notify, "notifyMinutes": c.NotifyMinutes, "notifyAtStart": c.NotifyAtStart,
		"fullscreenAlert": c.FullscreenAlert, "onlyWithLink": c.OnlyWithLink, "sound": c.Sound, "nativeApps": c.NativeApps,
		"onlyAccepted": c.OnlyAccepted, "onlyWithOthers": c.OnlyWithOthers, "calendarReminders": c.CalendarReminders,
		"theme": c.Theme, "weekTab": c.WeekTab, "showDeclined": c.ShowDeclined, "syncMinutes": c.SyncMinutes,
		"eventRules": append([]EventRule{}, c.EventRules...), // [] rather than null; Settings › Muted and hidden
	}
}

// set changes one of settings() from the page, refusing unknown keys and values the UI doesn't offer.
func (c *Config) set(key string, value json.RawMessage) error {
	flags := map[string]*bool{
		"notify": &c.Notify, "notifyAtStart": &c.NotifyAtStart, "fullscreenAlert": &c.FullscreenAlert,
		"onlyWithLink": &c.OnlyWithLink, "sound": &c.Sound, "weekTab": &c.WeekTab, "showDeclined": &c.ShowDeclined,
		"nativeApps": &c.NativeApps, "onlyAccepted": &c.OnlyAccepted, "onlyWithOthers": &c.OnlyWithOthers,
		"calendarReminders": &c.CalendarReminders,
	}
	if p, ok := flags[key]; ok {
		return json.Unmarshal(value, p)
	}
	var n int
	var s string
	switch key {
	case "notifyMinutes", "syncMinutes":
		choices, p := notifyMinuteChoices, &c.NotifyMinutes
		if key == "syncMinutes" {
			choices, p = syncMinuteChoices, &c.SyncMinutes
		}
		if err := json.Unmarshal(value, &n); err != nil || !slices.Contains(choices, n) {
			return fmt.Errorf("%s: %s is not one of %v", key, value, choices)
		}
		*p = n
	case "theme":
		if err := json.Unmarshal(value, &s); err != nil || !slices.Contains(themeChoices, s) {
			return fmt.Errorf("theme: %s", value)
		}
		c.Theme = s
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}

func (c Config) save() error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	// Write-then-rename so a crash mid-write can't leave a truncated config.
	tmp := p + ".tmp"
	// 0600: the config will hold account details.
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
