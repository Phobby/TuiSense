// Package termcolor corrects colour-depth detection for terminals that claim
// more colour than they can draw.
//
// macOS Terminal.app gained 24-bit colour only in macOS 26 (Tahoe). Earlier
// versions read the numbers of a 24-bit colour code ("38;2;R;G;B") as separate
// attributes — blink, reverse, hidden — so a 3D model, where every dot has its
// own colour, is drawn in shreds. Limoni assumes 24-bit colour whenever
// TERM_PROGRAM is Apple_Terminal; Apply removes that hint on older systems so
// Limoni falls back to the 256-colour palette TERM announces.
package termcolor

import (
	"os"
	"runtime"
	"strings"
)

// LegacyAppleTerminal reports whether the session is Terminal.app on a macOS
// without 24-bit colour. macOSMajor is the macOS major version (0 if unknown,
// which is treated as old: 256 colours are always safe).
func LegacyAppleTerminal(goos string, getenv func(string) string, macOSMajor int) bool {
	if goos != "darwin" || getenv("TERM_PROGRAM") != "Apple_Terminal" {
		return false
	}
	if ct := strings.ToLower(getenv("COLORTERM")); ct == "truecolor" || ct == "24bit" {
		return false
	}
	// macOS 26 reports itself as 16.x to programs built with older SDKs.
	return macOSMajor < 16
}

// Apply adjusts the environment before the terminal UI starts. It reports
// whether it changed anything.
func Apply() bool {
	if !LegacyAppleTerminal(runtime.GOOS, os.Getenv, MacOSMajor()) {
		return false
	}
	_ = os.Unsetenv("TERM_PROGRAM")
	return true
}
