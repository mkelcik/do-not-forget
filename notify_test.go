package main

import (
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

	if due := dueNotifications(events, now, 5*time.Minute, sent); len(due) != 1 || due[0].ID != "soon" {
		t.Fatalf("first pass: %+v", due)
	}
	if due := dueNotifications(events, now.Add(time.Minute), 5*time.Minute, sent); len(due) != 0 {
		t.Fatalf("notified twice: %+v", due)
	}
	if due := dueNotifications(events, now.Add(16*time.Minute), 5*time.Minute, sent); len(due) != 1 || due[0].ID != "later" {
		t.Fatalf("later: %+v", due)
	}
	// Moved meeting = new start = new reminder.
	events[0].Start = events[0].Start.Add(time.Hour)
	if due := dueNotifications(events, now.Add(62*time.Minute), 5*time.Minute, sent); len(due) != 1 || due[0].ID != "soon" {
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
