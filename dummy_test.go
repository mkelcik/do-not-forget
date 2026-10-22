package main

import (
	"cmp"
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

	if evs := fetchAll(context.Background(), nil, time.Now(), time.Now()); evs == nil {
		t.Fatal("fetchAll with nothing must be [] not nil (UI expects an array)")
	}

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

// A series answer reaches every loaded occurrence, also one answered on its own before.
func TestAnswerSeries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.json")
	os.WriteFile(path, []byte(`{"events":[
		{"title":"1:1","seriesId":"s","at":"+1h","dur":"30m","response":"pending"},
		{"title":"1:1","seriesId":"s","day":1,"at":"10:00","dur":"30m","response":"pending"},
		{"title":"other","at":"+2h","dur":"30m","response":"pending"}]}`), 0o644)
	base := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	from, to := base, base.AddDate(0, 0, 2)
	src := &dummySource{path: path, base: base}
	evs, _ := src.Events(context.Background(), from, to)

	src.Respond(context.Background(), evs[0].ID, "maybe") // an exception, as in Google
	if err := answerSeries(context.Background(), src, evs[0].SeriesID, "yes", from, to); err != nil {
		t.Fatal(err)
	}
	evs, _ = src.Events(context.Background(), from, to)
	if evs[0].Response != "yes" || evs[1].Response != "yes" || evs[2].Response != "pending" {
		t.Errorf("responses %q %q %q, want yes yes pending", evs[0].Response, evs[1].Response, evs[2].Response)
	}
}

// notInvitedSource has me on single occurrences only, not on the series.
type notInvitedSource struct{ answered map[string]string }

func (n *notInvitedSource) Events(context.Context, time.Time, time.Time) ([]Event, error) {
	return []Event{{ID: "occ", SeriesID: "s", Response: cmp.Or(n.answered["occ"], "pending")}}, nil
}

func (n *notInvitedSource) Respond(_ context.Context, id, response string) error {
	if id == "s" {
		return errNotInvited
	}
	n.answered[id] = response
	return nil
}

// Invited to one occurrence only: the series can't be answered, the occurrence still is, no error.
func TestAnswerSeriesNotInvited(t *testing.T) {
	src := &notInvitedSource{answered: map[string]string{}}
	if err := answerSeries(context.Background(), src, "s", "no", time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if src.answered["occ"] != "no" {
		t.Errorf("occurrence answered %q, want no", src.answered["occ"])
	}
}

// showcase.json feeds the website screenshots (make showcase); it must keep loading.
func TestShowcaseLoads(t *testing.T) {
	src := &dummySource{path: "showcase.json", base: time.Date(2026, 10, 16, 11, 0, 0, 0, time.Local)}
	evs, err := src.Events(context.Background(), time.Date(2026, 10, 16, 0, 0, 0, 0, time.Local), time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local))
	if err != nil || len(evs) < 15 {
		t.Fatalf("%d events, %v", len(evs), err)
	}
}
