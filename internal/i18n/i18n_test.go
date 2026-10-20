package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	callRe = regexp.MustCompile(`i18n\.(Tf?)\(("(?:[^"\\]|\\.)*")`)
	verbRe = regexp.MustCompile(`%(\[\d+\])?[-+# 0-9.]*(\[\d+\])?[a-zA-Z%]`)
)

type usage struct {
	key    string
	format bool
}

// sourceKeys returns every literal passed to T/Tf in the non-test sources.
func sourceKeys(t *testing.T) []usage {
	t.Helper()
	var out []usage
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/i18n/") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range callRe.FindAllStringSubmatch(string(src), -1) {
			k, err := strconv.Unquote(m[2])
			if err != nil {
				t.Fatalf("%s: bad literal %s", path, m[2])
			}
			out = append(out, usage{key: k, format: m[1] == "Tf"})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func translatable(s string) bool {
	return regexp.MustCompile(`[A-Za-z]{2,}`).MatchString(verbRe.ReplaceAllString(s, ""))
}

func TestEveryUIStringHasATurkishTranslation(t *testing.T) {
	keys := sourceKeys(t)
	if len(keys) < 500 {
		t.Fatalf("expected the UI to route its text through i18n, found only %d keys", len(keys))
	}
	for _, u := range keys {
		if translatable(u.key) && !Has(Turkish, u.key) {
			t.Errorf("missing Turkish translation for %q", u.key)
		}
	}
}

// dummyArgs builds arguments matching the verbs of an English format string.
func dummyArgs(format string) []any {
	var args []any
	for _, m := range verbRe.FindAllString(format, -1) {
		if m == "%%" {
			continue
		}
		switch m[len(m)-1] {
		case 'd', 'x', 'X':
			args = append(args, 7)
		case 'f', 'g', 'e':
			args = append(args, 1.5)
		case 'v':
			args = append(args, true)
		default:
			args = append(args, "x")
		}
	}
	return args
}

func TestTurkishFormatStringsAreValid(t *testing.T) {
	Set(Turkish)
	defer Set(English)
	for _, u := range sourceKeys(t) {
		if !u.format {
			continue
		}
		out := Tf(u.key, dummyArgs(u.key)...)
		if strings.Contains(out, "%!") {
			t.Errorf("Turkish format for %q is broken: %q", u.key, out)
		}
	}
}

func TestFallbackAndSwitching(t *testing.T) {
	Set(English)
	if T("Settings") != "Settings" {
		t.Fatal("English must return the source text")
	}
	Set(Turkish)
	defer Set(English)
	if T("Settings") != "Ayarlar" || T("no such key xyz") != "no such key xyz" {
		t.Fatal("Turkish lookup or fallback failed")
	}
	if got := Tf("%d of %d buttons were not detected.", 2, 19); got != "19 butondan 2 tanesi algılanmadı." {
		t.Fatalf("argument reordering failed: %q", got)
	}
	if l, ok := Parse("TR"); !ok || l != Turkish {
		t.Fatal("parse failed")
	}
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "tr_TR.UTF-8")
	if Detect() != Turkish {
		t.Fatal("tr_TR locale should select Turkish")
	}
}
