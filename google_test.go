package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

func TestGoogleEventMapping(t *testing.T) {
	var ge gEvent
	_ = json.Unmarshal([]byte(`{
		"id": "abc_20261016", "status": "confirmed", "summary": "Standup",
		"htmlLink": "https://www.google.com/calendar/event?eid=x",
		"start": {"dateTime": "2026-10-16T09:00:00+02:00"}, "end": {"dateTime": "2026-10-16T09:15:00+02:00"},
		"location": "Room 1", "description": "Agenda<br>1. <b>news</b> &amp; plans",
		"organizer": {"email": "boss@firma.sk"},
		"attendees": [
			{"email": "me@firma.sk", "self": true, "responseStatus": "tentative"},
			{"email": "eva@firma.sk", "displayName": "Eva", "responseStatus": "accepted"},
			{"email": "room@resource.calendar.google.com", "resource": true, "responseStatus": "accepted"}],
		"conferenceData": {"entryPoints": [
			{"entryPointType": "phone", "uri": "tel:+421"},
			{"entryPointType": "video", "uri": "https://meet.google.com/abc-defg-hij"}]}}`), &ge)
	e, ok := ge.event("me@firma.sk", Calendar{ID: "me@firma.sk", Name: "Práca", Color: "#0a84ff"})
	if !ok {
		t.Fatal("event skipped")
	}
	if e.ID != "me@firma.sk|me@firma.sk|abc_20261016" || e.Calendar != "Práca" || e.Color != "#0a84ff" {
		t.Errorf("id/calendar: %+v", e)
	}
	if want := time.Date(2026, 10, 16, 7, 0, 0, 0, time.UTC); !e.Start.Equal(want) || e.End.Sub(e.Start) != 15*time.Minute {
		t.Errorf("times: %v – %v", e.Start, e.End)
	}
	if e.Link != "https://meet.google.com/abc-defg-hij" || e.Response != "maybe" || e.Organizer != "boss@firma.sk" {
		t.Errorf("link/response/organizer: %+v", e)
	}
	if len(e.Attendees) != 2 || e.Attendees[1].Name != "Eva" || e.Attendees[1].Status != "yes" {
		t.Errorf("attendees (room skipped): %+v", e.Attendees)
	}
	if e.Description != "Agenda\n1. news & plans" {
		t.Errorf("description: %q", e.Description)
	}

	// All-day: dates at local midnight, end exclusive. No title, no guests, link in the location.
	ge = gEvent{}
	_ = json.Unmarshal([]byte(`{"id": "d", "start": {"date": "2026-10-16"}, "end": {"date": "2026-10-17"},
		"location": "https://firma.zoom.us/j/123?pwd=x"}`), &ge)
	e, _ = ge.event("a", Calendar{})
	if !e.AllDay || !e.Start.Equal(time.Date(2026, 10, 16, 0, 0, 0, 0, time.Local)) || e.Title != "(Bez názvu)" {
		t.Errorf("all-day: %+v", e)
	}
	if e.Link != "https://firma.zoom.us/j/123?pwd=x" || e.Response != "" {
		t.Errorf("zoom link / no RSVP: %+v", e)
	}

	for _, skip := range []string{`{"status": "cancelled"}`, `{"eventType": "workingLocation"}`} {
		var ge gEvent
		_ = json.Unmarshal([]byte(skip), &ge)
		if _, ok := ge.event("a", Calendar{}); ok {
			t.Errorf("%s should be skipped", skip)
		}
	}
}

