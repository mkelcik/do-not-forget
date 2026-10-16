package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// updatePublicKey verifies release signatures. The private half lives only in the
// UPDATE_SIGNING_KEY repository secret; CI signs every release archive with it.
var updatePublicKey = []byte(`-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEA+C4HqLvqFVvnGj1O5pxZOWlI3o45AjSImkoaePLKRao=
-----END PUBLIC KEY-----
`)

// signedProvider adds signatures to a provider that only knows SHA256SUMS (Wails' GitHub
// provider). For each release asset CI publishes "<asset>.sig": a raw Ed25519 signature
// over the asset's SHA-256 digest. The updater then checks it against updatePublicKey,
// and a missing or bad signature fails the update.
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
	sig, err := p.fetchSignature(ctx, url+".sig")
	if err != nil {
		return nil, fmt.Errorf("release signature: %w", err)
	}
	rel.Verification.SignatureAlgo = "ed25519"
	rel.Verification.Signature = sig
	return rel, nil
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
