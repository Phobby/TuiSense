package doctor

import (
	"errors"
	"strings"
	"testing"
)

func env(goos string, vars map[string]string) Env {
	return Env{GOOS: goos, Getenv: func(k string) string { return vars[k] },
		Stat: func(string) error { return errors.New("missing") }, Writable: func(string) bool { return false },
		Exe: "/home/u/.local/bin/tuisense"}
}

func find(fs []Finding, area string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Area == area {
			out = append(out, f)
		}
	}
	return out
}

func TestLinuxFindingsAreActionable(t *testing.T) {
	e := env("linux", map[string]string{"LANG": "C", "TERM": "xterm"})
	e.DualSense, e.DualSenseDenied = 1, 1
	fs := Check(e)
	if f := find(fs, "Terminal"); len(f) != 1 || f[0].Level != Warn || !strings.Contains(f[0].Detail, "UTF-8") {
		t.Fatalf("non-UTF-8 locales break Braille rendering: %+v", f)
	}
	if f := find(fs, "Clipboard"); len(f) != 1 || f[0].Level != Warn || !strings.Contains(f[0].Detail, "wl-clipboard") {
		t.Fatalf("missing clipboard tools must be explained: %+v", f)
	}
	if f := find(fs, "Permissions"); len(f) != 1 || !strings.Contains(f[0].Detail, "udev") {
		t.Fatalf("permission problems must be explained: %+v", f)
	}
	if f := find(fs, "Updates"); len(f) != 1 || !strings.Contains(f[0].Detail, "no pinned signing key") {
		t.Fatalf("update trust must be stated: %+v", f)
	}
}

func TestOtherPlatformsAreHonest(t *testing.T) {
	for _, goos := range []string{"windows", "darwin"} {
		e := env(goos, map[string]string{"LANG": "en_US.UTF-8", "WT_SESSION": "1"})
		e.Clipboard = []string{"pbcopy"}
		fs := Check(e)
		want := map[string]string{"windows": "XInput", "darwin": "No controller backend"}[goos]
		if f := find(fs, "Controllers"); len(f) != 1 || !strings.Contains(f[0].Detail, want) {
			t.Fatalf("%s: controller support must not be overstated: %+v", goos, f)
		}
		if len(find(fs, "Permissions")) != 0 {
			t.Fatalf("%s: no Linux permission advice", goos)
		}
	}
	e := env("linux", map[string]string{"LANG": "en_US.UTF-8"})
	e.Exe = "/usr/bin/tuisense"
	if f := find(Check(e), "Updates"); !strings.Contains(f[0].Detail, "package manager") {
		t.Fatal("managed installs are pointed to their package manager")
	}
}
