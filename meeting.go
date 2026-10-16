package main

import (
	"net/url"
	"regexp"
	"strings"
)

var zoomMeetingPath = regexp.MustCompile(`^/[jw]/(\d+)$`)

// nativeMeetingURL turns a Zoom or Teams web link into the desktop app's own link
// (zoommtg:, msteams:), or "" when there's no such form, e.g. Meet or a Zoom personal link.
// It's built only from links meetingService accepted, never taken from event data as is.
func nativeMeetingURL(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	switch meetingService(link) {
	case "Zoom":
		m := zoomMeetingPath.FindStringSubmatch(u.Path)
		if m == nil {
			return ""
		}
		q := url.Values{"action": {"join"}, "confno": {m[1]}}
		if pwd := u.Query().Get("pwd"); pwd != "" {
			q.Set("pwd", pwd)
		}
		// The host keeps a company's own Zoom domain (acme.zoom.us), which its SSO needs.
		return (&url.URL{Scheme: "zoommtg", Host: u.Host, Path: "/join", RawQuery: q.Encode()}).String()
	case "Teams":
		if !strings.EqualFold(u.Hostname(), "teams.microsoft.com") { // teams.live.com: no msteams: form
			return ""
		}
		native := "msteams:" + u.EscapedPath()
		if u.RawQuery != "" {
			native += "?" + u.RawQuery
		}
		return native
	}
	return ""
}

// schemeOf is the scheme of a native meeting link, for looking up its handler.
func schemeOf(native string) string {
	scheme, _, _ := strings.Cut(native, ":")
	return strings.ToLower(scheme)
}
