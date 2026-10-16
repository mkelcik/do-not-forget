# Contributing

Thanks for helping! Bug reports, translations and pull requests are all welcome.

## Before you start

- **Bugs and ideas**: open an [issue](https://github.com/mkelcik/do-not-forget/issues). For a
  bigger change, describe it in an issue first, so we can agree on the approach before you
  spend time on it.
- **Security problems**: don't open an issue, see [SECURITY.md](SECURITY.md).

## Development setup

Requirements: Go (the version in `go.mod`), Make, and on Linux
`libgtk-4-dev libwebkitgtk-6.0-dev`.

```bash
make tools          # golangci-lint and govulncheck
make run            # the app with fake events from dummy.json
make ci             # everything CI checks; must pass
```

To try Google Calendar, create your own OAuth client (see
[README → Your own Google client ID](README.md#your-own-google-client-id)) and either put it in
`.env` as `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` or enter it in Settings. Then
`make run RUNARGS=` runs with your accounts only.

[AGENTS.md](AGENTS.md) describes the code layout, conventions and the security invariants a
change must keep.

## Pull requests

- One topic per pull request, with a test for behaviour that can be tested (`go test`).
- Run `make ci` and, for UI changes, try them in the app; mention what you checked.
- Commit messages: a short imperative subject ("Show the calendar on all-day events") and a
  body that explains why.
- By contributing you agree your work is released under the [MIT License](LICENSE).

## Translations

Strings live in `frontend/i18n/<language>.json`, named by BCP 47 tag (`de.json`, `pt-BR.json`).
To add a language, copy `en.json`, set `"_name"` to the language's own name ("Deutsch") and
translate the values. Keep `{{placeholders}}` as they are, add every plural form your language
needs (`_one`, `_few`, `_other`, …) and no HTML. `go test -run TestTranslationFiles` checks all
of that. The language appears in Settings automatically.
