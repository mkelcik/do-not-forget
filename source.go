package main

import (
	"context"
	"errors"
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
	Service     string     `json:"service,omitempty"`     // Meet | Zoom | Teams, by the link's host; "" otherwise
	CalendarURL string     `json:"calendarUrl,omitempty"` // the event in the provider's web UI
	Attendees   []Attendee `json:"attendees,omitempty"`
	// Response is my RSVP: pending | yes | maybe | no; empty when there's nothing to answer
	// (my own event, no guest list).
	Response string `json:"response,omitempty"`
	// SeriesID is the whole recurring series, in the same form as ID; empty for one-off events.
	// Respond takes it in place of ID to answer the series; see answerSeries.
	SeriesID string `json:"seriesId,omitempty"`
	// Muted events show in the agenda but get no reminders or alert: their calendar is set so in
	// Settings, or the user muted the event or its series (see EventRule).
	Muted bool `json:"muted,omitempty"`
	// Reminders are the event's own reminder times in minutes before the start, as the calendar
	// has them; nil when the source doesn't say, empty when the event has none.
	Reminders []int `json:"reminders,omitempty"`

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

// errNotInvited: I'm not on the event's guest list, e.g. invited to one occurrence of a series
// but not to the series itself.
var errNotInvited = errors.New("not invited to this event")

// answerSeries answers a whole series, then every occurrence in [from, to) that still has another
// answer: occurrences answered on their own don't follow the series (Google keeps them as
// exceptions), and with an invitation to single occurrences there's no series to answer at all.
// Occurrences further out than `to` keep such answers.
func answerSeries(ctx context.Context, src Source, seriesID, response string, from, to time.Time) error {
	errSeries := src.Respond(ctx, seriesID, response)
	if errors.Is(errSeries, errNotInvited) {
		errSeries = nil // the occurrences below are all there is to answer
	}
	evs, err := src.Events(ctx, from, to)
	if err != nil {
		return errors.Join(errSeries, err)
	}
	errs := []error{errSeries}
	for _, e := range evs {
		if e.SeriesID == seriesID && e.Response != "" && e.Response != response {
			errs = append(errs, src.Respond(ctx, e.ID, response))
		}
	}
	return errors.Join(errs...)
}

// fetchAll merges every source into one list sorted by start.
// A failing source is logged and skipped so one broken account doesn't blank the rest.
func fetchAll(ctx context.Context, sources []Source, from, to time.Time) []Event {
	all := []Event{} // non-nil: the UI gets [] rather than null when there's nothing
	for _, s := range sources {
		evs, err := s.Events(ctx, from, to)
		if err != nil {
			log.Printf("source %T: %v", s, err)
			continue
		}
		for i := range evs {
			e := &evs[i]
			e.src = s
			// Only http(s) reaches the UI's buttons, and the service name comes from the host,
			// never from the link merely containing "zoom".
			if !externalURL(e.Link) {
				e.Link = ""
			}
			if !externalURL(e.CalendarURL) {
				e.CalendarURL = ""
			}
			e.Service = meetingService(e.Link)
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
