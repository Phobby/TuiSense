//go:build !darwin

package termcolor

// MacOSMajor returns 0 outside macOS.
func MacOSMajor() int { return 0 }
