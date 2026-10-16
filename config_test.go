package main

import (
	"encoding/json"
	"testing"
)

func TestConfigNormalise(t *testing.T) {
	// Before the Notify switch, notifyMinutes 0 meant "reminders off".
	c := defaultConfig()
	_ = json.Unmarshal([]byte(`{"notifyMinutes": 0, "syncMinutes": 7, "theme": "pink"}`), &c)
	c.normalise()
	if c.Notify || c.NotifyMinutes != 5 || c.SyncMinutes != 1 || c.Theme != "" {
		t.Errorf("normalised: notify %v/%d, sync %d, theme %q", c.Notify, c.NotifyMinutes, c.SyncMinutes, c.Theme)
	}
	// A config from 0.8.x has no new keys: they take the defaults.
	c = defaultConfig()
	_ = json.Unmarshal([]byte(`{"notifyMinutes": 10, "fullscreenAlert": false}`), &c)
	c.normalise()
	if !c.Notify || c.NotifyMinutes != 10 || c.FullscreenAlert || !c.Sound || !c.WeekTab || !c.ShowDeclined {
		t.Errorf("0.8.x config: %+v", c)
	}
}

func TestConfigSet(t *testing.T) {
	c := defaultConfig()
	for key, value := range map[string]string{
		"notify": `false`, "notifyMinutes": `15`, "notifyAtStart": `true`, "onlyWithLink": `true`,
		"sound": `false`, "theme": `"dark"`, "weekTab": `false`, "showDeclined": `false`, "syncMinutes": `5`,
	} {
		if err := c.set(key, json.RawMessage(value)); err != nil {
			t.Errorf("set %s=%s: %v", key, value, err)
		}
	}
	s := c.settings()
	if s["notify"] != false || s["notifyMinutes"] != 15 || s["theme"] != "dark" || s["syncMinutes"] != 5 || s["onlyWithLink"] != true {
		t.Errorf("settings after set: %v", s)
	}
	for key, value := range map[string]string{
		"notifyMinutes": `7`, "syncMinutes": `"5"`, "theme": `"pink"`, "notify": `"yes"`, "width": `100`, "accounts": `[]`,
	} {
		if err := c.set(key, json.RawMessage(value)); err == nil {
			t.Errorf("set %s=%s accepted", key, value)
		}
	}
	if c.NotifyMinutes != 15 || c.Theme != "dark" {
		t.Errorf("a refused value changed the config: %+v", c)
	}
}
