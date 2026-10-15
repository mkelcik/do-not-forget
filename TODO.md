# TODO

Čo ostáva spraviť. Poradie v rámci sekcie je zhruba poradie, v akom to chceme robiť.

## 1. Google konektor

- [ ] Vytvoriť projekt v Google Cloud Console, zapnúť Calendar API, OAuth consent screen
      (scope `calendar.events`), OAuth client typu „Desktop app“, prepnúť na „In production“
      (v režime Testing tokeny vypršia po 7 dňoch)
- [ ] Nastaviť secrets `GOOGLE_CLIENT_ID` a `GOOGLE_CLIENT_SECRET`, lokálne `.env`
      (build ich už vkladá cez `-X main.googleClientID` / `main.googleClientSecret`, premenné v Go ešte nie sú)
- [ ] Prihlásenie: prehliadač + loopback na `127.0.0.1` + PKCE
- [ ] Tokeny do systémovej kľúčenky (`go-keyring`: Secret Service / Keychain / Credential Manager);
      bez kľúčenky účet nepridať, nikdy do súboru
- [ ] Viac účtov naraz (pracovný + súkromný), každý ako samostatný `Source`
- [ ] Výber kalendárov v účte, farby z Google
- [ ] Načítanie dnes + zajtra (`singleEvents=true`), `htmlLink` → `CalendarURL`, organizátor, účastníci, moja odpoveď
- [ ] Odkaz na meeting z `conferenceData`, potom z miesta a popisu (Meet, Zoom, Teams)
- [ ] RSVP cez `events.patch` (poslať celý zoznam účastníkov, zmeniť len seba; jeden výskyt vs. séria)
- [ ] ID udalostí s prefixom účtu (tá istá pozvánka v dvoch účtoch má rovnaké ID)
- [ ] Vlastné Client ID v nastaveniach (pre forky a tých, čo nechcú náš client)

## 2. Funkčné nastavenia

- [ ] Prepojiť návrh nastavení s `config.json`: účty a kalendáre, notifikácie (minúty vopred,
      aj pri začiatku, len s odkazom, zvuk, celoobrazovkový alert), téma, zobrazovanie odmietnutých,
      odpočet v lište, interval synchronizácie
- [ ] Spustiť po prihlásení (Linux `.desktop` v autostart, macOS LaunchAgent, Windows Startup)
- [ ] Skutočná verzia v „O aplikácii“ (teraz natvrdo `0.1.0-dev`)
- [ ] Svetlá téma: uložiť voľbu, tmavá varianta ikonky v lište (`icon.go`)

## 3. Jazyky

- [ ] Texty do slovníka, angličtina predvolená, slovenčina ako preklad (UI, notifikácie, menu, tooltip)
- [ ] Výber jazyka v nastaveniach

## 4. Akcie podľa typu meetingu

- [ ] Otvárať priamo v aplikácii, keď je nainštalovaná (napr. `zoommtg://`, `msteams://`), inak prehliadač

## 5. Pred zverejnením

- [ ] Zverejniť repozitár
- [ ] Otestovať aktualizáciu end-to-end: spustiť `v0.0.1`, aktualizovať na `v0.0.2`
      (zatiaľ nejde: súkromný repo vráti updateru 404 a Wails to ticho berie ako „aktuálne“)
- [ ] Overenie appky u Google: doména, domovská stránka, privacy policy, odôvodnenie scope
- [ ] Vyskúšať na reálnom macOS a Windows (tray, notifikácie, alert, updater)
- [ ] README: screenshoty, inštalácia, stav projektu
- [ ] Súbory pre komunitu: SECURITY, CONTRIBUTING, CODE_OF_CONDUCT, CHANGELOG, šablóny issues a PR
- [ ] CI: dependabot (Wails vynechať, je pripnutý), nočný govulncheck, build na macOS/Windows v CI

## 6. Neskôr

- [ ] Ďalšie konektory: Microsoft 365 / Outlook, iCloud, CalDAV / iCal
- [ ] macOS notarizácia (Apple Developer účet), podpisovanie pre Windows
- [ ] Balíky: `.deb`, AppImage, Flathub, AUR, Homebrew cask, winget

## Technický dlh

- [ ] Tooltip na Linuxe ide cez reflexiu do Wails (`platform_linux.go`): poslať do Wails PR na `setTooltip`, potom odstrániť
- [ ] `GDK_BACKEND=x11` kvôli pozícii okna na Waylande (`main.go`): prejsť na layer-shell, keď ho Wails podporí
- [ ] Wails je pripnutý na `v3.0.0-beta.30`; pri update skontrolovať tooltip na KDE
- [ ] Po zrušení stlmenia môže upozornenie na novú verziu prísť až o 6 h
- [ ] macOS: stlmená ikonka nie je template (nereaguje na svetlý/tmavý menu bar), alert cez fullscreen Spaces
