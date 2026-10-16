package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// Google OAuth client of the app, set at build time (-X, from .env or CI secrets).
// Empty in a build without them: Google then needs the user's own client from Settings.
var (
	googleClientID     string
	googleClientSecret string // Google requires it for desktop clients too, even with PKCE
)

// oauthClient is a Google OAuth client: the one built into the app, or the user's own from
// Settings (for forks, builds without ours, or policies that only allow their own). Tokens
// belong to the client that issued them, so every account remembers its client's ID.
type oauthClient struct{ ID, Secret string }

func builtinClient() oauthClient { return oauthClient{googleClientID, googleClientSecret} }

// validClientID is the shape of Google OAuth client IDs.
func validClientID(id string) bool {
	return strings.HasSuffix(id, ".apps.googleusercontent.com") && !strings.ContainsAny(id, " /|")
}

// The own client's secret is kept in the keychain like tokens (its ID is in the config).
// The entry name has no "@", so it can't clash with an account.
const clientSecretKey = "google-client-secret"

func saveClientSecret(s string) error   { return keyring.Set(keyringService, clientSecretKey, s) }
func loadClientSecret() (string, error) { return keyring.Get(keyringService, clientSecretKey) }
func deleteClientSecret() error {
	if err := keyring.Delete(keyringService, clientSecretKey); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// calendar.events: read events and RSVP. calendarlist.readonly: names and colours of calendars.
var googleScopes = []string{
	"https://www.googleapis.com/auth/calendar.events",
	"https://www.googleapis.com/auth/calendar.calendarlist.readonly",
}

const (
	keyringService = "do-not-forget"
	googleTimeout  = 30 * time.Second
)

// Tests point these at a fake.
var (
	googleAPI       = "https://www.googleapis.com/calendar/v3"
	googleRevokeURL = "https://oauth2.googleapis.com/revoke"
)

var errDisconnected = errors.New("account disconnected")

func (c oauthClient) config(redirect string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ID,
		ClientSecret: c.Secret,
		// Not oauth2/google: it pulls in the GCE metadata client.
		Endpoint: oauth2.Endpoint{ //nolint:gosec // G101: public endpoint URLs, not credentials
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
		RedirectURL: redirect,
		Scopes:      googleScopes,
	}
}

// oauthCtx carries the HTTP client x/oauth2 uses for token exchange and refresh.
func oauthCtx() context.Context {
	return context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: googleTimeout})
}

