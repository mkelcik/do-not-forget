package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the user's persisted settings, in <UserConfigDir>/do-not-forget/config.json.
type Config struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "do-not-forget", "config.json")
}

// loadConfig never fails: a missing or broken file just means defaults.
func loadConfig() Config {
	c := Config{Width: 380, Height: 560}
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func (c Config) save() error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	// Write-then-rename so a crash mid-write can't leave a truncated config.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
