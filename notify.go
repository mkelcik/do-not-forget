package main

import (
	"fmt"
	"strings"
	"time"
)

// dueNotifications returns events starting within `lead` that haven't been announced yet,
// and marks them in `sent`. Keyed by ID+start so a rescheduled meeting notifies again.
// Declined, all-day and already-started events are skipped.
func dueNotifications(events []Event, now time.Time, lead time.Duration, sent map[string]bool) []Event {
	var due []Event
	for _, e := range events {
		key := e.ID + "@" + e.Start.Format(time.RFC3339)
		if e.AllDay || e.Response == "no" || sent[key] || !e.Start.After(now) || e.Start.Sub(now) > lead {
			continue
		}
		sent[key] = true
		due = append(due, e)
	}
	return due
}

// notificationText is the title and body of a meeting reminder.
func notificationText(e Event, now time.Time) (title, body string) {
	title = fmt.Sprintf("%s · %s", e.Title, untilText(&e, now))
	parts := []string{e.Start.Format("15:04") + " – " + e.End.Format("15:04"), e.Calendar}
	if e.Location != "" {
		parts = append(parts, e.Location)
	}
	if e.Response == "pending" {
		parts = append(parts, "neodpovedané")
	}
	return title, strings.Join(parts, " · ")
}

// muteLabel describes an active mute for the menu and tooltip; "" when not muted.
func muteLabel(until, now time.Time) string {
	switch {
	case !now.Before(until):
		return ""
	case !until.Before(muteForever):
		return "Stlmené, kým ich nezapneš"
	case until.Hour() == 0 && until.Minute() == 0:
		return "Stlmené do konca dňa"
	default:
		return "Stlmené do " + until.Format("15:04")
	}
}
