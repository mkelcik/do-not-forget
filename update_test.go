package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

func TestUpdaterPicksArchiveAlongsideDeb(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		assets := []github.ReleaseAsset{
			{Name: "do-not-forget_0.9.0_" + arch + ".deb"},
			{Name: "do-not-forget_linux_" + arch + ".tar.gz.sig"},
			{Name: "do-not-forget_linux_" + arch + ".tar.gz"},
		}
		if got := github.DefaultAssetMatcher(updater.CheckRequest{Platform: "linux", Arch: arch}, assets); got != 2 {
			t.Errorf("%s: picked asset %d, want the tar.gz archive", arch, got)
		}
	}
}
