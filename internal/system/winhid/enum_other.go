//go:build !windows

package winhid

// Enumerate lists HID collections; Windows only.
func Enumerate() ([]Device, error) { return nil, nil }

// Available reports whether hid.dll could be loaded; Windows only.
func Available() bool { return false }
