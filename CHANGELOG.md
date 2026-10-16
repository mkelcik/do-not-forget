# Changelog

Notable changes, newest first. The format follows [Keep a Changelog](https://keepachangelog.com/);
versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Google Calendar: several accounts at once, browser sign-in with PKCE, tokens in the system
  keychain, calendar selection with Google's colours, RSVP, and meeting links from Meet, Zoom and
  Teams.
- An own Google OAuth client ID in Settings, for builds without the built-in one or
  organisations that only allow their own apps.
- Meet and "open in calendar" links open as the account the event came from.
- macOS: the menu bar shows an icon, icon and text, or text only; the title is shortened to a
  chosen length while the countdown always shows.
- Event descriptions keep their line breaks, have clickable links and fold when long.
- All-day events show their calendar.

### Changed

- macOS: the tray icon is the calendar used on other systems, adapting to light and dark menu
  bars.
- The menu bar text shows only for today's meetings.

### Security

- Disconnecting an account revokes its access at Google and deletes its tokens; a failed
  delete is reported.
- Links from event data open only as http(s), and meeting services are recognised by exact host.

## [0.0.3] - 2026-10-15

### Added

- Translations: English, Slovak and Czech, chosen automatically or in Settings.
- A 7-day tab.

### Fixed

- Release builds use production mode; update checks time out instead of hanging.

## [0.0.2] - 2026-10-15

### Added

- Windows on ARM64 release archives.

## [0.0.1] - 2026-10-15

### Added

- First release: tray app with the next meeting and countdown, agenda for today and tomorrow,
  reminders, a maximised alert when a meeting starts, mute, RSVP, and signed in-app updates.

[Unreleased]: https://github.com/mkelcik/do-not-forget/compare/v0.0.3...HEAD
[0.0.3]: https://github.com/mkelcik/do-not-forget/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/mkelcik/do-not-forget/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/mkelcik/do-not-forget/releases/tag/v0.0.1
