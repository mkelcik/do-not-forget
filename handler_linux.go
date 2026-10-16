package main

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// hasURLHandler reports whether an installed app opens this native meeting link: some
// .desktop file handles x-scheme-handler/<scheme>.
func hasURLHandler(native string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// The scheme is zoommtg or msteams: native links are only built by nativeMeetingURL.
	out, err := exec.CommandContext(ctx, "xdg-mime", "query", "default", "x-scheme-handler/"+schemeOf(native)).Output() //nolint:gosec // G204: fixed command, scheme from nativeMeetingURL
	return err == nil && strings.TrimSpace(string(out)) != ""
}
