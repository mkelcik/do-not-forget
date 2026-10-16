package main

import (
	"errors"
	"io"
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
	go func() { done <- m.disconnect(g, revokeAndDelete, func() {}) }()
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

	if err := m.disconnect(stale, revokeAndDelete, func() {}); err != nil {
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

// A revoke Google doesn't confirm isn't a successful disconnect: the token and the account stay,
// marked for a retry. Removing it from this computer only then works without Google.
func TestDisconnectWhenRevokeFails(t *testing.T) {
	m, cfg := testManager(t)
	revokes := 0
	status, body := http.StatusServiceUnavailable, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		revokes++
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	old := googleRevokeURL
	googleRevokeURL = srv.URL
	defer func() { googleRevokeURL = old }()

	_ = m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "a", RefreshToken: "r"}, nil)
	g := (*m.list)[0]
	var re *revokeError
	if err := m.disconnect(g, revokeAndDelete, func() {}); !errors.As(err, &re) {
		t.Fatalf("disconnect with Google down: %v, want a revokeError", err)
	}
	if _, err := loadToken("me@x.sk"); err != nil || len(*m.list) != 1 || len(cfg.Accounts) != 1 || !g.revokeFailed() {
		t.Fatalf("after a failed revoke: token err %v, %d sources, config %+v, revokeFailed %v", err, len(*m.list), cfg.Accounts, g.revokeFailed())
	}

	if err := m.disconnect(g, deleteOnly, func() {}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadToken("me@x.sk"); !errors.Is(err, keyring.ErrNotFound) || len(*m.list) != 0 || len(cfg.Accounts) != 0 {
		t.Errorf("after removing locally: token err %v, %d sources, config %+v", err, len(*m.list), cfg.Accounts)
	}
	if revokes != 1 {
		t.Errorf("%d revoke calls, want 1 (removing locally doesn't call Google)", revokes)
	}

	// A 400 for a bad request isn't proof the token is dead: keep it for a retry.
	status, body = http.StatusBadRequest, `{"error": "invalid_request"}`
	_ = m.connect("me@x.sk", builtinClient(), &oauth2.Token{AccessToken: "b", RefreshToken: "r"}, nil)
	if err := m.disconnect((*m.list)[0], revokeAndDelete, func() {}); !errors.As(err, &re) {
		t.Errorf("400 invalid_request: %v, want a revokeError", err)
	}
	if _, err := loadToken("me@x.sk"); err != nil {
		t.Errorf("token gone after a failed revoke: %v", err)
	}
	// "invalid_token": already revoked or expired at Google, as good as revoked.
	body = `{"error": "invalid_token", "error_description": "Token expired or revoked"}`
	if err := m.disconnect((*m.list)[0], revokeAndDelete, func() {}); err != nil {
		t.Errorf("revoke of an already invalid token: %v", err)
	}
	if _, err := loadToken("me@x.sk"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("token kept after a confirmed revoke: %v", err)
	}
}
