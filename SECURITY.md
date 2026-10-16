# Security Policy

## Supported versions

Only the latest release gets fixes. The app updates itself; please check you're on the latest
version before reporting.

## Reporting a vulnerability

Please **don't open a public issue**. Report it privately through GitHub:
[Security → Report a vulnerability](https://github.com/mkelcik/do-not-forget/security/advisories/new).

Include what you found, how to reproduce it and what an attacker could do with it. You'll get
an answer within a week; once a fix is released, the advisory is published and, if you like,
credits you.

## What's in scope

The parts that matter most:

- OAuth sign-in (loopback redirect, PKCE, state) and token handling: tokens must only ever be
  in the system keychain, a disconnected account must not keep or regain access, and a
  disconnect Google didn't confirm must not look like one that succeeded.
- Links from event data: only http(s) may be opened, and a link must not be presented as a
  meeting service (Meet, Zoom, Teams) it doesn't belong to.
- HTML injection from event titles, descriptions or other calendar data into the app's windows.
- The updater: downloads must be verified against `SHA256SUMS` and Ed25519 signatures, one of
  them over a manifest binding the version, OS, architecture and hash (no downgrades or
  cross-platform swaps).

Not vulnerabilities: the Google client secret built into release binaries (desktop OAuth
clients can't keep a secret; PKCE protects the sign-in), and anything that needs an attacker
who already controls your user account.