// googleLogin runs the installed-app flow: the browser signs in and redirects to a one-shot
// server on 127.0.0.1, PKCE protects the code. Gives up after 5 minutes.
func googleLogin(ctx context.Context, c oauthClient, openURL func(string) error) (*oauth2.Token, error) {
	if c.ID == "" {
		return nil, errors.New("no Google client ID")
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer func() { _ = ln.Close() }()
	conf := c.config(fmt.Sprintf("http://127.0.0.1:%d/", ln.Addr().(*net.TCPAddr).Port))
	verifier, state := oauth2.GenerateVerifier(), oauth2.GenerateVerifier()

	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state { // favicon requests and strays
			http.NotFound(w, r)
			return
		}
		res := result{code: q.Get("code")}
		msg := T("google.signed_in")
		if res.code == "" {
			res.err = fmt.Errorf("sign-in failed: %s", q.Get("error"))
			msg = T("google.failed")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>do-not-forget</title>`+
			`<p style="font:16px system-ui;text-align:center;margin-top:20vh">%s</p>`, html.EscapeString(msg))
		select {
		case done <- res:
		default:
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	if err := openURL(conf.AuthCodeURL(state,
		oauth2.AccessTypeOffline,                    // a refresh token
		oauth2.SetAuthURLParam("prompt", "consent"), // ... also when the account was connected before
		oauth2.S256ChallengeOption(verifier))); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	select {
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}
		return conf.Exchange(oauthCtx(), res.code, oauth2.VerifierOption(verifier))
	case <-ctx.Done():
		return nil, errors.New("sign-in timed out")
	}
}

// Tokens live in the system keychain only (Keychain, Secret Service, Credential Manager),
// keyed by the account's email; never in the config file.
func saveToken(email string, t *oauth2.Token) error {
	b, err := json.Marshal(t) //nolint:gosec // G117: serialised for the keychain, which is the point
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, email, string(b))
}

func loadToken(email string) (*oauth2.Token, error) {
	s, err := keyring.Get(keyringService, email)
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	return &t, json.Unmarshal([]byte(s), &t)
}

// deleteToken removes the account's token; one that isn't there counts as removed.
func deleteToken(email string) error {
	if err := keyring.Delete(keyringService, email); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// revokeToken withdraws the app's access at Google; revoking the refresh token ends the whole grant.
func revokeToken(t *oauth2.Token) error {
	tok := t.RefreshToken
	if tok == "" {
		tok = t.AccessToken
	}
	ctx, cancel := context.WithTimeout(context.Background(), googleTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleRevokeURL, strings.NewReader(url.Values{"token": {tok}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest { // 400: already invalid
		return fmt.Errorf("revoke: %s", resp.Status)
	}
	return nil
}

// forgetToken revokes the token at Google (best effort, logged) and deletes it from the keychain.
// tok may be nil, then the keychain's copy is revoked.
func forgetToken(email string, tok *oauth2.Token) error {
	if tok == nil {
		tok, _ = loadToken(email)
	}
	if tok != nil {
		if err := revokeToken(tok); err != nil {
			log.Printf("%s: %v", email, err) // still delete locally; access can be removed in the Google account
		}
	}
	return deleteToken(email)
}

// savingTokenSource writes refreshed tokens back to the keychain, until it's closed: a refresh
// still in flight when the account is disconnected must not store the token again.
type savingTokenSource struct {
	email string
	base  oauth2.TokenSource

	mu     sync.Mutex // also serialises saving against close
	tok    *oauth2.Token
	closed bool
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, errDisconnected
	}
	t, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errDisconnected
	}
	if t.AccessToken != s.tok.AccessToken {
		s.tok = t
		_ = saveToken(s.email, t) // still usable this session if the keychain balks
	}
	return t, nil
}

// close stops all further saving; forget also revokes the token and deletes it from the keychain.
func (s *savingTokenSource) close(forget bool) error {
	s.mu.Lock()
	s.closed = true
	tok := s.tok
	s.mu.Unlock()
	if !forget {
		return nil
	}
	return forgetToken(s.email, tok)
}

// googleSource is one Google account. Events come from every calendar the user hasn't hidden.
type googleSource struct {
	email    string
	clientID string             // the OAuth client the token belongs to
	client   *http.Client       // nil when the token couldn't be loaded
	ts       *savingTokenSource // nil with client
	life     context.Context    // cancelled by close, which aborts requests in flight
	stop     context.CancelFunc

	mu        sync.Mutex
	hidden    map[string]bool // calendar IDs unticked in Settings
	calendars []Calendar      // from the last fetch, for Settings
	err       error           // last fetch error, for Settings
	closed    bool
}

// Calendar is one of an account's calendars as Settings shows it.
type Calendar struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	On    bool   `json:"on"`
}

func newGoogleSource(email string, c oauthClient, tok *oauth2.Token, hidden []string) *googleSource {
	g := brokenGoogleSource(email, nil)
	g.clientID = c.ID
	g.ts = &savingTokenSource{email: email, base: c.config("").TokenSource(oauthCtx(), tok), tok: tok}
	g.client = oauth2.NewClient(oauthCtx(), g.ts)
	g.client.Timeout = googleTimeout
	for _, id := range hidden {
		g.hidden[id] = true
	}
	return g
}

// brokenGoogleSource is an account whose token couldn't be loaded: listed with err, fetches nothing.
func brokenGoogleSource(email string, err error) *googleSource {
	g := &googleSource{email: email, hidden: map[string]bool{}, err: err}
	g.life, g.stop = context.WithCancel(context.Background())
	return g
}

// close ends the account's session: requests in flight are cancelled and tokens are no longer
// saved. forget also revokes the token and removes it from the keychain; an error means the
// keychain still holds it, and close can be retried.
func (g *googleSource) close(forget bool) error {
	g.stop()
	g.mu.Lock()
	g.closed = true
	g.err = errDisconnected
	g.mu.Unlock()
	var err error
	if g.ts != nil {
		err = g.ts.close(forget)
	} else if forget {
		err = forgetToken(g.email, nil)
	}
	if err != nil {
		g.setErr(fmt.Errorf("credentials not removed from the keychain: %w", err))
	}
	return err
}

func (g *googleSource) setErr(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.err = err
}

// bound limits ctx to the request timeout and to the account's life.
func (g *googleSource) bound(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, googleTimeout)
	stop := context.AfterFunc(g.life, cancel)
	return ctx, func() { stop(); cancel() }
}

func (g *googleSource) setHidden(id string, hidden bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.hidden[id] = hidden
}

// status is the account as Settings shows it.
func (g *googleSource) status() (cals []Calendar, errText string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.err != nil {
		errText = g.err.Error()
	}
	return append([]Calendar{}, g.calendars...), errText
}

// do calls the Calendar API and decodes the JSON answer into out.
func (g *googleSource) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	u := googleAPI + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("%s %s: %s %s", method, path, resp.Status, e.Error.Message)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type gCalendar struct {
	ID              string `json:"id"`
	Summary         string `json:"summary"`
	SummaryOverride string `json:"summaryOverride"`
	BackgroundColor string `json:"backgroundColor"`
	Primary         bool   `json:"primary"`
	Selected        bool   `json:"selected"`
}

func (g *googleSource) calendarList(ctx context.Context) ([]gCalendar, error) {
	var all []gCalendar
	q := url.Values{}
	for {
		var page struct {
			Items         []gCalendar `json:"items"`
			NextPageToken string      `json:"nextPageToken"`
		}
		if err := g.do(ctx, http.MethodGet, "/users/me/calendarList", q, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if page.NextPageToken == "" {
			return all, nil
		}
		q.Set("pageToken", page.NextPageToken)
	}
}

// accountEmail is the primary calendar's ID, which is the account's address;
// it spares an extra openid/email scope.
func accountEmail(ctx context.Context, c oauthClient, tok *oauth2.Token) (email string, hidden []string, err error) {
	// A plain client: nothing to save yet, the account has no keychain entry.
	g := brokenGoogleSource("", nil)
	defer g.stop()
	g.client = c.config("").Client(oauthCtx(), tok)
	g.client.Timeout = googleTimeout
	ctx, cancel := context.WithTimeout(ctx, googleTimeout)
	defer cancel()
	cals, err := g.calendarList(ctx)
	if err != nil {
		return "", nil, err
	}
	for _, c := range cals {
		if c.Primary {
			email = c.ID
		}
		if !c.Selected { // start with what the Google Calendar web UI shows
			hidden = append(hidden, c.ID)
		}
	}
	if email == "" {
		return "", nil, errors.New("no primary calendar")
	}
	return email, hidden, nil
}

func (g *googleSource) Events(ctx context.Context, from, to time.Time) ([]Event, error) {
	g.mu.Lock()
	if g.closed || g.client == nil { // disconnected, or no token in the keychain
		err := g.err
		g.mu.Unlock()
		return nil, err
	}
	g.mu.Unlock()
	ctx, cancel := g.bound(ctx)
	defer cancel()
	evs, err := g.events(ctx, from, to)
	g.mu.Lock()
	if !g.closed { // keep close's message
		g.err = err
	}
	g.mu.Unlock()
	return evs, err
}

func (g *googleSource) events(ctx context.Context, from, to time.Time) ([]Event, error) {
	gcals, err := g.calendarList(ctx)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	cals := make([]Calendar, 0, len(gcals))
	for _, c := range gcals {
		name := c.SummaryOverride
		if name == "" {
			name = c.Summary
		}
		cals = append(cals, Calendar{ID: c.ID, Name: name, Color: c.BackgroundColor, On: !g.hidden[c.ID]})
	}
	g.calendars = cals
	g.mu.Unlock()

	var out []Event
	for _, c := range cals {
		if !c.On {
			continue
		}
		q := url.Values{
			"timeMin":      {from.Format(time.RFC3339)},
			"timeMax":      {to.Format(time.RFC3339)},
			"singleEvents": {"true"}, // recurring events as instances
			"orderBy":      {"startTime"},
			"maxResults":   {"250"},
		}
		for {
			var page struct {
				Items         []gEvent `json:"items"`
				NextPageToken string   `json:"nextPageToken"`
			}
			if err := g.do(ctx, http.MethodGet, "/calendars/"+url.PathEscape(c.ID)+"/events", q, nil, &page); err != nil {
				return nil, fmt.Errorf("%s: %w", c.Name, err)
			}
			for _, ge := range page.Items {
				if e, ok := ge.event(g.email, c); ok {
					out = append(out, e)
				}
			}
			if page.NextPageToken == "" {
				break
			}
			q.Set("pageToken", page.NextPageToken)
		}
	}
	return out, nil
}

type gEvent struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	EventType   string `json:"eventType"`
	Summary     string `json:"summary"`
	Location    string `json:"location"`
	Description string `json:"description"`
	HTMLLink    string `json:"htmlLink"`
	HangoutLink string `json:"hangoutLink"`
	Start       gTime  `json:"start"`
	End         gTime  `json:"end"`
	Organizer   struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Self        bool   `json:"self"`
	} `json:"organizer"`
	Attendees []struct {
		Email          string `json:"email"`
		DisplayName    string `json:"displayName"`
		ResponseStatus string `json:"responseStatus"`
		Self           bool   `json:"self"`
		Resource       bool   `json:"resource"` // rooms and equipment
	} `json:"attendees"`
	ConferenceData struct {
		EntryPoints []struct {
			EntryPointType string `json:"entryPointType"`
			URI            string `json:"uri"`
		} `json:"entryPoints"`
	} `json:"conferenceData"`
}

// gTime is either a date (all-day events, end exclusive) or a date-time.
type gTime struct {
	Date     string `json:"date"`
	DateTime string `json:"dateTime"`
}

func (t gTime) parse() (time.Time, bool, error) {
	if t.Date != "" {
		d, err := time.ParseInLocation(time.DateOnly, t.Date, time.Local)
		return d, true, err
	}
	d, err := time.Parse(time.RFC3339, t.DateTime)
	return d.Local(), false, err
}

var googleResponse = map[string]string{"needsAction": "pending", "accepted": "yes", "tentative": "maybe", "declined": "no"}

// event maps a Google event; ok is false for ones the agenda doesn't show.
func (ge gEvent) event(account string, cal Calendar) (Event, bool) {
	if ge.Status == "cancelled" || ge.EventType == "workingLocation" {
		return Event{}, false
	}
	start, allDay, err1 := ge.Start.parse()
	end, _, err2 := ge.End.parse()
	if err1 != nil || err2 != nil {
		return Event{}, false
	}
	e := Event{
		ID:          googleEventID(account, cal.ID, ge.ID),
		Title:       ge.Summary,
		Calendar:    cal.Name,
		Color:       cal.Color,
		Start:       start,
		End:         end,
		AllDay:      allDay,
		Location:    ge.Location,
		Description: htmlToText(ge.Description),
		Organizer:   ge.Organizer.DisplayName,
		CalendarURL: ge.HTMLLink,
	}
	if e.Title == "" {
		e.Title = T("event.no_title")
	}
	if e.Organizer == "" {
		e.Organizer = ge.Organizer.Email
	}
	for _, a := range ge.Attendees {
		if a.Resource {
			continue
		}
		name := a.DisplayName
		if name == "" {
			name = a.Email
		}
		status := googleResponse[a.ResponseStatus]
		e.Attendees = append(e.Attendees, Attendee{Name: name, Status: status, Self: a.Self})
		if a.Self && !ge.Organizer.Self { // my own meetings have nothing to answer
			e.Response = status
		}
	}
	for _, ep := range ge.ConferenceData.EntryPoints {
		if ep.EntryPointType == "video" && e.Link == "" && externalURL(ep.URI) {
			e.Link = ep.URI
		}
	}
	if e.Link == "" {
		e.Link = ge.HangoutLink
	}
	if e.Link == "" {
		e.Link = meetingLink(ge.Location + "\n" + e.Description)
	}
	return e, true
}

// IDs carry the account and calendar: the same invitation in two accounts has the same event ID,
// and RSVP needs the calendar. Neither Google IDs nor emails contain "|".
func googleEventID(account, calendarID, eventID string) string {
	return account + "|" + calendarID + "|" + eventID
}

func splitGoogleEventID(id string) (account, calendarID, eventID string, ok bool) {
	p := strings.Split(id, "|")
	if len(p) != 3 {
		return "", "", "", false
	}
	return p[0], p[1], p[2], true
}

var (
	httpsURLRe = regexp.MustCompile(`https://[^\s<>"]+`)
	meetCodeRe = regexp.MustCompile(`^/[a-z]{3}-[a-z]{4}-[a-z]{3}$`)
)

// meetingLink finds a Meet, Zoom or Teams URL in free text such as the location or description.
func meetingLink(s string) string {
	for _, u := range httpsURLRe.FindAllString(s, -1) {
		if u = strings.TrimRight(u, ".,;)"); meetingService(u) != "" {
			return u
		}
	}
	return ""
}

// meetingService names the service a meeting URL belongs to, judged by its exact host (so
// "attackerzoom.us" isn't Zoom); "" for anything else.
func meetingService(link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	hasPrefix := func(prefixes ...string) bool {
		return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(u.Path, p) })
	}
	switch {
	case host == "meet.google.com" && meetCodeRe.MatchString(u.Path):
		return "Meet"
	case (host == "zoom.us" || strings.HasSuffix(host, ".zoom.us")) && hasPrefix("/j/", "/my/", "/w/"):
		return "Zoom"
	case (host == "teams.microsoft.com" || host == "teams.live.com") && hasPrefix("/l/meetup-join/", "/meet/"):
		return "Teams"
	}
	return ""
}

