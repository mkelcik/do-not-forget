package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

func testManager(t *testing.T) (*accountManager, *Config) {
	t.Helper()
	keyring.MockInit()
	var mu sync.Mutex
	var list []*googleSource
	cfg := &Config{}
	m := &accountManager{mu: &mu, list: &list, removing: map[string]bool{},
		saveConfig: func(f func(*Config)) { mu.Lock(); defer mu.Unlock(); f(cfg) }}
	return m, cfg
}

// A disconnect still waiting on Google's revoke endpoint must not delete a reconnect's token
// or config entry: the reconnect is refused until the disconnect is done, then works.
func TestDisconnectThenReconnect(t *testing.T) {
	m, cfg := testManager(t)
	revoking, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(revoking)
		<-release
	}))
	defer srv.Close()
	old := googleRevokeURL
	googleRevokeURL = srv.URL
	defer func() { googleRevokeURL = old }()

	if err := m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"}, nil); err != nil {
		t.Fatal(err)
	}
	g := (*m.list)[0]
	done := make(chan error)
	go func() { done <- m.disconnect(g, func() {}) }()
	<-revoking
	if err := m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "a2", RefreshToken: "r2"}, nil); !errors.Is(err, errBusy) {
		t.Fatalf("reconnect during disconnect: %v, want errBusy", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(*m.list) != 0 || len(cfg.Accounts) != 0 {
		t.Fatalf("after disconnect: %d sources, config %+v", len(*m.list), cfg.Accounts)
	}

	if err := m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "a2", RefreshToken: "r2"}, nil); err != nil {
		t.Fatal(err)
	}
	if tok, err := loadToken("me@x.sk"); err != nil || tok.RefreshToken != "r2" {
		t.Errorf("reconnected token: %+v, %v", tok, err)
	}
	if len(*m.list) != 1 || len(cfg.Accounts) != 1 {
		t.Errorf("after reconnect: %d sources, config %+v", len(*m.list), cfg.Accounts)
	}
}

// A disconnect of a session a reconnect has already replaced leaves the new one alone.
func TestStaleDisconnectIsNoop(t *testing.T) {
	m, cfg := testManager(t)
	_ = m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"}, []string{"holidays"})
	stale := (*m.list)[0]
	_ = m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "a2", RefreshToken: "r2"}, nil)

	if err := m.disconnect(stale, func() {}); err != nil {
		t.Fatal(err)
	}
	if tok, err := loadToken("me@x.sk"); err != nil || tok.RefreshToken != "r2" {
		t.Errorf("new token: %+v, %v", tok, err)
	}
	if len(*m.list) != 1 || (*m.list)[0] == stale || len(cfg.Accounts) != 1 {
		t.Errorf("state: %d sources, config %+v", len(*m.list), cfg.Accounts)
	}
	if cfg.Accounts[0].Hidden[0] != "holidays" {
		t.Errorf("calendar choice lost on reconnect: %+v", cfg.Accounts[0])
	}
}

// Accounts remember their client; when it's no longer set up they stop and ask to reconnect.
func TestOwnClientAccounts(t *testing.T) {
	m, cfg := testManager(t)
	own := oauthClient{ID: "1-abc.apps.googleusercontent.com", Secret: "s"}
	_ = m.connect("work@x.sk", own, &oauth2.Token{AccessToken: "a", RefreshToken: "r"}, nil)
	_ = m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "b", RefreshToken: "r"}, nil)
	if cfg.Accounts[0].ClientID != own.ID || cfg.Accounts[1].ClientID != "" {
		t.Fatalf("client IDs in config: %+v", cfg.Accounts)
	}

	m.dropClients(func(id string) bool { return id == googleClientID }) // own client removed
	work, me := (*m.list)[0], (*m.list)[1]
	if _, _, errText := work.status(); work.client != nil || errText != errClientGone.Error() {
		t.Errorf("account on the removed client: client %v, error %q", work.client, errText)
	}
	if me.client == nil {
		t.Error("account on the built-in client stopped too")
	}
	if tok, err := loadToken("work@x.sk"); err != nil || tok.AccessToken != "a" {
		t.Errorf("token kept for reconnecting or disconnecting: %+v %v", tok, err)
	}
}

func TestValidClientID(t *testing.T) {
	for id, want := range map[string]bool{
		"123-abc.apps.googleusercontent.com": true,
		"123-abc":                            false,
		"a b.apps.googleusercontent.com":     false,
		"":                                   false,
	} {
		if validClientID(id) != want {
			t.Errorf("validClientID(%q) != %v", id, want)
		}
	}
}

// Reconnecting an account keeps the name the user gave it.
func TestReconnectKeepsLabel(t *testing.T) {
	m, cfg := testManager(t)
	_ = m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "a"}, nil)
	cfg.Accounts[0].Label = "Personal"
	cfg.Accounts[0].Names = map[string]string{"wife@x.sk": "Janka"}
	_ = m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "b"}, nil)
	g := (*m.list)[0]
	if label, _, _ := g.status(); label != "Personal" || g.calendarNames()["wife@x.sk"] != "Janka" {
		t.Errorf("after reconnect: label %q, names %v", label, g.calendarNames())
	}
}
