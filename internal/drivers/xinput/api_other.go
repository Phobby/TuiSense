//go:build !windows

package xinput

// newAPI: XInput exists only on Windows.
func newAPI() API { return nil }
