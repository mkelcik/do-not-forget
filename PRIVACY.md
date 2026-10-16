# Privacy Policy

Last updated: 16 October 2026

do-not-forget is a desktop app that runs on your computer. It has no servers of its own and
collects no analytics or telemetry. This policy explains what the app accesses and where that
data goes.

## Google Calendar

When you connect a Google account, the app asks Google for two permissions:

- **`calendar.events`**: see your events and change your own response (yes / maybe / no) when
  you RSVP in the app.
- **`calendar.calendarlist.readonly`**: see the names and colours of your calendars, so you can
  choose which ones to show.

The app uses this access only to show your upcoming events, remind you of them and send RSVPs
you choose to make. It talks to Google's APIs directly from your computer. Event data is kept in
memory while the app runs and is never written to disk, sent to the developers or shared with
anyone else.

The app's use and transfer of information received from Google APIs adheres to the
[Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy),
including the Limited Use requirements.

## What is stored on your computer

- **Access tokens** for connected accounts, and the client secret if you enter your own Google
  client ID: only in your system's keychain (macOS Keychain, Secret Service on Linux, Windows
  Credential Manager).
- **Settings** (`config.json` in your user configuration folder): window size, reminder and
  display preferences, language, the email addresses of connected accounts, which calendars are
  hidden, and your own Google client ID if you set one. No tokens or event data.

## Network connections

- **Google** (`accounts.google.com`, `oauth2.googleapis.com`, `www.googleapis.com`): sign-in,
  token refresh and revocation, and calendar data.
- **GitHub** (`api.github.com`, `github.com`): release builds check for updates every few hours and
  download new versions from this project's GitHub Releases. GitHub sees your IP address, as with
  any web request; the app sends no information about you or your calendars.

Links you click (meeting links, "open in calendar") open in your web browser.

## Removing your data

- **Disconnect** an account in Settings: the app revokes its access at Google and deletes its
  tokens from the keychain.
- You can also revoke access at any time in your
  [Google Account permissions](https://myaccount.google.com/permissions).
- Uninstalling the app and deleting its configuration folder removes everything else.

## Changes and contact

Changes to this policy are published in this file in the project's repository. Questions:
open an issue at <https://github.com/mkelcik/do-not-forget/issues>.
