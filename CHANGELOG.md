# Changelog

Notable changes, newest first. The format follows [Keep a Changelog](https://keepachangelog.com/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Answer a recurring meeting for the whole series: after Yes / Maybe / No, pick "This event"
  or "All events", as in Google Calendar. In the agenda and in the meeting-start alert.
- An answer that couldn't be sent now says so under the buttons, instead of quietly reverting.

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

[Unreleased]: https://github.com/mkelcik/do-not-forget/compare/v0.8.4...HEAD
[0.8.4]: https://github.com/mkelcik/do-not-forget/compare/v0.8.3...v0.8.4
[0.8.3]: https://github.com/mkelcik/do-not-forget/compare/v0.8.2...v0.8.3
[0.8.2]: https://github.com/mkelcik/do-not-forget/compare/v0.8.1...v0.8.2
[0.8.1]: https://github.com/mkelcik/do-not-forget/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/mkelcik/do-not-forget/releases/tag/v0.8.0
