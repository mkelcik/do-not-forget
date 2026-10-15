package main

import (
	"context"
	"log"
	"sort"
	"time"
)

// Event is one calendar entry, normalised across sources. JSON tags are what the frontend reads.
type Event struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Calendar    string     `json:"calendar"`
	Color       string     `json:"color"`
	Start       time.Time  `json:"start"`
	End         time.Time  `json:"end"`
	AllDay      bool       `json:"allDay,omitempty"`
	Location    string     `json:"location,omitempty"`
	Description string     `json:"description,omitempty"`
	Organizer   string     `json:"organizer,omitempty"`
	Link        string     `json:"link,omitempty"`        // meeting URL (Meet, Zoom, Teams, ...)
	CalendarURL string     `json:"calendarUrl,omitempty"` // the event in the provider's web UI
	Attendees   []Attendee `json:"attendees,omitempty"`
	// Response is my RSVP: pending | yes | maybe | no; empty when there's nothing to answer
	// (my own event, no guest list).
	Response string `json:"response,omitempty"`

	src Source // which source produced it, so RSVPs go back to the right account
}

type Attendee struct {
	Name   string `json:"name"`
	Status string `json:"status"` // pending | yes | maybe | no
	Self   bool   `json:"self,omitempty"`
}

// Source is a calendar provider: dummy for development, Google, later others.
type Source interface {
	// Events returns events overlapping [from, to).
	Events(ctx context.Context, from, to time.Time) ([]Event, error)
	// Respond sets my RSVP (yes | maybe | no) on the event.
	Respond(ctx context.Context, eventID, response string) error
}

// fetchAll merges every source into one list sorted by start.
// A failing source is logged and skipped so one broken account doesn't blank the rest.
func fetchAll(ctx context.Context, sources []Source, from, to time.Time) []Event {
	var all []Event
	for _, s := range sources {
		evs, err := s.Events(ctx, from, to)
		if err != nil {
			log.Printf("source %T: %v", s, err)
			continue
		}
		for i := range evs {
			evs[i].src = s
		}
		all = append(all, evs...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Start.Before(all[j].Start) })
	return all
}

// nextEvent is the running or upcoming timed event I haven't declined, nil if none.
func nextEvent(events []Event, now time.Time) *Event {
	for i, e := range events {
		if !e.AllDay && e.Response != "no" && e.End.After(now) {
			return &events[i]
		}
	}
	return nil
}
