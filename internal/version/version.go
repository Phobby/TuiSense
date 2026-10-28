// Package version is the single source of TuiSense's application version.
//
// Release builds set Version through the linker from the repository's
// VERSION file or the release tag:
//
//	go build -ldflags "-X github.com/Phobby/TuiSense/internal/version.Version=0.8.0" ./cmd/tuisense
//
// Development builds fall back to the module version recorded by the Go
// toolchain (e.g. `go install …@v0.8.0`) and otherwise report "dev".
package version

import (
	"runtime/debug"
	"strings"
)

// Version is injected at build time (without a leading "v").
var Version = ""

// String returns the semantic version without a leading "v" ("dev" when unknown).
func String() string {
	if v := strings.TrimPrefix(strings.TrimSpace(Version), "v"); v != "" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimPrefix(bi.Main.Version, "v"); v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

// Display is the short form shown in the UI ("v0.8.0" or "dev").
func Display() string {
	v := String()
	if v == "dev" {
		return v
	}
	return "v" + v
}
