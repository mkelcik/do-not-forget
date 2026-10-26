# Changelog

Notable changes, newest first. The format follows [Keep a Changelog](https://keepachangelog.com/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- "Check for updates" in Settings › About checks GitHub Releases right away instead of waiting
  for the next check (every 6 hours) and says whether you have the latest version.

## [0.12.0] - 2026-10-26

### Added

- Snooze: the meeting-start alert has "Remind me in 1 min" and "in 5 min", and reminders have
  "Snooze 5 min". The meeting comes back unless it was declined or is over by then.
- Mute or hide an event, or its whole series, from its details in the agenda. A muted event gets
  no reminders or alert; a hidden one also leaves the agenda. A 🔕 tab next to the
  day tabs lists both, with Undo; it shows only while there are some.
- A bell next to each calendar in Settings turns reminders and the alert off for its events.
- Reminder filters: only accepted meetings, only meetings with other people.
- "Reminder times from the calendar": remind at the times set on each event in Google Calendar
  (its own or the calendar's default notifications) instead of one time for every meeting.

## [0.11.1] - 2026-10-24

### Changed

- A `.deb` or `.rpm` install updates from the app: Update downloads and verifies the new
  package, installs it after asking for your password and restarts. It used to open the
  download page. This works from 0.11.1 on, so install this version by hand once.

## [0.11.0] - 2026-10-23

### Added

- An account whose Google sign-in expired or was revoked now says so above the agenda and in
  Settings, with a "Connect again" button that opens Google's sign-in for that account.

## [0.10.0] - 2026-10-22

### Added

- `.rpm` packages for Fedora and openSUSE (x86-64 and ARM64), installed with
  `sudo dnf install ./do-not-forget_<version>_<arch>.rpm` (or `sudo zypper install`).
- The AUR package `do-not-forget-bin` for Arch Linux.
- A Flatpak package for Flathub. It starts at login through the Background portal and leaves
  updates to Flatpak. Each release now also carries the source with vendored Go modules, which
  Flathub builds from.

### Fixed

- Settings › Menu bar options meant for macOS only no longer show on Linux and Windows.
- A failed change to "Start at login" shows a translated message instead of the raw error.

## [0.9.0] - 2026-10-22

### Added

- Answer a recurring meeting for the whole series: after Yes / Maybe / No, pick "This event"
  or "All events", as in Google Calendar. In the agenda and in the meeting-start alert.
- An answer that couldn't be sent now says so under the buttons, instead of quietly reverting.
- `.deb` packages for Ubuntu, Kubuntu and Debian (x86-64 and ARM64), installed with
  `sudo apt install ./do-not-forget_<version>_<arch>.deb`.

### Changed

- The website moved to [donotforget.dev](https://donotforget.dev/), with new screenshots and
  the meeting-start alert.

### Security

- Built with Go 1.27.3, which fixes a certificate verification issue in `crypto/x509`
  (GO-2026-6809).

## [0.8.4] - 2026-10-16

### Added

- Zoom and Teams meetings open in their desktop apps when installed, otherwise in the browser
  (Settings › General). Zoom links with a registration token keep opening in the browser.

### Changed

- Updated Go dependencies (godbus/dbus, x/sys and others).

## [0.8.3] - 2026-10-16

### Added

- Settings now work and are saved: reminders on or off, how many minutes before, a second one
  at the start, sound, the full-screen alert, only meetings with a link, theme, the 7-day tab,
  showing declined events and how often calendars sync.
- Start at login (macOS login item, Windows Run key, Linux autostart).

## [0.8.2] - 2026-10-16

### Fixed

- macOS: the app now asks for permission to show notifications; without it macOS dropped
  reminders and update notices.

### Added

- Settings › About shows a newer version with an Update button, and the settings button gets a
  dot, so updates don't depend on notifications.

### Security

- Updates check a signed manifest of version, OS, architecture and hash, so an old signed
  archive can't be offered as a newer version.
- A disconnect Google doesn't confirm keeps the account and its token, with a retry, removal
  from this computer only, and a link to the Google Account permissions.

## [0.8.1] - 2026-10-16

### Added

- Homebrew: `brew install --cask mkelcik/tap/do-not-forget`; releases update the cask.
- macOS app icon (Dock, Finder, Spotlight, Launchpad).
- Name your accounts ("Work", "Personal") and calendars (a shared one Google calls by its
  owner's email): the names replace the emails everywhere.

## [0.8.0] - 2026-10-16

First public release.

### Added

- Tray / menu bar app for Linux, macOS and Windows: the next meeting with a countdown, and an
  agenda for today, tomorrow and the next 7 days with overlapping meetings grouped as conflicts.
- Google Calendar: several accounts at once, browser sign-in with PKCE, tokens only in the
  system keychain, calendar selection with Google's colours, and RSVP (yes / maybe / no).
- Join with one click: Meet, Zoom and Teams links from the conference data, the location or the
  description. Meet and "open in calendar" open as the account the event came from.
- An own Google OAuth client ID in Settings, for builds without the built-in one or
  organisations that only allow their own apps.
- Reminders before a meeting, a maximised alert when it starts, and mute for an hour, the rest
  of the day or until turned back on.
- macOS: the menu bar shows an icon, icon and text, or text only; the title is shortened to a
  chosen length while the countdown always shows.
- Event details: attendees, organizer, descriptions with line breaks and clickable links that
  fold when long.
- English, Slovak and Czech, chosen automatically or in Settings.
- In-app updates from GitHub Releases, verified by checksum and Ed25519 signature.

### Security

- Disconnecting an account revokes its access at Google and deletes its tokens.
- Links from event data open only as http(s), and meeting services are recognised by exact host.

[Unreleased]: https://github.com/mkelcik/do-not-forget/compare/v0.12.0...HEAD
[0.12.0]: https://github.com/mkelcik/do-not-forget/compare/v0.11.1...v0.12.0
[0.11.1]: https://github.com/mkelcik/do-not-forget/compare/v0.11.0...v0.11.1
[0.11.0]: https://github.com/mkelcik/do-not-forget/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/mkelcik/do-not-forget/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/mkelcik/do-not-forget/compare/v0.8.4...v0.9.0
[0.8.4]: https://github.com/mkelcik/do-not-forget/compare/v0.8.3...v0.8.4
[0.8.3]: https://github.com/mkelcik/do-not-forget/compare/v0.8.2...v0.8.3
[0.8.2]: https://github.com/mkelcik/do-not-forget/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/mkelcik/do-not-forget/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/mkelcik/do-not-forget/releases/tag/v0.8.0
