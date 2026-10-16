# TODO

What's left, roughly in the order we want to do it within each section.

## Google

- [ ] Google verification: verified domain for the home page and privacy policy, scope
      justification and demo video for `calendar.events`, then publish the app ("In production";
      in Testing, tokens expire after 7 days)
- [ ] `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` as GitHub Actions secrets for release builds
- [ ] Check over a few days with real accounts: token refresh, RSVP, disconnect
- [ ] RSVP for the whole series, not just one occurrence

## Settings that work

- [ ] Wire the rest of the Settings mock to `config.json`: notifications (minutes before, also at
      the start, only meetings with a link, sound, full-screen alert), theme, showing declined
      events, sync interval
- [ ] Start at login (Linux `.desktop` autostart, macOS LaunchAgent, Windows Startup)
- [ ] Light theme: save the choice; a dark tray icon variant for light panels (`icon.go`)

## Meeting apps

- [ ] Open meetings directly in the installed app (`zoommtg://`, `msteams://`), otherwise the
      browser; the link guard in `openURL` must allow those schemes explicitly

## Before going public

- [ ] Make the repository public and turn on GitHub Pages (`main` / `/docs`) and private
      vulnerability reporting
- [ ] Test an update end to end: run `v0.0.x`, update to the next release (a private repository
      returns 404 to the updater, which Wails treats as "up to date")
- [ ] Try on real Windows and on Linux with Google (tray, notifications, alert, updater, keychain)
- [ ] Screenshots in the README and on the website (`make showcase`)
- [ ] CI: build and test on macOS and Windows, nightly govulncheck

## Later

- [ ] More providers: Microsoft 365 / Outlook, iCloud, CalDAV / iCal
- [ ] macOS notarisation (Apple Developer account), Windows code signing
- [ ] Packages: `.deb`, AppImage, Flathub, AUR, Homebrew cask, winget

## Technical debt

- [ ] The Linux tooltip goes through reflection into Wails (`platform_linux.go`): send Wails a PR
      for `setTooltip`, then remove it
- [ ] `GDK_BACKEND=x11` for window placement on Wayland (`main.go`): move to layer-shell once
      Wails supports it
- [ ] Wails is pinned to `v3.0.0-beta.30`; on upgrade check the tooltip on KDE
- [ ] After unmuting, the update notification can take up to 6 h to come back
- [ ] macOS: the muted tray icon isn't a template (doesn't follow light / dark menu bars);
      the alert over full-screen Spaces
