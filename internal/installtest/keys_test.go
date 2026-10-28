package installtest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var keyRe = regexp.MustCompile(`ssh-ed25519 [A-Za-z0-9+/=]+`)

func keysIn(t *testing.T, rel string, strip func(string) string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatal(err)
	}
	keys := keyRe.FindAllString(strip(string(b)), -1)
	sort.Strings(keys)
	return keys
}

// The updater and both installers must pin exactly the same release keys.
func TestPinnedKeysAreInSync(t *testing.T) {
	noComments := func(s string) string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(l), "#") {
				out = append(out, l)
			}
		}
		return strings.Join(out, "\n")
	}
	want := keysIn(t, "internal/update/trusted_keys.txt", noComments)
	for _, f := range []string{"scripts/install.sh", "scripts/install.ps1"} {
		got := keysIn(t, f, noComments)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s pins %v, internal/update/trusted_keys.txt pins %v", f, got, want)
		}
	}
}
