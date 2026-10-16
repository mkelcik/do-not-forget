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

// withLink keeps the events that have a meeting link ("only meetings with a link").
func withLink(events []Event) []Event {
	return slices.DeleteFunc(slices.Clone(events), func(e Event) bool { return e.Link == "" })
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
