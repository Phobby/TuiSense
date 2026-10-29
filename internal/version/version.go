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

// Commit is the Git commit the binary was built from, injected at build time
// (-X github.com/Phobby/TuiSense/internal/version.Commit=<sha>). Builds
// without it fall back to the VCS revision the Go toolchain recorded.
var Commit = ""

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

// CommitHash returns the full Git commit of this build ("" when unknown).
// A build from a modified working tree reports the commit with "-dirty".
func CommitHash() string {
	if c := strings.TrimSpace(Commit); c != "" {
		return c
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	rev, dirty := "", false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev != "" && dirty {
		rev += "-dirty"
	}
	return rev
}

// ShortCommit is the abbreviated commit ("" when unknown).
func ShortCommit() string {
	c := CommitHash()
	if len(c) > 12 && !strings.HasSuffix(c, "-dirty") {
		return c[:12]
	}
	if i := strings.Index(c, "-dirty"); i > 12 {
		return c[:12] + "-dirty"
	}
	return c
}
