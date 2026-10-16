package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config is the user's persisted settings, in <UserConfigDir>/do-not-forget/config.json.
type Config struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	// NotifyMinutes is how long before a meeting to send a reminder; 0 turns reminders off.
	NotifyMinutes int `json:"notifyMinutes"`
	// FullscreenAlert covers the screen when a meeting starts, for when a notification gets missed.
	FullscreenAlert bool `json:"fullscreenAlert"`
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

	c := Config{Width: 380, Height: 560, NotifyMinutes: 5, FullscreenAlert: true, TrayStyle: "both", TrayTitleMax: 25}
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if !validTrayStyle(c.TrayStyle) {
		c.TrayStyle = "both"
	}
	if !validTrayTitleMax(c.TrayTitleMax) {
		c.TrayTitleMax = 25
	}
	return c
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
