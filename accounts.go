package main

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"golang.org/x/oauth2"
)

var errBusy = errors.New("an account is being disconnected")

// accountManager connects and disconnects Google accounts. Both touch the keychain and the
// config by email, so they run one at a time from start to finish (revocation included):
// a disconnect finishing late can't delete the token or config entry of a newer connection.
type accountManager struct {
	ops sync.Mutex // held for a whole connect commit or disconnect

	mu         *sync.Mutex // main's lock: guards list and removing
	list       *[]*googleSource
	removing   map[string]bool // disconnects in progress, by email; Settings blocks connecting meanwhile
	saveConfig func(func(*Config))
}

// busy reports a disconnect in progress; connecting is refused until it's done.
func (m *accountManager) busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.removing) > 0
}

// connect stores a freshly signed-in account. Connecting one that's already there replaces its
// token and keeps its calendar choice; the old session stops saving tokens first.
func (m *accountManager) connect(email string, c oauthClient, tok *oauth2.Token, hidden []string) error {
	if m.busy() { // refuse at once rather than wait out a revoke
		return errBusy
	}
	m.ops.Lock()
	defer m.ops.Unlock()
	if m.busy() { // a disconnect that started meanwhile
		return errBusy
	}
	m.mu.Lock()
	for _, a := range *m.list {
		if a.email == email {
			_ = a.close(false)
		}
	}
	m.mu.Unlock()
	if err := saveToken(email, tok); err != nil {
		return fmt.Errorf("keychain: %w", err) // no keychain, no account: tokens never go to a file
	}
	clientID := c.ID
	if clientID == googleClientID {
		clientID = "" // the built-in one, whatever it is in a later build
	}
	m.saveConfig(func(cfg *Config) {
		for i, a := range cfg.Accounts {
			if a.Email == email {
				hidden = a.Hidden
				cfg.Accounts[i].ClientID = clientID
				return
			}
		}
		cfg.Accounts = append(cfg.Accounts, Account{Email: email, ClientID: clientID, Hidden: hidden})
	})
	g := newGoogleSource(email, c, tok, hidden)
	m.mu.Lock()
	defer m.mu.Unlock()
	if i := slices.IndexFunc(*m.list, func(a *googleSource) bool { return a.email == email }); i >= 0 {
		(*m.list)[i] = g
	} else {
		*m.list = append(*m.list, g)
	}
	return nil
}

// errClientGone marks accounts whose OAuth client is no longer set up: their tokens can't be refreshed.
var errClientGone = errors.New("its Google client ID was changed; connect the account again")

// dropClients ends the sessions of accounts whose client is no longer usable after the own
// client changed; they stay listed with errClientGone. Their tokens stay in the keychain until
// they're reconnected or disconnected.
func (m *accountManager) dropClients(usable func(clientID string) bool) {
	m.ops.Lock()
	defer m.ops.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, g := range *m.list {
		if g.client != nil && !usable(g.clientID) {
			_ = g.close(false)
			b := brokenGoogleSource(g.email, errClientGone)
			b.clientID = g.clientID
			(*m.list)[i] = b
		}
	}
}

// disconnect revokes g's token, deletes it from the keychain and drops the account. g is the
// session the user clicked; if a reconnect has replaced it since, there's nothing left to do.
// If the keychain refuses, the account stays listed with the error. marked runs once the
// account shows as disconnecting.
func (m *accountManager) disconnect(g *googleSource, marked func()) error {
	m.mu.Lock()
	m.removing[g.email] = true
	m.mu.Unlock()
	marked()
	defer func() {
		m.mu.Lock()
		delete(m.removing, g.email)
		m.mu.Unlock()
	}()

	m.ops.Lock()
	defer m.ops.Unlock()
	m.mu.Lock()
	current := slices.Contains(*m.list, g)
	m.mu.Unlock()
	if !current {
		return nil
	}
	if err := g.close(true); err != nil {
		return err
	}
	m.saveConfig(func(c *Config) {
		c.Accounts = slices.DeleteFunc(c.Accounts, func(a Account) bool { return a.Email == g.email })
	})
	m.mu.Lock()
	defer m.mu.Unlock()
	*m.list = slices.DeleteFunc(*m.list, func(a *googleSource) bool { return a == g })
	return nil
}
