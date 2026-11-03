//go:build !windows

package xinput

// newAPI: XInput exists only on Windows.
func newAPI() API { return nil }

// Available reports whether an XInput DLL could be loaded (Windows only).
func Available() bool { return false }
