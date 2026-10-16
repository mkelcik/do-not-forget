package main

import (
	"strings"
	"testing"
	"time"
)

func TestDueNotifications(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	ev := func(id string, in time.Duration, resp string) Event {
		return Event{ID: id, Start: now.Add(in), End: now.Add(in + 30*time.Minute), Response: resp}
	}
	events := []Event{
		ev("soon", 4*time.Minute, "yes"),
		ev("later", 20*time.Minute, ""),
		ev("declined", 3*time.Minute, "no"),
		ev("running", -time.Minute, ""),
		{ID: "allday", AllDay: true, Start: now.Add(time.Minute), End: now.Add(24 * time.Hour)},
	}
	sent := map[string]bool{}

	if due := dueNotifications(events, now, 5*time.Minute, 0, sent); len(due) != 1 || due[0].ID != "soon" {
		t.Fatalf("first pass: %+v", due)
	}
	if due := dueNotifications(events, now.Add(time.Minute), 5*time.Minute, 0, sent); len(due) != 0 {
		t.Fatalf("notified twice: %+v", due)
	}
	if due := dueNotifications(events, now.Add(16*time.Minute), 5*time.Minute, 0, sent); len(due) != 1 || due[0].ID != "later" {
		t.Fatalf("later: %+v", due)
	}
	// Already started: skipped without grace, caught within it (wake from sleep), once.
	started := []Event{ev("woke", -90*time.Second, "")}
	if due := dueNotifications(started, now, 15*time.Second, 0, sent); len(due) != 0 {
		t.Fatalf("no grace: %+v", due)
	}
	if due := dueNotifications(started, now, 15*time.Second, 3*time.Minute, sent); len(due) != 1 {
		t.Fatalf("grace: %+v", due)
	}
	if due := dueNotifications(started, now.Add(time.Minute), 15*time.Second, 3*time.Minute, sent); len(due) != 0 {
		t.Fatalf("grace twice: %+v", due)
	}

	// Moved meeting = new start = new reminder.
	events[0].Start = events[0].Start.Add(time.Hour)
	if due := dueNotifications(events, now.Add(62*time.Minute), 5*time.Minute, 0, sent); len(due) != 1 || due[0].ID != "soon" {
		t.Fatalf("rescheduled: %+v", due)
	}
}

func TestMuteLabel(t *testing.T) {
	now := time.Date(2026, 10, 15, 14, 30, 0, 0, time.Local)
	for until, want := range map[time.Time]string{
		{}:                    "",
		now.Add(-time.Minute): "",
		now.Add(time.Hour):    "Stlmené do 15:30",
		time.Date(2026, 10, 16, 0, 0, 0, 0, time.Local): "Stlmené do konca dňa",
		muteForever: "Stlmené, kým ich nezapneš",
	} {
		if got := muteLabel(until, now); got != want {
			t.Errorf("muteLabel(%v) = %q, want %q", until, got, want)
		}
	}
}

func TestUntilTextDays(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Bratislava")
	if err != nil {
		t.Skip(err)
	}
	now := time.Date(2026, 3, 28, 23, 0, 0, 0, loc) // clocks go forward the next night
	for start, want := range map[time.Time]string{
		now.Add(30 * time.Minute):   "o 30 min",
		now.Add(90 * time.Minute):   "zajtra",
		now.Add(25 * time.Hour):     "o 2 dni",
		now.Add(6 * 24 * time.Hour): "o 7 dní", // Apr 4 00:00: the lost hour moves it a day
	} {
		e := Event{Start: start, End: start.Add(time.Hour)}
		if got := untilText(&e, now); !strings.EqualFold(got, want) {
			t.Errorf("untilText(%s) = %q, want %q", start, got, want)
		}
	}
}

func TestTruncateTitle(t *testing.T) {
	for _, c := range []struct {
		in   string
		max  int
		want string
	}{
		{"Standup", 25, "Standup"},
		{"Everpure Video Interview - Michal Kelčík", 25, "Everpure Video Interview…"},
		{"Team Astartes - weekly sync", 15, "Team Astartes…"}, // no dangling " -"
		{"Čľťžý ďáľšíé", 6, "Čľťžý…"},                         // runes, not bytes
	} {
		if got := truncateTitle(c.in, c.max); got != c.want {
			t.Errorf("truncateTitle(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}
