package main

import (
	"runtime"
	"testing"
)

func TestNativeMeetingURL(t *testing.T) {
	for link, want := range map[string]string{
		"https://zoom.us/j/123456?pwd=abc":                                                      "zoommtg://zoom.us/join?action=join&confno=123456&pwd=abc",
		"https://purestorage.zoom.us/j/98531439918":                                             "zoommtg://purestorage.zoom.us/join?action=join&confno=98531439918",
		"https://us02web.zoom.us/w/555?tk=x&pwd=p%2F1":                                          "zoommtg://us02web.zoom.us/join?action=join&confno=555&pwd=p%2F1",
		"https://teams.microsoft.com/l/meetup-join/19%3ameeting_x%40thread.v2/0?context=%7b%7d": "msteams:/l/meetup-join/19%3ameeting_x%40thread.v2/0?context=%7b%7d",
		// No native form: the browser opens these.
		"https://zoom.us/my/room":                                 "", // personal link, no meeting number
		"https://teams.live.com/meet/123":                         "", // consumer Teams
		"https://meet.google.com/abc-defg-hij":                    "",
		"https://attackerzoom.us/j/123":                           "", // not Zoom (meetingService)
		"https://zoom.us.evil.com/j/1":                            "",
		"zoommtg://zoom.us/join?confno=1":                         "", // never passed through from event data
		"https://example.com/teams.microsoft.com/l/meetup-join/x": "",
	} {
		if got := nativeMeetingURL(link); got != want {
			t.Errorf("nativeMeetingURL(%q) = %q, want %q", link, got, want)
		}
	}
	if s := schemeOf("msteams:/l/meetup-join/x"); s != "msteams" {
		t.Errorf("schemeOf: %q", s)
	}
}

func TestHasURLHandler(t *testing.T) {
	if hasURLHandler("do-not-forget-no-such-scheme://x") {
		t.Error("a scheme nothing registers has a handler")
	}
	// Desktop CI runners have a browser for https; a Linux runner may have no desktop at all.
	if runtime.GOOS != "linux" && !hasURLHandler("https://example.com") {
		t.Error("no handler for https")
	}
}
