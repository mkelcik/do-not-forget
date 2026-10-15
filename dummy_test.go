package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDummySourceAndMerge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.json")
	os.WriteFile(path, []byte(`{"calendars":{"W":"#00f"},"events":[
		{"calendar":"W","title":"later","at":"+30m","dur":"15m"},
		{"calendar":"W","title":"clock","day":1,"at":"09:30","dur":"1h"},
		{"calendar":"W","title":"bday","allDay":true},
		{"calendar":"W","title":"past","at":"-2h","dur":"30m"}]}`), 0o644)

	base := time.Date(2026, 10, 15, 12, 0, 30, 0, time.Local)
	src := &dummySource{path: path, base: base}
	evs := fetchAll(context.Background(), []Source{src}, base.Add(-time.Hour), base.AddDate(0, 0, 2))

	if len(evs) != 3 { // "past" ended before `from`
		t.Fatalf("got %d events: %+v", len(evs), evs)
	}
	if evs[0].Title != "bday" || evs[1].Title != "later" || evs[2].Title != "clock" {
		t.Fatalf("bad order: %v %v %v", evs[0].Title, evs[1].Title, evs[2].Title)
	}
	if want := time.Date(2026, 10, 15, 12, 30, 0, 0, time.Local); !evs[1].Start.Equal(want) {
		t.Errorf("relative start = %v, want %v", evs[1].Start, want)
	}
	if want := time.Date(2026, 10, 16, 10, 30, 0, 0, time.Local); !evs[2].End.Equal(want) {
		t.Errorf("clock end = %v, want %v", evs[2].End, want)
	}
	if evs[1].Color != "#00f" {
		t.Errorf("color = %q", evs[1].Color)
	}
	if n := nextEvent(evs, base); n == nil || n.Title != "later" {
		t.Errorf("nextEvent = %+v", n)
	}

	// Declining "later" makes "clock" the next one.
	src.Respond(context.Background(), evs[1].ID, "no")
	evs = fetchAll(context.Background(), []Source{src}, base.Add(-time.Hour), base.AddDate(0, 0, 2))
	if n := nextEvent(evs, base); n == nil || n.Title != "clock" {
		t.Errorf("after decline nextEvent = %+v", n)
	}
}
