# do-not-forget

[![CI](https://github.com/mkelcik/do-not-forget/actions/workflows/ci.yml/badge.svg)](https://github.com/mkelcik/do-not-forget/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/mkelcik/do-not-forget?include_prereleases)](https://github.com/mkelcik/do-not-forget/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A tray app for Linux, macOS and Windows that keeps your next meeting one glance away,
in the spirit of [MeetingBar](https://github.com/leits/MeetingBar).

- **Next meeting in the tray**: hover for title, calendar and countdown; click for the agenda.
- **Today and tomorrow** from several calendars merged into one list, with overlapping meetings grouped as conflicts.
- **Join with one click**: Google Meet, Zoom and Teams links are detected.
- **RSVP** (yes / maybe / no) without opening the calendar.
- **Reminders** before a meeting, and a **maximised alert at the start** for when a notification goes unnoticed.
- **Mute** reminders for an hour, the rest of the day, or until you turn them back on.
- **Updates itself** from GitHub Releases, with checksum verification.

> Status: early development. The UI and reminders work against a local development
> source; the Google Calendar connector is in progress.

## Install

Download the archive for your system from [Releases](https://github.com/mkelcik/do-not-forget/releases):

| System | File | Notes |
| --- | --- | --- |
| Linux x86-64 / ARM64 | `do-not-forget_linux_*.tar.gz` | Needs GTK 4 and WebKitGTK 6 (`sudo apt install libgtk-4-1 libwebkitgtk-6.0-4`). GNOME needs the AppIndicator extension for tray icons. |
| macOS (Apple Silicon) | `do-not-forget_darwin_arm64.zip` | Not notarized yet: right-click the app → Open on first launch. |
| Windows x86-64 | `do-not-forget_windows_amd64.zip` | SmartScreen may warn about an unknown publisher. |

## Development

Requirements: Go 1.27.2+, Make, and on Linux `libgtk-4-dev libwebkitgtk-6.0-dev`.

```bash
make tools   # golangci-lint, govulncheck (re-run after upgrading Go)
make run     # starts with fake events from dummy.json
make ci      # everything CI checks
```

`dummy.json` is a development event source: times are `"HH:MM"` (with `"day": 1` for tomorrow)
or offsets from app start like `"+12m"`. `-preview-alert` shows the meeting-start alert right away.

Releases: tag a commit on `main` (`git tag v0.1.0 && git push origin v0.1.0`); CI builds
Linux, macOS and Windows archives and publishes them with `SHA256SUMS`.

Built with Go and [Wails v3](https://v3alpha.wails.io/).

## License

[MIT](LICENSE)
