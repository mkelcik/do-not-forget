package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// dummySource reads events from a JSON file on every fetch, so edits show up on the next refresh.
// Times are relative to app start, so "+12m" stays put instead of drifting with each reload.
type dummySource struct {
	path string
	base time.Time

	mu        sync.Mutex
	responses map[string]string // RSVPs made in the app by event or series ID, layered over the file
}

type dummyFile struct {
	Calendars map[string]string `json:"calendars"` // name -> color
	Events    []struct {
		Event
		Day int    `json:"day"` // 0 today, 1 tomorrow, -1 yesterday
		At  string `json:"at"`  // "HH:MM" on that day, or "+12m" / "-35m" from app start
		Dur string `json:"dur"` // Go duration, e.g. "15m", "1h30m"
	} `json:"events"`
}

func (d *dummySource) Events(_ context.Context, from, to time.Time) ([]Event, error) {
	raw, err := os.ReadFile(d.path)
	if err != nil {
		return nil, err
	}
	var f dummyFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", d.path, err)
	}
	var out []Event
	for i, fe := range f.Events {
		e := fe.Event
		e.ID = "dummy-" + strconv.Itoa(i)
		e.Color = f.Calendars[e.Calendar]
		if e.Start, e.End, err = d.span(fe.Day, fe.At, fe.Dur, e.AllDay); err != nil {
			return nil, fmt.Errorf("%s: event %q: %w", d.path, e.Title, err)
		}
		if e.CalendarURL == "" { // stand-in for Google's htmlLink: the day view on the web
			e.CalendarURL = e.Start.Format("https://calendar.google.com/calendar/r/day/2006/1/2")
		}
		d.mu.Lock()
		// Like Google: a series answer covers every occurrence, one answered on its own keeps that.
		if r, ok := d.responses[e.SeriesID]; ok && e.SeriesID != "" {
			e.Response = r
		}
		if r, ok := d.responses[e.ID]; ok {
			e.Response = r
		}
		d.mu.Unlock()
		for j := range e.Attendees {
			if e.Attendees[j].Self && e.Response != "" {
				e.Attendees[j].Status = e.Response
			}
		}
		if e.Start.Before(to) && e.End.After(from) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (d *dummySource) span(day int, at, dur string, allDay bool) (start, end time.Time, err error) {
	y, m, dd := d.base.Date()
	midnight := time.Date(y, m, dd+day, 0, 0, 0, 0, d.base.Location())
	if allDay {
		return midnight, midnight.AddDate(0, 0, 1), nil
	}
	if strings.HasPrefix(at, "+") || strings.HasPrefix(at, "-") {
		off, err := time.ParseDuration(at)
		if err != nil {
			return start, end, err
		}
		start = d.base.Add(off).Truncate(time.Minute)
	} else {
		t, err := time.Parse("15:04", at)
		if err != nil {
			return start, end, err
		}
		start = midnight.Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute)
	}
	length, err := time.ParseDuration(dur)
	if err != nil {
		return start, end, err
	}
	return start, start.Add(length), nil
}

func (d *dummySource) Respond(_ context.Context, eventID, response string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.responses == nil {
		d.responses = map[string]string{}
	}
	d.responses[eventID] = response
	return nil
}
