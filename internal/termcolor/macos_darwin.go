//go:build darwin

package termcolor

import (
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// MacOSMajor returns the macOS major version (15 for "15.5"), or 0.
func MacOSMajor() int {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return 0
	}
	major, _ := strconv.Atoi(strings.SplitN(strings.TrimSpace(v), ".", 2)[0])
	return major
}
