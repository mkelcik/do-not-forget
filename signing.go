package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// updatePublicKey verifies release signatures. The private half lives only in the
// UPDATE_SIGNING_KEY repository secret; CI signs every release archive with it.
var updatePublicKey = []byte(`-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEA+C4HqLvqFVvnGj1O5pxZOWlI3o45AjSImkoaePLKRao=
-----END PUBLIC KEY-----
`)

// signedProvider adds signatures to a provider that only knows SHA256SUMS (Wails' GitHub
// provider). For each release asset CI publishes two raw Ed25519 signatures:
//
//   - "<asset>.manifest.sig" over updateManifest: the version, OS, architecture, asset name and
//     SHA-256 together. Checked here, so an old signed archive can't be offered as a newer
//     version (a downgrade to fixed bugs) or as another platform's build.
//   - "<asset>.sig" over the SHA-256 alone, which Wails checks again while installing (and
//     which 0.8.0 and 0.8.1, from before manifests, rely on).
//
// A missing or bad signature fails the update.
type signedProvider struct {
	updater.Provider
	client *http.Client
}

func (p signedProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	rel, err := p.Provider.Check(ctx, req)
	if err != nil || rel == nil {
		return rel, err
	}
	url, _ := rel.Metadata["github.asset.url"].(string)
	if url == "" || rel.Verification == nil || len(rel.Verification.Digest) == 0 {
		return nil, errors.New("release has no asset URL or checksum, refusing unsigned update")
	}
	manifestSig, err := p.fetchSignature(ctx, url+".manifest.sig")
	if err != nil {
		return nil, fmt.Errorf("release manifest signature: %w", err)
	}
	if err := verifyManifest(rel, runtime.GOOS, runtime.GOARCH, manifestSig); err != nil {
		return nil, err
	}
	sig, err := p.fetchSignature(ctx, url+".sig")
	if err != nil {
		return nil, fmt.Errorf("release signature: %w", err)
	}
	rel.Verification.SignatureAlgo = "ed25519"
	rel.Verification.Signature = sig
	return rel, nil
}

// updateManifest is what CI signs for each release asset (see release.yml); the updater
// rebuilds it from the offered release and its own platform, so every field is vouched for.
func updateManifest(version, goos, goarch, asset string, sha256 []byte) []byte {
	return fmt.Appendf(nil, "do-not-forget update\nversion: %s\nos: %s\narch: %s\nasset: %s\nsha256: %x\n",
		version, goos, goarch, asset, sha256)
}

// verifyManifest checks the release's manifest signature, and that the asset is the build for
// this OS and architecture.
func verifyManifest(rel *updater.Release, goos, goarch string, sig []byte) error {
	asset := rel.Artifact.Filename
	if !strings.HasPrefix(asset, fmt.Sprintf("do-not-forget_%s_%s.", goos, goarch)) {
		return fmt.Errorf("release asset %q isn't the %s/%s build", asset, goos, goarch)
	}
	key, err := publicKey()
	if err != nil {
		return err
	}
	manifest := updateManifest(strings.TrimPrefix(rel.Version, "v"), goos, goarch, asset, rel.Verification.Digest)
	if !ed25519.Verify(key, manifest, sig) {
		return fmt.Errorf("release manifest signature doesn't match version %s of %s", rel.Version, asset)
	}
	return nil
}

func publicKey() (ed25519.PublicKey, error) {
	block, _ := pem.Decode(updatePublicKey)
	if block == nil {
		return nil, errors.New("update public key: not PEM")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("update public key: %w", err)
	}
	ek, ok := k.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("update public key: not Ed25519")
	}
	return ek, nil
}

func (p signedProvider) fetchSignature(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	sig, err := io.ReadAll(io.LimitReader(resp.Body, ed25519.SignatureSize+1))
	if err != nil {
		return nil, err
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("signature is %d bytes, want %d", len(sig), ed25519.SignatureSize)
	}
	return sig, nil
}
