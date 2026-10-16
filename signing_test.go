package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// fakeProvider stands in for Wails' GitHub provider: one release with a SHA256SUMS digest.
type fakeProvider struct{ url, version, asset string }

func (fakeProvider) Name() string { return "fake" }
func (f fakeProvider) Check(context.Context, updater.CheckRequest) (*updater.Release, error) {
	d := sha256.Sum256([]byte("archive"))
	return &updater.Release{
		Version:      f.version,
		Artifact:     updater.Artifact{Filename: f.asset},
		Metadata:     map[string]any{"github.asset.url": f.url},
		Verification: &updater.Verification{DigestAlgo: "sha256", Digest: d[:]},
	}, nil
}
func (fakeProvider) Download(context.Context, *updater.Release, io.Writer, func(int64, int64)) error {
	return nil
}

// testSigningKey swaps updatePublicKey for a fresh key for the test's duration.
func testSigningKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(nil)
	der, _ := x509.MarshalPKIXPublicKey(pub)
	old := updatePublicKey
	updatePublicKey = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	t.Cleanup(func() { updatePublicKey = old })
	return priv
}

func TestSignedProvider(t *testing.T) {
	priv := testSigningKey(t)
	d := sha256.Sum256([]byte("archive"))
	asset := fmt.Sprintf("do-not-forget_%s_%s.zip", runtime.GOOS, runtime.GOARCH)
	// What release.yml publishes for version 1.0.0 of this platform's build.
	files := map[string][]byte{
		"/" + asset + ".sig":          ed25519.Sign(priv, d[:]),
		"/" + asset + ".manifest.sig": ed25519.Sign(priv, updateManifest("1.0.0", runtime.GOOS, runtime.GOARCH, asset, d[:])),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if b, ok := files[r.URL.Path]; ok {
			_, _ = w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	check := func(version, name, path string) (*updater.Release, error) {
		p := signedProvider{Provider: fakeProvider{url: srv.URL + path, version: version, asset: name}, client: srv.Client()}
		return p.Check(context.Background(), updater.CheckRequest{})
	}

	rel, err := check("1.0.0", asset, "/"+asset)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Verification.SignatureAlgo != "ed25519" || !ed25519.Verify(priv.Public().(ed25519.PublicKey), d[:], rel.Verification.Signature) {
		t.Fatalf("signature not attached: %+v", rel.Verification)
	}

	// The same signed archive offered as a newer version: a downgrade in disguise.
	if _, err := check("9.0.0", asset, "/"+asset); err == nil {
		t.Error("an old archive relabelled 9.0.0 was accepted")
	}
	// Another platform's build, or no signatures at all.
	if _, err := check("1.0.0", "do-not-forget_plan9_mips.zip", "/"+asset); err == nil {
		t.Error("another platform's asset was accepted")
	}
	if _, err := check("1.0.0", asset, "/unsigned.zip"); err == nil {
		t.Error("unsigned release accepted")
	}
}

// The manifest's exact bytes are what release.yml signs with printf; keep them in step.
func TestUpdateManifestFormat(t *testing.T) {
	got := string(updateManifest("0.8.2", "darwin", "arm64", "do-not-forget_darwin_arm64.zip", []byte{0xab, 0xcd}))
	want := "do-not-forget update\nversion: 0.8.2\nos: darwin\narch: arm64\nasset: do-not-forget_darwin_arm64.zip\nsha256: abcd\n"
	if got != want {
		t.Errorf("manifest %q, want %q", got, want)
	}
}

// A server that accepts the request and never answers must not hang the update loop.
func TestSignedProviderTimeout(t *testing.T) {
	stall := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-stall }))
	defer srv.Close()
	defer close(stall)

	p := signedProvider{Provider: fakeProvider{url: srv.URL + "/a.tar.gz", version: "1.0.0", asset: "a.tar.gz"},
		client: &http.Client{Timeout: 100 * time.Millisecond}}
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
