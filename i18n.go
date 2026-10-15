package main

// Translations live in frontend/i18n/<BCP 47 tag>.json (en, sk, pt-BR, …), one flat file per
// language with "_name" set to the language's own name. Every file there is embedded with
// the frontend, so adding a language is adding a file. English is the source and fallback;
// a regional file only needs what differs from its base (pt-BR → pt → en).
// Plurals are i18next-style key suffixes with CLDR categories: key_one, key_few, key_other.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"sync/atomic"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

const i18nDir = "i18n"

// Language is one available translation, as listed in Settings.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type catalog struct {
	files   map[string]map[string]string // canonical tag -> strings
	tags    []language.Tag               // English first: the matcher's default
	matcher language.Matcher
}

// loadCatalog reads every <tag>.json in dir; files with an invalid tag are skipped (CI rejects them).
func loadCatalog(fsys fs.FS) (*catalog, error) {
	entries, err := fs.ReadDir(fsys, i18nDir)
	if err != nil {
		return nil, err
	}
	c := &catalog{files: map[string]map[string]string{}, tags: []language.Tag{language.English}}
	for _, e := range entries {
		code, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		tag, err := language.Parse(code)
		if err != nil {
			log.Printf("i18n: skipping %s: %v", e.Name(), err)
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(i18nDir, e.Name()))
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		c.files[tag.String()] = m
		if tag != language.English {
			c.tags = append(c.tags, tag)
		}
	}
	if c.files["en"] == nil {
		return nil, fmt.Errorf("%s/en.json missing", i18nDir)
	}
	c.matcher = language.NewMatcher(c.tags)
	return c, nil
}

func (c *catalog) languages() []Language {
	out := make([]Language, 0, len(c.tags))
	for _, t := range c.tags {
		out = append(out, Language{Code: t.String(), Name: c.files[t.String()]["_name"]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// resolve picks the best available translation for the user's preferences, most preferred
// first: the language chosen in Settings, else the system's. Falls back to English.
func (c *catalog) resolve(prefs ...string) string {
	var want []language.Tag
	for _, p := range prefs {
		if t, err := language.Parse(p); err == nil {
			want = append(want, t)
		}
	}
	_, i, conf := c.matcher.Match(want...)
	if conf == language.No {
		return "en"
	}
	return c.tags[i].String()
}

// translator returns code's strings layered over its base language and English.
func (c *catalog) translator(code string) *translator {
	tag := language.Make(code)
	merged := map[string]string{}
	layers := []string{"en"}
	if base, _ := tag.Base(); base.String() != "en" {
		layers = append(layers, base.String())
	}
	layers = append(layers, code)
	for _, l := range layers {
		for k, v := range c.files[l] {
			merged[k] = v
		}
	}
	return &translator{lang: tag, code: code, strings: merged}
}

type translator struct {
	lang    language.Tag
	code    string
	strings map[string]string
}

// T translates key, filling {{name}} from args given as name, value pairs.
// A "count" arg selects the plural form (key_one, key_few, …, key_other).
func (t *translator) T(key string, args ...any) string {
	vars := map[string]string{}
	for i := 0; i+1 < len(args); i += 2 {
		vars[fmt.Sprint(args[i])] = fmt.Sprint(args[i+1])
	}
	s, ok := "", false
	if n, isInt := countArg(args); isInt {
		s, ok = t.strings[key+"_"+pluralName(t.lang, n)]
		if !ok {
			s, ok = t.strings[key+"_other"]
		}
	}
	if !ok {
		if s, ok = t.strings[key]; !ok {
			s = key // a missing key shows up visibly instead of as an empty label
		}
	}
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

func countArg(args []any) (int, bool) {
	for i := 0; i+1 < len(args); i += 2 {
		if args[i] == "count" {
			n, ok := args[i+1].(int)
			return n, ok
		}
	}
	return 0, false
}

var pluralNames = map[plural.Form]string{
	plural.Other: "other", plural.Zero: "zero", plural.One: "one",
	plural.Two: "two", plural.Few: "few", plural.Many: "many",
}

// pluralName is the CLDR category of integer n in lang, the same rules as JS Intl.PluralRules.
func pluralName(lang language.Tag, n int) string {
	if n < 0 {
		n = -n
	}
	return pluralNames[plural.Cardinal.MatchPlural(lang, n, 0, 0, 0, 0)]
}

// systemLanguages reads the POSIX locale (sk_SK.UTF-8 → sk-SK). macOS and Windows GUI apps
// often have none; the webview then reports navigator.languages and we re-resolve.
func systemLanguages() []string {
	var out []string
	for _, v := range []string{os.Getenv("LANGUAGE"), os.Getenv("LC_ALL"), os.Getenv("LC_MESSAGES"), os.Getenv("LANG")} {
		for _, l := range strings.Split(v, ":") {
			l, _, _ = strings.Cut(l, ".")
			l, _, _ = strings.Cut(l, "@")
			if l != "" && l != "C" && l != "POSIX" {
				out = append(out, strings.ReplaceAll(l, "_", "-"))
			}
		}
	}
	return out
}

// tr is the active translation; swapped whole when the language changes.
var tr atomic.Pointer[translator]

// T translates with the active language.
func T(key string, args ...any) string { return tr.Load().T(key, args...) }
