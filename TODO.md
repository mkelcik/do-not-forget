# TODO

What's left, roughly in the order we want to do it within each section.

## Google

- [ ] Check over a few days with real accounts: token refresh, RSVP, disconnect

## Settings that work

- [ ] A dark tray icon variant for light panels on Linux and Windows (`icon.go`)

## Releases

- [ ] Try on real Windows and on Linux with Google (tray, notifications, alert, updater, keychain)

## Later

- [ ] More providers: Microsoft 365 / Outlook, iCloud, CalDAV / iCal
- [ ] macOS notarisation (Apple Developer account), Windows code signing
- [ ] Packages: AppImage, Flathub, AUR, winget (done: Homebrew `mkelcik/homebrew-tap`, `.deb`)

## Technical debt

- [ ] The Linux tooltip goes through reflection into Wails (`platform_linux.go`): Wails v3.1.0
      implements `SetTooltip`, but its title follows `SetLabel` (the SNI Title); switch if that's
      acceptable on KDE, then remove the reflection
- [ ] `GDK_BACKEND=x11` for window placement on Wayland (`main.go`): move to layer-shell once
      Wails supports it
- [ ] Wails is pinned to `v3.1.0`; check the tooltip on KDE after this and every upgrade
- [ ] After unmuting, the update notification can take up to 6 h to come back (Settings ›
      About shows it right away)
- [ ] macOS: the muted tray icon isn't a template (doesn't follow light / dark menu bars);
      the alert over full-screen Spaces
