# TODO

What's left, roughly in the order we want to do it within each section.

## Google

- [ ] Google verification (app is In production): verify `donotforget.dev` as a Domain
      property in Search Console, point branding (home page, privacy policy, authorised domain)
      at it, re-request branding verification, then submit data access with the scope
      justification and a demo video for `calendar.events`
- [ ] Check over a few days with real accounts: token refresh, RSVP, disconnect
- [ ] RSVP for the whole series, not just one occurrence

## Settings that work

- [ ] A dark tray icon variant for light panels on Linux and Windows (`icon.go`)

## Releases

- [ ] Try on real Windows and on Linux with Google (tray, notifications, alert, updater, keychain)
- [ ] Retake the website screenshot (`make showcase`): the current one still wraps 12-hour times

## Later

- [ ] More providers: Microsoft 365 / Outlook, iCloud, CalDAV / iCal
- [ ] macOS notarisation (Apple Developer account), Windows code signing
- [ ] Packages: `.deb`, AppImage, Flathub, AUR, winget (Homebrew: `mkelcik/homebrew-tap`)

## Technical debt

- [ ] The Linux tooltip goes through reflection into Wails (`platform_linux.go`): send Wails a PR
      for `setTooltip`, then remove it
- [ ] `GDK_BACKEND=x11` for window placement on Wayland (`main.go`): move to layer-shell once
      Wails supports it
- [ ] Wails is pinned to `v3.0.0-beta.30`; on upgrade check the tooltip on KDE
- [ ] After unmuting, the update notification can take up to 6 h to come back (Settings ›
      About shows it right away)
- [ ] macOS: the muted tray icon isn't a template (doesn't follow light / dark menu bars);
      the alert over full-screen Spaces
