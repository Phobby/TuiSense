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
		want := map[string]string{"windows": "XInput", "darwin": "without cgo"}[goos]
		if f := find(fs, "Controllers"); len(f) != 1 || !strings.Contains(f[0].Detail, want) {
			t.Fatalf("%s: controller support must not be overstated: %+v", goos, f)
		}
		if len(find(fs, "Permissions")) != 0 {
			t.Fatalf("%s: no Linux permission advice", goos)
		}
	}
	mac := env("darwin", nil)
	mac.MacHID, mac.DualSense = true, 1
	if f := find(Check(mac), "Controllers"); len(f) != 1 || !strings.Contains(f[0].Detail, "1 DualSense found (IOKit HID") {
		t.Fatalf("macOS lists the DualSense it reads: %+v", f)
	}
	e := env("linux", map[string]string{"LANG": "en_US.UTF-8"})
	e.Exe = "/usr/bin/tuisense"
	if f := find(Check(e), "Updates"); !strings.Contains(f[0].Detail, "package manager") {
		t.Fatal("managed installs are pointed to their package manager")
	}
}

// Terminal.app before macOS 26 cannot draw 24-bit colour; doctor says so and
// recommends a terminal that can.
func TestAppleTerminalColourDepth(t *testing.T) {
	vars := map[string]string{"LANG": "en_US.UTF-8", "TERM": "xterm-256color", "TERM_PROGRAM": "Apple_Terminal"}
	e := env("darwin", vars)
	e.MacOSMajor = 15
	if f := find(Check(e), "Colours"); len(f) != 1 || !strings.Contains(f[0].Detail, "256 colours") || !strings.Contains(f[0].Detail, "iTerm2") {
		t.Fatalf("legacy Terminal.app: %+v", f)
	}
	e.MacOSMajor = 26
	if f := find(Check(e), "Colours"); len(f) != 1 || f[0].Level != OK {
		t.Fatalf("Terminal.app on macOS 26: %+v", f)
	}
}

func TestRedactRemovesPersonalPaths(t *testing.T) {
	got := Redact(`Exe C:\Users\Yağız Öztürk\AppData\Local\Programs\TuiSense\bin\tuisense.exe by Yağız Öztürk`, `C:\Users\Yağız Öztürk`, "Yağız Öztürk", "")
	if strings.Contains(got, "Yağız") || !strings.Contains(got, `~\AppData`) {
		t.Fatalf("not redacted: %s", got)
	}
}

func TestInstallMethod(t *testing.T) {
	for exe, want := range map[string]string{
		`C:\Users\a b\AppData\Local\Programs\TuiSense\bin\tuisense.exe`: "PowerShell installer",
		`D:\Tools\TuiSense\tuisense.exe`:                                "portable",
		`C:\Users\a\scoop\apps\tuisense\current\tuisense.exe`:           "package manager",
		"/home/u/.local/bin/tuisense":                                   "install script",
	} {
		if got := installMethod(exe); !strings.Contains(got, want) {
			t.Errorf("%s: %s", exe, got)
		}
	}
}

// The detailed report lists what TuiSense would show for each controller
// and never contains a device path.
func TestHardwareReportIsComplete(t *testing.T) {
	var b strings.Builder
	WriteHardware(&b, "windows", Hardware{Backends: []string{"xinput", "windows-hid"},
		Controllers: []Controller{{Driver: "windows-hid", Model: "MSI Force GC20 V2", ModelID: "msi.force_gc20_v2", Visual: "msi_gc20",
			RawName: "Force GC20 V2", IDs: "0db0:0001:0001", Identity: "identified", Input: "not available"}},
		HID: []HIDEntry{{IDs: "0db0:0001:0001", Usage: "01:05", Product: "Force GC20 V2"}}})
	out := b.String()
	for _, want := range []string{"Controllers found: 1", "msi_gc20", "0db0:0001:0001", "identified", "HID only (not visible to XInput)", "not available"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	b.Reset()
	WriteHardware(&b, "windows", Hardware{})
	if !strings.Contains(b.String(), "No controller detected. Check the USB/Bluetooth connection and try again.") {
		t.Fatal("an empty scan gives an actionable message")
	}
}
