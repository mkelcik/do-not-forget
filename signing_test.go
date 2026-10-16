package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// fakeProvider stands in for Wails' GitHub provider: one release with a SHA256SUMS digest.
type fakeProvider struct{ url string }

func (fakeProvider) Name() string { return "fake" }
func (f fakeProvider) Check(context.Context, updater.CheckRequest) (*updater.Release, error) {
	d := sha256.Sum256([]byte("archive"))
	return &updater.Release{
		Version:      "1.0.0",
		Metadata:     map[string]any{"github.asset.url": f.url},
		Verification: &updater.Verification{DigestAlgo: "sha256", Digest: d[:]},
	}, nil
}
func (fakeProvider) Download(context.Context, *updater.Release, io.Writer, func(int64, int64)) error {
	return nil
}

func TestSignedProvider(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	d := sha256.Sum256([]byte("archive"))
	sig := ed25519.Sign(priv, d[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/signed.tar.gz.sig" {
			_, _ = w.Write(sig)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	p := signedProvider{Provider: fakeProvider{url: srv.URL + "/signed.tar.gz"}, client: srv.Client()}
	rel, err := p.Check(context.Background(), updater.CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if rel.Verification.SignatureAlgo != "ed25519" || !ed25519.Verify(priv.Public().(ed25519.PublicKey), d[:], rel.Verification.Signature) {
		t.Fatalf("signature not attached: %+v", rel.Verification)
	}

	// No .sig published: refuse rather than fall back to checksum-only.
	p.Provider = fakeProvider{url: srv.URL + "/unsigned.tar.gz"}
	if _, err := p.Check(context.Background(), updater.CheckRequest{}); err == nil {
		t.Fatal("unsigned release accepted")
	}
}

// A server that accepts the request and never answers must not hang the update loop.
func TestSignedProviderTimeout(t *testing.T) {
	stall := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-stall }))
	defer srv.Close()
	defer close(stall)

	p := signedProvider{Provider: fakeProvider{url: srv.URL + "/a.tar.gz"}, client: &http.Client{Timeout: 100 * time.Millisecond}}
	done := make(chan error, 1)
	go func() { _, err := p.Check(context.Background(), updater.CheckRequest{}); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stalled signature accepted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Check hung on a stalled server")
	}
}
