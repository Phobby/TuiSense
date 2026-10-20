// Package i18n translates TuiSense's user interface. English source strings
// are the catalog keys, so a missing translation always falls back to the
// original English text and never breaks the UI.
//
// Usage:
//
//	i18n.T("Start drift test")                 // plain text
//	i18n.Tf("%d of %d buttons", seen, total)   // fmt verbs; translations may
//	                                           // reorder them with %[2]d
//
// Technical identifiers, JSON keys, and log messages stay in English.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// Lang is a supported UI language.
type Lang string

const (
	English Lang = "en"
	Turkish Lang = "tr"
)

// Languages lists the selectable languages in display order.
var Languages = []Lang{English, Turkish}

// Name returns the language's own name (always shown untranslated).
func (l Lang) Name() string {
	if l == Turkish {
		return "Türkçe"
	}
	return "English"
}

var current atomic.Value // Lang

func init() { current.Store(English) }

// Parse converts a persisted value; unknown values yield ok=false.
func Parse(s string) (Lang, bool) {
	switch Lang(strings.ToLower(strings.TrimSpace(s))) {
	case English:
		return English, true
	case Turkish:
		return Turkish, true
	}
	return English, false
}

// Detect picks the language from the POSIX locale environment.
func Detect() Lang {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "tr") {
				return Turkish
			}
			return English
		}
	}
	return English
}

// Set changes the active language (safe from any goroutine).
func Set(l Lang) { current.Store(l) }

// Current returns the active language.
func Current() Lang { return current.Load().(Lang) }

// T translates s into the active language.
func T(s string) string {
	if Current() == Turkish {
		if v, ok := turkish[s]; ok {
			return v
		}
	}
	return s
}

// Tf translates a format string and applies fmt.Sprintf.
func Tf(format string, args ...any) string {
	return fmt.Sprintf(T(format), args...)
}

// Has reports whether s has a translation for lang (used by tests).
func Has(lang Lang, s string) bool {
	if lang == English {
		return true
	}
	_, ok := turkish[s]
	return ok
}