// externalURL is what the app hands to the OS to open: http(s) links only, never file:,
// custom schemes or javascript: from event data.
func externalURL(link string) bool {
	u, err := url.Parse(link)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

var (
	htmlBreakRe  = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>|</li>`)
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	blankLinesRe = regexp.MustCompile(`\n[ \t\x{00a0}]*(?:\n[ \t\x{00a0}]*)+\n`) // 3+ line breaks, blanks between
)

// htmlToText flattens the HTML Google stores in descriptions edited in its web UI.
func htmlToText(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	s = htmlBreakRe.ReplaceAllString(s, "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = blankLinesRe.ReplaceAllString(html.UnescapeString(s), "\n\n")
	return strings.TrimSpace(s)
}

// Respond sets my RSVP on this occurrence. attendees is replaced as a whole by a patch, so it's
// read first and only my entry changes. sendUpdates=none: the organizer sees the answer in their
// calendar, without an email.
func (g *googleSource) Respond(ctx context.Context, id, response string) error {
	g.mu.Lock()
	if g.closed || g.client == nil {
		err := g.err
		g.mu.Unlock()
		return err
	}
	g.mu.Unlock()
	_, calID, eventID, ok := splitGoogleEventID(id)
	if !ok {
		return fmt.Errorf("not a Google event: %s", id)
	}
	status := ""
	for k, v := range googleResponse {
		if v == response {
			status = k
		}
	}
	if status == "" || status == "needsAction" {
		return fmt.Errorf("unknown response %q", response)
	}
	ctx, cancel := g.bound(ctx)
	defer cancel()
	path := "/calendars/" + url.PathEscape(calID) + "/events/" + url.PathEscape(eventID)
	var ev struct {
		Attendees []map[string]any `json:"attendees"` // kept whole: every field must survive
	}
	if err := g.do(ctx, http.MethodGet, path, nil, nil, &ev); err != nil {
		return err
	}
	found := false
	for _, a := range ev.Attendees {
		if self, _ := a["self"].(bool); self {
			a["responseStatus"] = status
			found = true
		}
	}
	if !found {
		return errors.New("not invited to this event")
	}
	return g.do(ctx, http.MethodPatch, path, url.Values{"sendUpdates": {"none"}}, map[string]any{"attendees": ev.Attendees}, nil)
}