func TestMeetingLink(t *testing.T) {
	for in, want := range map[string]string{
		"Join https://teams.microsoft.com/l/meetup-join/19%3ameeting/0 now": "https://teams.microsoft.com/l/meetup-join/19%3ameeting/0",
		"meet.google.com/abc-defg-hij without scheme":                       "",
		"see https://zoom.us/my/room.":                                      "https://zoom.us/my/room",
		"https://firma.zoom.us/j/1?pwd=x":                                   "https://firma.zoom.us/j/1?pwd=x",
		// Look-alike hosts are skipped for the real link after them.
		"https://attackerzoom.us/j/123 or https://zoom.us/j/9":        "https://zoom.us/j/9",
		"https://zoom.us.evil.com/j/1 https://meet.google.com.evil/x": "",
		"http://zoom.us/j/1 (not https)":                              "",
		"nothing here":                                                "",
	} {
		if got := meetingLink(in); got != want {
			t.Errorf("meetingLink(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMeetingServiceAndExternalURL(t *testing.T) {
	for link, want := range map[string]string{
		"https://meet.google.com/abc-defg-hij":                 "Meet",
		"https://us02web.zoom.us/j/123":                        "Zoom",
		"https://teams.live.com/meet/123":                      "Teams",
		"https://attackerzoom.us/j/123":                        "",
		"https://evil.com/zoom.us/j/1":                         "",
		"https://zoom.us@evil.com/j/1":                         "", // userinfo trick: the host is evil.com
		"https://teams.microsoft.com.evil.com/l/meetup-join/x": "",
	} {
		if got := meetingService(link); got != want {
			t.Errorf("meetingService(%q) = %q, want %q", link, got, want)
		}
	}
	for link, want := range map[string]bool{
		"https://x.com": true, "http://x.com": true,
		"file:///etc/passwd": false, "javascript:alert(1)": false, "zoommtg://zoom.us/join": false, "https:/no-host": false,
	} {
		if got := externalURL(link); got != want {
			t.Errorf("externalURL(%q) = %v", link, got)
		}
	}
}

// A refresh in flight while the account is disconnected must not put the token back.
func TestDisconnectStopsTokenSaving(t *testing.T) {
	keyring.MockInit()
	revoked := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		revoked = r.Form.Get("token")
	}))
	defer srv.Close()
	old := googleRevokeURL
	googleRevokeURL = srv.URL
	defer func() { googleRevokeURL = old }()

	tok := &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"}
	if err := saveToken("me@x.sk", tok); err != nil {
		t.Fatal(err)
	}
	refreshing, release := make(chan struct{}), make(chan struct{})
	ts := &savingTokenSource{email: "me@x.sk", tok: tok, base: tokenFunc(func() (*oauth2.Token, error) {
		close(refreshing)
		<-release // the refresh is still on the wire...
		return &oauth2.Token{AccessToken: "a2", RefreshToken: "r1"}, nil
	})}
	g := brokenGoogleSource("me@x.sk", nil)
	g.ts = ts
	done := make(chan error)
	go func() { _, err := ts.Token(); done <- err }()
	<-refreshing
	if err := g.close(true); err != nil { // ...when the account is disconnected
		t.Fatal(err)
	}
	close(release)
	if err := <-done; !errors.Is(err, errDisconnected) {
		t.Errorf("refresh after close: %v", err)
	}
	if _, err := loadToken("me@x.sk"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("token back in the keychain: %v", err)
	}
	if revoked != "r1" {
		t.Errorf("revoked %q, want the refresh token", revoked)
	}
	if g.life.Err() == nil {
		t.Error("requests in flight not cancelled")
	}
	if err := deleteToken("never-there@x.sk"); err != nil {
		t.Errorf("deleting a missing token: %v", err)
	}
}

// A keychain that refuses to delete is reported, not taken for success.
func TestDisconnectReportsKeychainFailure(t *testing.T) {
	keyring.MockInitWithError(errors.New("locked"))
	defer keyring.MockInit()
	g := brokenGoogleSource("me@x.sk", nil)
	if err := g.close(true); err == nil {
		t.Fatal("close succeeded with a failing keychain")
	}
	if _, errText := g.status(); !strings.Contains(errText, "locked") {
		t.Errorf("status error %q", errText)
	}
}

type tokenFunc func() (*oauth2.Token, error)

func (f tokenFunc) Token() (*oauth2.Token, error) { return f() }

// Respond reads the event and patches only my entry, keeping every other attendee field.
func TestGoogleRespond(t *testing.T) {
	var patched map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/calendars/me@firma.sk/events/ev1" {
			t.Errorf("path %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"attendees": [
				{"email": "me@firma.sk", "self": true, "responseStatus": "needsAction"},
				{"email": "eva@firma.sk", "responseStatus": "accepted", "optional": true, "comment": "late"}]}`)
		case http.MethodPatch:
			if r.URL.Query().Get("sendUpdates") != "none" {
				t.Errorf("sendUpdates %q", r.URL.Query().Get("sendUpdates"))
			}
			_ = json.NewDecoder(r.Body).Decode(&patched)
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()
	old := googleAPI
	googleAPI = srv.URL
	defer func() { googleAPI = old }()

	g := brokenGoogleSource("me@firma.sk", nil)
	g.client = srv.Client()
	if err := g.Respond(context.Background(), googleEventID("me@firma.sk", "me@firma.sk", "ev1"), "no"); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(patched)
	for _, want := range []string{`"responseStatus":"declined","self":true`, `"comment":"late"`, `"optional":true`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("patch %s lacks %s", b, want)
		}
	}
	if err := g.Respond(context.Background(), "dummy-1", "yes"); err == nil {
		t.Error("a non-Google ID must fail")
	}
}

func TestHTMLToText(t *testing.T) {
	for in, want := range map[string]string{
		"plain\ntext stays":                                 "plain\ntext stays",
		"Hi<br>line<br/><br><br><br>after &lt;gap&gt;":      "Hi\nline\n\nafter <gap>",
		"<p>one</p><p>two</p><ul><li>a</li><li>b</li></ul>": "one\ntwo\na\nb",
	} {
		if got := htmlToText(in); got != want {
			t.Errorf("htmlToText(%q) = %q, want %q", in, got, want)
		}
	}
}
