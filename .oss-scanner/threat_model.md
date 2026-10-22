# Threat model

## What this project does and where untrusted input enters

Do Not Forget is a desktop tray app (Linux, macOS, Windows): Go backend, plain HTML/JS frontend in a
Wails v3 webview. It reads the user's Google Calendar, shows upcoming events, reminds before they start
and opens meeting links. AGENTS.md describes the layout and the security invariants.

Untrusted input:

- **Calendar event data** from the Google Calendar API (`google.go`): titles, descriptions, locations,
  attendees, conference data and links. Anyone can send the user an invitation, so treat every field as
  attacker-controlled. It reaches the webview (`frontend/index.html`, `frontend/alert.html`) and, as
  links, the OS (`openURL`, `nativeMeetingURL`).
- **The OAuth loopback callback** on `127.0.0.1` (`google.go`), reachable by other local processes and by
  web pages through the browser.
- **GitHub Releases** for the in-app updater (`update.go`, `latest.go`, `signing.go`): release metadata and
  downloaded archives, verified against `SHA256SUMS` and Ed25519 signatures.

Trusted: the user, `config.json` in the user's config dir, translations in `frontend/i18n/`, environment
variables and command-line flags.

## Components that matter most / least

Most: event data reaching HTML or a URL handler, link handling (`source.go` sanitising, `meetingService`
exact-host matching, http(s)-only `openURL`), the OAuth flow and token storage (tokens only in the system
keychain, never on disk or in logs), disconnect / revoke (`accounts.go`), update verification.

Least / out of scope: `dummy.go`, `dummy.json`, `showcase.json` (development data), `scripts/`,
`packaging/`, `docs/` (static website), `.github/`.

## How to exercise it

`go test ./...` runs everything; tests use `httptest` servers for Google and GitHub and `keyring.MockInit`
for the keychain. The UI needs a display, so drive the frontend logic by reading the HTML/JS and the Go
side through the tests. The built binary is `/src/do-not-forget`.

## How we rate severity

- Critical: code execution, installing an update that fails signature verification, leaking OAuth tokens
  or the client secret, script execution in the webview from event data.
- High: opening a non-http(s) URL or a URL handler from event data, a token saved after disconnect,
  forging the OAuth callback to connect an attacker's account, HTML injection from event data.
- Medium: local-only issues that need another process already running as the user.
- Low: crashes and denial of service from malformed event data.
