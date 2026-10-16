# AGENTS.md

Guidance for coding agents (and humans) working in this repository.

## What this is

Do Not Forget (`do-not-forget` in code, paths and the repository) is a tray / menu bar app for Linux, macOS and Windows that shows the next
meeting, reminds you before it starts and lets you join or RSVP in one click. Go backend,
plain HTML/JS frontend, [Wails v3](https://v3alpha.wails.io/) in between. One `main` package,
no frontend build step.

## Layout

| Path | What |
| --- | --- |
| `main.go` | App wiring: windows, tray, events between Go and the frontend, reminders, alert |
| `source.go` | `Event`, `Source` interface, `fetchAll` (merges sources, sanitises links) |
| `google.go` | Google Calendar `Source`: OAuth (loopback + PKCE), keychain tokens, REST calls, event mapping, link detection |
| `accounts.go` | Connecting / disconnecting accounts, serialised end to end |
| `dummy.go`, `dummy.json` | Development source with fake events (`make run`) |
| `config.go` | `config.json` in the user config dir (no secrets) |
| `i18n.go`, `frontend/i18n/*.json` | Translations; Go resolves the language and sends strings to the page |
| `notify.go` | Which events are due for a reminder, mute labels |
| `update.go`, `signing.go` | In-app updater from GitHub Releases, Ed25519 signature check |
| `icon.go` | Tray icon drawn in code |
| `platform_linux.go`, `platform_other.go` | Platform bits (Linux tray tooltip via D-Bus, taskbar) |
| `frontend/index.html` | Popup: agenda, details, settings |
| `frontend/alert.html` | Maximised meeting-start alert |
| `scripts/dist.sh` | Release archive for the current OS/arch |
| `docs/` | GitHub Pages site: home page and privacy policy (plain HTML, no build) |

## Commands

```bash
make tools          # golangci-lint + govulncheck (re-run after a Go upgrade)
make run            # build and run with dummy.json
make run RUNARGS=   # run with connected accounts only
make showcase       # English UI with showcase.json, for website screenshots
make ci             # fmt-check, tidy-check, vet, lint, test (-race), govulncheck
```

`make ci` must pass before a change is done. On macOS `make run` wraps the binary in
`build/do-not-forget-dev.app`: notifications abort outside an app bundle.

Google in dev builds needs `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` in `.env` (ignored by
Git), or an own client entered in Settings.

## Conventions

- Match the surrounding code: short doc comments that say *why*, no boilerplate.
- Keep `go.mod` lean. The Calendar API is called over plain REST; `oauth2/google` and
  `google.golang.org/api` are deliberately not used.
- Wails is pinned (`v3.0.0-beta.30`); `platform_linux.go` reaches into its internals.
- Every user-facing string goes through `T()` (Go) or `t()` (JS) with a key in **all**
  `frontend/i18n/*.json`. `TestTranslationFiles` enforces matching keys and placeholders,
  every plural form a language needs (and no others), and no HTML in translations.
- Commit messages: imperative subject, body explaining why. No AI co-author trailers or
  "Generated with" lines.

## Invariants (security)

- OAuth tokens and the own client secret live **only** in the system keychain. Never in
  `config.json`, logs or another file. If the keychain fails, adding an account fails.
- A disconnected `googleSource` must never save a token again (`savingTokenSource.close`).
- Connect and disconnect go through `accountManager` so they can't interleave.
- Links reach the OS only through the backend (`openURL` / the `open` event), http(s) only.
  The frontend never calls Wails' `Browser.OpenURL` directly.
- Meeting services (Meet, Zoom, Teams) are recognised by exact host in `meetingService`,
  never by a substring of the URL.
- Values inserted into HTML are escaped with `esc()`; translations are inserted as is.
- Updates are verified against `SHA256SUMS` and an Ed25519 signature (`signing.go`).
- A change to what data the app accesses, stores or sends (scopes, config fields, network
  hosts) must update `docs/privacy.html`; Google's verification relies on it being accurate.

## Tests

Plain `go test`; fakes over mocks (`httptest` servers, `keyring.MockInit`, `dummySource`).
UI changes have no automated tests: check them with `make run` and say so if you couldn't.
