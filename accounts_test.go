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

	if err := m.connect("me@x.sk", &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"}, nil); err != nil {
		t.Fatal(err)
	}
	g := (*m.list)[0]
	done := make(chan error)
	go func() { done <- m.disconnect(g, func() {}) }()
	<-revoking
	if err := m.connect("me@x.sk", &oauth2.Token{AccessToken: "a2", RefreshToken: "r2"}, nil); !errors.Is(err, errBusy) {
		t.Fatalf("reconnect during disconnect: %v, want errBusy", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(*m.list) != 0 || len(cfg.Accounts) != 0 {
		t.Fatalf("after disconnect: %d sources, config %+v", len(*m.list), cfg.Accounts)
	}

	if err := m.connect("me@x.sk", &oauth2.Token{AccessToken: "a2", RefreshToken: "r2"}, nil); err != nil {
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
	_ = m.connect("me@x.sk", &oauth2.Token{AccessToken: "a1", RefreshToken: "r1"}, []string{"holidays"})
	stale := (*m.list)[0]
	_ = m.connect("me@x.sk", &oauth2.Token{AccessToken: "a2", RefreshToken: "r2"}, nil)

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
