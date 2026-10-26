package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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

func TestEventRules(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	evs := []Event{{ID: "a"}, {ID: "b1", SeriesID: "b"}, {ID: "b2", SeriesID: "b"}, {ID: "c"}, {ID: "d", Muted: true}}
	var c Config
	c.setRule(EventRule{ID: "a", Until: now.Add(time.Hour)})
	c.setRule(EventRule{ID: "b", Hide: true})
	got := func() string {
		out := make([]string, 0, len(evs))
		for _, e := range c.applyRules(evs) {
			if e.Muted {
				e.ID += "(muted)"
			}
			out = append(out, e.ID)
		}
		return strings.Join(out, " ")
	}
	if g := got(); g != "a(muted) c d(muted)" {
		t.Errorf("rules: %q", g)
	}
	// Muting what was hidden replaces the rule.
	c.setRule(EventRule{ID: "b"})
	if g := got(); g != "a(muted) b1(muted) b2(muted) c d(muted)" || len(c.EventRules) != 2 {
		t.Errorf("replaced: %q, %+v", g, c.EventRules)
	}
	c.dropRule("a")
	if g := got(); g != "a b1(muted) b2(muted) c d(muted)" {
		t.Errorf("dropped: %q", g)
	}
}

func TestEventRulesOverlap(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	evs := []Event{{ID: "b1", SeriesID: "b"}, {ID: "b2", SeriesID: "b"}}
	shown := func(c Config) string {
		out := make([]string, 0, len(evs))
		for _, e := range c.applyRules(evs) {
			if e.Muted {
				e.ID += "(muted)"
			}
			out = append(out, e.ID)
		}
		return strings.Join(out, " ")
	}
	// The series muted, then one occurrence hidden: that one goes, whichever rule is first.
	var c Config
	c.setRule(EventRule{ID: "b"})
	c.setRule(EventRule{ID: "b1", Hide: true, Until: now.Add(time.Hour)})
	if g := shown(c); g != "b2(muted)" {
		t.Errorf("series muted, occurrence hidden: %q", g)
	}
	// One occurrence muted, then the series hidden: all of it goes.
	c = Config{}
	c.setRule(EventRule{ID: "b1", Until: now.Add(time.Hour)})
	c.setRule(EventRule{ID: "b", Hide: true})
	if g := shown(c); g != "" {
		t.Errorf("occurrence muted, series hidden: %q", g)
	}
}

func TestExpireRules(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	c := Config{EventRules: []EventRule{
		{ID: "over", Until: now.Add(-time.Minute)},
		{ID: "moved", Until: now.Add(-time.Minute)}, // rescheduled while the computer slept
		{ID: "later", Until: now.Add(time.Minute)},  // not in this fetch: keeps its end
		{ID: "series"},
	}}
	fetched := []Event{{ID: "moved", End: now.Add(48 * time.Hour)}, {ID: "x", SeriesID: "series", End: now}}
	if !c.expireRules(fetched, now) || len(c.EventRules) != 3 {
		t.Fatalf("expired: %+v", c.EventRules)
	}
	if r := c.EventRules[0]; r.ID != "moved" || !r.Until.Equal(now.Add(48*time.Hour)) {
		t.Errorf("moved: %+v", r)
	}
	if r := c.EventRules[1]; r.ID != "later" || !r.Until.Equal(now.Add(time.Minute)) || !c.EventRules[2].Until.IsZero() {
		t.Errorf("kept: %+v", c.EventRules)
	}
	if c.expireRules(fetched, now) { // nothing changed: no needless write
		t.Error("expired twice")
	}
	if !c.expireRules(fetched, now.Add(time.Hour)) || len(c.EventRules) != 2 || c.EventRules[1].ID != "series" {
		t.Errorf("an hour later: %+v", c.EventRules)
	}
}

func TestFetchDays(t *testing.T) {
	if d := (Config{}).fetchDays(); d != 7 {
		t.Errorf("default: %d", d)
	}
	// A popup reminder can be up to 4 weeks (40320 min) before the event.
	if d := (Config{CalendarReminders: true}).fetchDays(); time.Duration(d)*24*time.Hour < 40320*time.Minute+24*time.Hour {
		t.Errorf("calendar reminders: %d days", d)
	}
}
