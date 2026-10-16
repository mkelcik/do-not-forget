package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"golang.org/x/text/language"
)

// TestMain sets Slovak as the active language, which the string assertions in other tests use.
func TestMain(m *testing.M) {
	c, err := loadCatalog(os.DirFS("frontend"))
	if err != nil {
		panic(err)
	}
	tr.Store(c.translator("sk"))
	os.Exit(m.Run())
}

var placeholder = regexp.MustCompile(`\{\{(\w+)\}\}`)

func placeholders(s string) string {
	matches := placeholder.FindAllStringSubmatch(s, -1)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, m[1])
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// TestTranslationFiles is what a contributed <tag>.json has to pass: a canonical BCP 47 file
// name, a _name, only keys English has, the same {{placeholders}}, every plural form its
// language needs, and no markup (strings are inserted into the UI as HTML).
func TestTranslationFiles(t *testing.T) {
	c, err := loadCatalog(os.DirFS("frontend"))
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir("frontend/" + i18nDir)
	if len(entries) != len(c.files) {
		t.Errorf("%d files in %s but %d loaded: check file names are BCP 47 tags", len(entries), i18nDir, len(c.files))
	}
	en := c.files["en"]
	isPlural := func(base string) bool { _, ok := en[base+"_other"]; return ok }

	for _, e := range entries {
		code := strings.TrimSuffix(e.Name(), ".json")
		tag, err := language.Parse(code)
		if err != nil {
			t.Errorf("%s: not a BCP 47 language tag: %v", e.Name(), err)
			continue
		}
		if tag.String() != code {
			t.Errorf("%s: rename to %s.json (canonical form)", e.Name(), tag.String())
			continue
		}
		file := c.files[code]
		if file["_name"] == "" {
			t.Errorf("%s: missing \"_name\" (the language's own name, e.g. \"Slovenčina\")", e.Name())
		}
		required := map[string]bool{}
		for n := range 1000 {
			required[pluralName(tag, n)] = true
		}
		hasPlural := map[string]bool{}
		for key, val := range file {
			if strings.ContainsAny(val, "<>") {
				t.Errorf("%s: %s: no HTML in translations", e.Name(), key)
			}
			if i := strings.LastIndex(key, "_"); i > 0 && isPlural(key[:i]) {
				base, form := key[:i], key[i+1:]
				if !required[form] {
					t.Errorf("%s: %s: %q is not a plural form %s uses", e.Name(), key, form, code)
				}
				hasPlural[base] = true
				if placeholders(val) != placeholders(en[base+"_other"]) {
					t.Errorf("%s: %s: placeholders {{%s}}, English has {{%s}}", e.Name(), key, placeholders(val), placeholders(en[base+"_other"]))
				}
				continue
			}
			ref, ok := en[key]
			if !ok {
				t.Errorf("%s: unknown key %s (not in en.json)", e.Name(), key)
				continue
			}
			if key != "_name" && placeholders(val) != placeholders(ref) {
				t.Errorf("%s: %s: placeholders {{%s}}, English has {{%s}}", e.Name(), key, placeholders(val), placeholders(ref))
			}
		}
		for base := range hasPlural {
			for form := range required {
				if _, ok := file[base+"_"+form]; !ok {
					t.Errorf("%s: %s_%s missing (%s needs every form: %v)", e.Name(), base, form, code, keys(required))
				}
			}
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestTranslator(t *testing.T) {
	c, err := loadCatalog(os.DirFS("frontend"))
	if err != nil {
		t.Fatal(err)
	}
	for prefs, want := range map[string]string{
		"sk-SK": "sk", "cs": "cs", "de": "en", "pt-BR": "en", "": "en", "fr,cs-CZ": "cs",
	} {
		if got := c.resolve(strings.Split(prefs, ",")...); got != want {
			t.Errorf("resolve(%q) = %s, want %s", prefs, got, want)
		}
	}

	if got := c.languages(); got[0].Name != "Čeština" {
		t.Errorf("languages() = %v, want Čeština first (collation, not bytes)", got)
	}

	sk := c.translator("sk")
	for n, want := range map[int]string{1: "Dnes ešte 1 udalosť", 3: "Dnes ešte 3 udalosti", 5: "Dnes ešte 5 udalostí", 0: "Dnes ešte 0 udalostí"} {
		if got := sk.T("header.left", "count", n); got != want {
			t.Errorf("sk plural %d = %q, want %q", n, got, want)
		}
	}
	if got := c.translator("en").T("header.left", "count", 1); got != "1 more event today" {
		t.Errorf("en plural = %q", got)
	}
	if got := sk.T("in.min", "m", 12); got != "o 12 min" {
		t.Errorf("vars = %q", got)
	}
	sk.strings = map[string]string{} // nothing translated: key shows, so gaps are visible
	if got := sk.T("missing.key"); got != "missing.key" {
		t.Errorf("missing = %q", got)
	}
}
