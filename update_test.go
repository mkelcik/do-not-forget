package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestUpdaterPicksArchiveAlongsideDeb(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		assets := []github.ReleaseAsset{
			{Name: "do-not-forget_0.9.0_" + arch + ".deb"},
			{Name: "do-not-forget_0.9.0_" + arch + ".deb.manifest.sig"},
			{Name: "do-not-forget_linux_" + arch + ".tar.gz.sig"},
			{Name: "do-not-forget_linux_" + arch + ".tar.gz"},
		}
		if got := github.DefaultAssetMatcher(updater.CheckRequest{Platform: "linux", Arch: arch}, assets); got != len(assets)-1 {
			t.Errorf("%s: picked asset %d, want the tar.gz archive", arch, got)
		}
	}
}

func TestPackageAsset(t *testing.T) {
	for _, c := range []struct{ kind, version, goarch, want string }{
		{"deb", "0.10.0", "amd64", "do-not-forget_0.10.0_amd64.deb"},
		{"deb", "v0.10.0", "arm64", "do-not-forget_0.10.0_arm64.deb"},
		{"rpm", "0.10.0", "amd64", "do-not-forget_0.10.0_x86_64.rpm"},
		{"rpm", "0.10.0", "arm64", "do-not-forget_0.10.0_aarch64.rpm"},
		{"deb", "0.11.0-rc1", "amd64", "do-not-forget_0.11.0~rc1_amd64.deb"},
	} {
		if got := packageAsset(c.kind, c.version, c.goarch); got != c.want {
			t.Errorf("packageAsset(%s, %s, %s) = %s, want %s", c.kind, c.version, c.goarch, got, c.want)
		}
	}
}

func TestDownloadPackage(t *testing.T) {
	priv := testSigningKey(t)
	pkg := []byte("package")
	d := sha256.Sum256(pkg)
	asset := packageAsset("deb", "1.0.0", runtime.GOARCH)
	sign := func(version, asset string) []byte {
		return ed25519.Sign(priv, updateManifest(version, runtime.GOOS, runtime.GOARCH, asset, d[:]))
	}
	for name, c := range map[string]struct {
		sig []byte
		ok  bool
	}{
		"signed":           {sign("1.0.0", asset), true},
		"older version":    {sign("0.9.0", asset), false}, // an old package renamed as the new one
		"other asset":      {sign("1.0.0", "do-not-forget_1.0.0_other.deb"), false},
		"bad signature":    {make([]byte, ed25519.SignatureSize), false},
		"missing manifest": {nil, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/" + asset:
				_, _ = w.Write(pkg)
			case "/" + asset + ".manifest.sig":
				if c.sig == nil {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write(c.sig)
			default:
				http.NotFound(w, r)
			}
		}))
		path, err := downloadPackage(context.Background(), srv.Client(), srv.URL+"/", "deb", "1.0.0")
		srv.Close()
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok %v", name, err, c.ok)
		}
		if err == nil {
			if b, _ := os.ReadFile(path); !bytes.Equal(b, pkg) { //nolint:gosec // G304: test temp file
				t.Errorf("%s: downloaded %q", name, b)
			}
			_ = os.Remove(path)
		}
	}
}

func TestCheckResult(t *testing.T) {
	for _, c := range []struct {
		newer string
		err   error
		want  string
	}{
		{"", nil, "latest"},
		{"0.13.0", nil, "found"},
		{"", errors.New("offline"), "failed"},
		{"", fmt.Errorf("check: %w", errUpdating), "updating"},
	} {
		if got := checkResult(c.newer, c.err); got != c.want {
			t.Errorf("checkResult(%q, %v) = %q, want %q", c.newer, c.err, got, c.want)
		}
	}
}
