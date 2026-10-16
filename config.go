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
	// Sound plays the system sound with notifications.
	Sound bool `json:"sound"`
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
}

func validTrayStyle(s string) bool { return s == "icon" || s == "text" || s == "both" }

func validTrayTitleMax(n int) bool { return n >= 10 && n <= 100 }

// muteForever is "until I turn it back on".
var muteForever = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

func (c Config) mutedAt(now time.Time) bool { return now.Before(c.MutedUntil) }

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
	return Config{Width: 380, Height: 560, Notify: true, NotifyMinutes: 5, FullscreenAlert: true, Sound: true,
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
		"fullscreenAlert": c.FullscreenAlert, "onlyWithLink": c.OnlyWithLink, "sound": c.Sound,
		"theme": c.Theme, "weekTab": c.WeekTab, "showDeclined": c.ShowDeclined, "syncMinutes": c.SyncMinutes,
	}
}

// set changes one of settings() from the page, refusing unknown keys and values the UI doesn't offer.
func (c *Config) set(key string, value json.RawMessage) error {
	flags := map[string]*bool{
		"notify": &c.Notify, "notifyAtStart": &c.NotifyAtStart, "fullscreenAlert": &c.FullscreenAlert,
		"onlyWithLink": &c.OnlyWithLink, "sound": &c.Sound, "weekTab": &c.WeekTab, "showDeclined": &c.ShowDeclined,
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
