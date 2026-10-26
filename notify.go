package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// dueNotifications returns events starting between now-grace and now+lead that haven't been
// announced yet, and marks them in `sent`. The grace lets a start alert still fire when the
// laptop wakes just after a meeting began. Declined and all-day events are skipped.
func dueNotifications(events []Event, now time.Time, lead, grace time.Duration, sent map[string]bool) []Event {
	var due []Event
	for _, e := range events {
		key := sentKey(e)
		if e.AllDay || e.Response == "no" || sent[key] || now.Sub(e.Start) > grace || e.Start.Sub(now) > lead {
			continue
		}
		sent[key] = true
		due = append(due, e)
	}
	return due
}

// dueReminders returns the events with a reminder due: at `lead` before the start, or with
// fromCalendar at each of the event's own reminder times, when its calendar has them. Each time
// fires once (marked in `sent`); several due at once, say after the app starts, make one
// reminder. With atStart a separate notification covers the start, so a calendar reminder at
// 0 min is left to it. Declined and all-day events are skipped.
func dueReminders(events []Event, now time.Time, lead time.Duration, fromCalendar, atStart bool, sent map[string]bool) []Event {
	var due []Event
	for _, e := range events {
		leads := []time.Duration{lead}
		if fromCalendar && e.Reminders != nil {
			leads = leads[:0]
			for _, m := range e.Reminders {
				if m > 0 || !atStart {
					leads = append(leads, time.Duration(m)*time.Minute)
				}
			}
		}
		fire := false
		for _, l := range leads {
			key := sentKey(e) + "-" + l.String()
			// A reminder before the start is late once the meeting began; one "at the start"
			// (0 min) still counts 15s after, so it isn't missed between two ticks.
			late := time.Duration(0)
			if l == 0 {
				late = 15 * time.Second
			}
			if e.AllDay || e.Response == "no" || sent[key] || now.Sub(e.Start) > late || e.Start.Sub(now) > l {
				continue
			}
			sent[key] = true
			fire = true
		}
		if fire {
			due = append(due, e)
		}
	}
	return due
}

// snoozed is a reminder or alert put off until `at`; it then comes back for the same event.
type snoozed struct {
	id    string
	at    time.Time
	alert bool // comes back as the alert, else as a notification
}

// dueSnoozes splits off the snoozes whose time has come. They come back with the event as it is
// now, and only if it would still get one: not if it was declined, muted, hidden or filtered
// out, is over, everything is muted, or Settings turned that kind of reminder off since.
func dueSnoozes(snoozes []snoozed, c Config, events []Event, now time.Time) (left []snoozed, alerts, reminders []Event) {
	live := c.remindable(events)
	for _, s := range snoozes {
		if now.Before(s.at) {
			left = append(left, s)
			continue
		}
		i := slices.IndexFunc(live, func(e Event) bool { return e.ID == s.id })
		switch {
		case i < 0 || live[i].Response == "no" || !live[i].End.After(now) || c.mutedAt(now):
		case s.alert && c.FullscreenAlert:
			alerts = append(alerts, live[i])
		case !s.alert && c.Notify:
			reminders = append(reminders, live[i])
		}
	}
	return left, alerts, reminders
}

// mergeAlerts joins two lists of alerts for one window, each meeting once, sorted by start.
func mergeAlerts(a, b []Event) []Event {
	out := slices.Clone(a)
	for _, e := range b {
		if !slices.ContainsFunc(out, func(x Event) bool { return x.ID == e.ID }) {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(x, y Event) int { return x.Start.Compare(y.Start) })
	return out
}

// forgetSent lets e's reminders fire again, e.g. after sending one failed.
func forgetSent(sent map[string]bool, e Event) {
	for k := range sent {
		if strings.HasPrefix(k, sentKey(e)) {
			delete(sent, k)
		}
	}
}

// sentKey is ID+start, so a rescheduled meeting notifies again.
func sentKey(e Event) string { return e.ID + "@" + e.Start.Format(time.RFC3339) }

// notificationText is the title and body of a meeting reminder.
func notificationText(e Event, now time.Time) (title, body string) {
	title = fmt.Sprintf("%s · %s", e.Title, untilText(&e, now))
	parts := []string{e.Start.Format("15:04") + " – " + e.End.Format("15:04"), e.Calendar}
	if e.Location != "" {
		parts = append(parts, e.Location)
	}
	if e.Response == "pending" {
		parts = append(parts, T("notify.unanswered"))
	}
	return title, strings.Join(parts, " · ")
}

// muteLabel describes an active mute for the menu and tooltip; "" when not muted.
func muteLabel(until, now time.Time) string {
	switch {
	case !now.Before(until):
		return ""
	case !until.Before(muteForever):
		return T("mute.forever")
	case until.Hour() == 0 && until.Minute() == 0:
		return T("mute.end_of_day")
	default:
		return T("mute.until", "time", until.Format("15:04"))
	}
}
