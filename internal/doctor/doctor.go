// Package doctor implements `tuisense doctor`: a quick, read-only check of
// everything TuiSense depends on at runtime. It changes nothing and does not
// use the network.
package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Phobby/TuiSense/internal/clipboard"
	"github.com/Phobby/TuiSense/internal/compat"
	"github.com/Phobby/TuiSense/internal/system"
	"github.com/Phobby/TuiSense/internal/system/evdev"
	"github.com/Phobby/TuiSense/internal/update"
	"github.com/Phobby/TuiSense/internal/version"
)

// Level of a finding.
type Level int

const (
	OK Level = iota
	Info
	Warn
)

// Finding is one line of the report.
type Finding struct {
	Level  Level
	Area   string
	Detail string
}

// Env abstracts the inspected system (faked in tests).
type Env struct {
	GOOS      string
	Getenv    func(string) string
	Stat      func(string) error
	Writable  func(string) bool
	LookPath  func(string) (string, error)
	Clipboard []string
	Exe       string
	Keys      int
	// Controllers counts DualSense hidraw nodes and evdev gamepads, with
	// how many are not accessible.
	DualSense, DualSenseDenied, Gamepads, GamepadsDenied int
}

// System inspects the real machine.
func System() Env {
	e := Env{GOOS: runtime.GOOS, Getenv: os.Getenv, Stat: func(p string) error { _, err := os.Stat(p); return err },
		Writable: compat.SystemEnv{}.Writable, LookPath: exec.LookPath, Clipboard: clipboard.New().Available(), Keys: len(update.TrustedKeys())}
	if exe, err := os.Executable(); err == nil {
		e.Exe = exe
	}
	if runtime.GOOS == "linux" {
		if devs, err := system.ScanDualSenseDevices(); err == nil {
			for _, d := range devs {
				e.DualSense++
				if !d.Readable || !e.Writable(d.HIDRawPath) {
					e.DualSenseDenied++
				}
			}
		}
		if devs, err := evdev.Scan("/sys", "/dev/input"); err == nil {
			for _, d := range devs {
				if d.IsGamepad() && !d.Virtual() {
					e.Gamepads++
					if f, err := os.Open(d.EventPath); err != nil {
						e.GamepadsDenied++
					} else {
						f.Close()
					}
				}
			}
		}
	}
	return e
}

// Check produces the findings for env.
func Check(e Env) []Finding {
	var out []Finding
	add := func(l Level, area, detail string) { out = append(out, Finding{l, area, detail}) }
	add(OK, "TuiSense", fmt.Sprintf("%s on %s/%s", version.Display(), e.GOOS, runtime.GOARCH))

	// Terminal rendering: UTF-8 (Braille 3D model), colour depth, environment.
	locale := e.Getenv("LC_ALL") + e.Getenv("LC_CTYPE") + e.Getenv("LANG")
	switch {
	case e.GOOS == "windows" && e.Getenv("WT_SESSION") != "":
		add(OK, "Terminal", "Windows Terminal")
	case e.GOOS == "windows":
		add(Warn, "Terminal", "Use Windows Terminal for colours, mouse input and Unicode Braille rendering")
	case !strings.Contains(strings.ToUpper(locale), "UTF-8") && !strings.Contains(strings.ToUpper(locale), "UTF8"):
		add(Warn, "Terminal", "The locale is not UTF-8; the 3D view needs Unicode Braille characters (set LANG to a UTF-8 locale)")
	default:
		add(OK, "Terminal", fmt.Sprintf("TERM=%s, UTF-8 locale", e.Getenv("TERM")))
	}
	if ct := strings.ToLower(e.Getenv("COLORTERM")); ct == "truecolor" || ct == "24bit" || e.Getenv("WT_SESSION") != "" {
		add(OK, "Colours", "24-bit colour")
	} else {
		add(Info, "Colours", "24-bit colour not announced (COLORTERM); colours may be approximated")
	}

	// Clipboard.
	if len(e.Clipboard) > 0 {
		add(OK, "Clipboard", strings.Join(e.Clipboard, ", "))
	} else if e.GOOS == "linux" {
		add(Warn, "Clipboard", "No clipboard tool found: install wl-clipboard (Wayland) or xclip/xsel (X11), or set TUISENSE_OSC52=1 for terminal clipboard support")
	} else {
		add(Warn, "Clipboard", "No clipboard tool found; reports can still be saved to files")
	}

	// Controllers and permissions.
	switch e.GOOS {
	case "windows":
		add(Info, "Controllers", "XInput controllers are read through Microsoft's XInput API (experimental; the model is not identified). DualSense, DualShock 4 and Switch Pro are not supported on Windows yet")
	case "linux":
		add(Info, "Controllers", fmt.Sprintf("%d DualSense (hidraw), %d other gamepad(s) found", e.DualSense, e.Gamepads))
		if e.DualSenseDenied > 0 {
			l := Warn
			detail := "A DualSense is not accessible; install TuiSense's udev rule (tuisense --print-udev) or use Device Details → Driver & Compatibility"
			if e.Stat(compat.UdevRulePath) == nil {
				detail = "The udev rule is installed; reconnect the controller"
			}
			add(l, "Permissions", detail)
		}
		if e.GamepadsDenied > 0 {
			add(Warn, "Permissions", "A gamepad's input node is not readable; log in on the local seat or add your user to the 'input' group")
		}
		if e.Stat("/proc/asound/cards") == nil {
			add(OK, "Audio", "ALSA available (controller audio tests over USB)")
		} else {
			add(Info, "Audio", "ALSA not found; controller audio tests are unavailable")
		}
		if e.Writable("/dev/uinput") {
			add(Info, "uinput", "/dev/uinput is writable (not used by TuiSense)")
		}
	default:
		add(Info, "Controllers", "No controller backend is implemented on this system yet; the gallery, profiles and reports work")
	}

	// Application updates.
	switch {
	case update.Managed(e.Exe):
		add(Info, "Updates", "Installed by a package manager; update with that package manager")
	case e.Keys == 0:
		add(Info, "Updates", "This build has no pinned signing key: it checks for new versions but does not download them")
	default:
		add(OK, "Updates", fmt.Sprintf("%d pinned signing key(s); verified updates can be installed", e.Keys))
	}
	return out
}

// Run prints the report.
func Run(w io.Writer) int {
	for _, f := range Check(System()) {
		icon := map[Level]string{OK: "✓", Info: "·", Warn: "!"}[f.Level]
		fmt.Fprintf(w, "%s %-12s %s\n", icon, f.Area, f.Detail)
	}
	return 0
}
