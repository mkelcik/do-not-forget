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

func TestRemindable(t *testing.T) {
	me, guest := Attendee{Name: "me", Self: true}, Attendee{Name: "guest"}
	evs := []Event{
		{ID: "link", Link: "https://meet.google.com/abc-defg-hij", Response: "yes", Attendees: []Attendee{me, guest}},
		{ID: "nolink", Response: "yes", Attendees: []Attendee{me, guest}},
		{ID: "pending", Link: "https://zoom.us/j/1", Response: "pending", Attendees: []Attendee{me, guest}},
		{ID: "maybe", Link: "https://zoom.us/j/1", Response: "maybe", Attendees: []Attendee{me, guest}},
		{ID: "alone", Link: "https://zoom.us/j/1"},
		{ID: "muted", Link: "https://zoom.us/j/1", Muted: true, Attendees: []Attendee{me, guest}},
	}
	ids := func(c Config) (out []string) {
		for _, e := range c.remindable(evs) {
			out = append(out, e.ID)
		}
		return out
	}
	for _, c := range []struct {
		cfg  Config
		want string
	}{
		{Config{}, "link nolink pending maybe alone"},
		{Config{OnlyWithLink: true}, "link pending maybe alone"},
		{Config{OnlyAccepted: true}, "link nolink alone"},
		{Config{OnlyWithOthers: true}, "link nolink pending maybe"},
	} {
		if got := strings.Join(ids(c.cfg), " "); got != c.want {
			t.Errorf("%+v: %q, want %q", c.cfg, got, c.want)
		}
	}
	if len(evs) != 6 || evs[1].ID != "nolink" {
		t.Errorf("input changed: %+v", evs)
	}
}

func TestDueReminders(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	ev := func(id string, in time.Duration, reminders []int) Event {
		return Event{ID: id, Start: now.Add(in), End: now.Add(in + 30*time.Minute), Reminders: reminders}
	}
	events := []Event{
		ev("own", 40*time.Minute, []int{30, 10}),
		ev("none", 4*time.Minute, []int{}), // the calendar has no reminder for it
		ev("unknown", 4*time.Minute, nil),  // the calendar doesn't say: the app's time
		ev("atstart", 0, []int{0}),
	}
	ids := func(evs []Event) string {
		out := make([]string, 0, len(evs))
		for _, e := range evs {
			out = append(out, e.ID)
		}
		return strings.Join(out, " ")
	}
	sent := map[string]bool{}
	if got := ids(dueReminders(events, now, 5*time.Minute, true, false, sent)); got != "unknown atstart" {
		t.Fatalf("now: %q", got)
	}
	if got := ids(dueReminders(events, now.Add(10*time.Minute), 5*time.Minute, true, false, sent)); got != "own" {
		t.Fatalf("30 min before: %q", got)
	}
	if got := ids(dueReminders(events, now.Add(31*time.Minute), 5*time.Minute, true, false, sent)); got != "own" {
		t.Fatalf("10 min before: %q", got)
	}
	if got := ids(dueReminders(events, now.Add(32*time.Minute), 5*time.Minute, true, false, sent)); got != "" {
		t.Fatalf("twice: %q", got)
	}
	// Both of its times passed while the app wasn't running: one reminder, not two.
	sent = map[string]bool{}
	if got := ids(dueReminders(events[:1], now.Add(35*time.Minute), 5*time.Minute, true, false, sent)); got != "own" {
		t.Fatalf("catching up: %q", got)
	}
	if got := ids(dueReminders(events[:1], now.Add(36*time.Minute), 5*time.Minute, true, false, sent)); got != "" {
		t.Fatalf("catching up twice: %q", got)
	}
	// Without the setting every event gets the app's time.
	sent = map[string]bool{}
	if got := ids(dueReminders(events, now, 5*time.Minute, false, false, sent)); got != "none unknown atstart" {
		t.Fatalf("app's time: %q", got)
	}
	// Just after the start: a reminder before it is late, one at the start isn't yet.
	started := []Event{ev("before", -10*time.Second, []int{10}), ev("at", -10*time.Second, []int{0})}
	if got := ids(dueReminders(started, now, 5*time.Minute, true, false, map[string]bool{})); got != "at" {
		t.Fatalf("just started: %q", got)
	}
	// "At the start too" covers a calendar reminder at 0 min: no second notification.
	if got := ids(dueReminders(events, now, 5*time.Minute, true, true, map[string]bool{})); got != "unknown" {
		t.Fatalf("with the start notification: %q", got)
	}
	forgetSent(sent, events[1])
	if got := ids(dueReminders(events, now, 5*time.Minute, false, false, sent)); got != "none" {
		t.Fatalf("after forgetSent: %q", got)
	}
}

func TestDueSnoozes(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	ev := func(id string) Event { return Event{ID: id, Start: now.Add(-time.Minute), End: now.Add(time.Hour)} }
	events := []Event{ev("a"), ev("b"), {ID: "over", End: now}, {ID: "declined", End: now.Add(time.Hour), Response: "no"},
		{ID: "muted", End: now.Add(time.Hour), Muted: true}}
	snoozes := []snoozed{
		{id: "a", at: now, alert: true},
		{id: "b", at: now.Add(-time.Second)},
		{id: "a", at: now.Add(time.Minute)}, // not yet
		{id: "over", at: now}, {id: "declined", at: now}, {id: "muted", at: now}, {id: "gone", at: now},
	}
	ids := func(evs []Event) string {
		out := make([]string, 0, len(evs))
		for _, e := range evs {
			out = append(out, e.ID)
		}
		return strings.Join(out, " ")
	}
	on := Config{Notify: true, FullscreenAlert: true}
	left, alerts, reminders := dueSnoozes(snoozes, on, events, now)
	if len(left) != 1 || left[0].at != now.Add(time.Minute) || ids(alerts) != "a" || ids(reminders) != "b" {
		t.Errorf("due: left %+v, alerts %q, reminders %q", left, ids(alerts), ids(reminders))
	}
	// Switched off in Settings after snoozing: dropped, not delivered.
	left, alerts, reminders = dueSnoozes(snoozes, Config{}, events, now)
	if len(left) != 1 || len(alerts) != 0 || len(reminders) != 0 {
		t.Errorf("switched off: left %+v, alerts %q, reminders %q", left, ids(alerts), ids(reminders))
	}
	muted := on
	muted.MutedUntil = now.Add(time.Hour)
	if _, alerts, reminders = dueSnoozes(snoozes, muted, events, now); len(alerts)+len(reminders) != 0 {
		t.Errorf("all muted: alerts %q, reminders %q", ids(alerts), ids(reminders))
	}
}

func TestMergeAlerts(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	starting := []Event{{ID: "new", Start: now}}
	snoozed := []Event{{ID: "old", Start: now.Add(-5 * time.Minute)}, {ID: "new", Start: now}}
	got := mergeAlerts(starting, snoozed)
	if len(got) != 2 || got[0].ID != "old" || got[1].ID != "new" {
		t.Errorf("merged: %+v", got)
	}
	if mergeAlerts(nil, nil) != nil {
		t.Error("nothing due must stay empty")
	}
}
