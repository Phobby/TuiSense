//go:build windows

package doctor

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// osVersion reports the real Windows build (RtlGetVersion is not shimmed).
func osVersion() string {
	v := windows.RtlGetVersion()
	name := "Windows 10"
	if v.MajorVersion == 10 && v.BuildNumber >= 22000 {
		name = "Windows 11"
	}
	return fmt.Sprintf("%s (%d.%d.%d)", name, v.MajorVersion, v.MinorVersion, v.BuildNumber)
}
