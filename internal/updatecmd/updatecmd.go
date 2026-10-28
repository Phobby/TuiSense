// Package updatecmd implements `tuisense update`: a foreground check,
// verified download and install of TuiSense itself. It never touches
// controller firmware.
package updatecmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Phobby/TuiSense/internal/system"
	"github.com/Phobby/TuiSense/internal/update"
)

// Config is everything the command needs (overridden in tests).
type Config struct {
	Current  string
	Source   update.Source
	Dir      string
	Exe      string
	Keys     []update.TrustedKey
	GOOS     string
	GOARCH   string
	Validate update.Validator
}

// DefaultConfig describes the running installation.
func DefaultConfig(current string) Config {
	return Config{Current: current, Dir: filepath.Join(system.DataDir(), "updates"), Exe: executable(),
		Keys: update.TrustedKeys(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Validate: update.RunVersion}
}

func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		return r
	}
	return exe
}

// CleanupAfterStart removes temporary files of a previous update once the
// new version has started (the previous executable is kept for --rollback).
func CleanupAfterStart() {
	if exe := executable(); exe != "" {
		update.Cleanup(exe)
	}
}

// Run executes the command and returns the exit code.
func Run(args []string, out io.Writer, current string) int {
	return RunWith(args, out, DefaultConfig(current))
}

// RunWith executes the command with an explicit configuration.
func RunWith(args []string, out io.Writer, c Config) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(out)
	check := fs.Bool("check", false, "only check whether a newer version exists")
	rollback := fs.Bool("rollback", false, "restore the version replaced by the last update")
	pre := fs.Bool("prerelease", false, "include pre-releases")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *rollback {
		if err := update.Rollback(c.Exe); err != nil {
			fmt.Fprintln(out, "Rollback failed:", err)
			return 1
		}
		fmt.Fprintln(out, "Restored the previous TuiSense version.")
		return 0
	}
	cur, err := update.ParseSemver(c.Current)
	if err != nil {
		fmt.Fprintf(out, "TuiSense %s is a development build; install a release to use updates.\n", c.Current)
		return 1
	}
	fmt.Fprintf(out, "Installed: TuiSense v%s (%s/%s)\n", cur, c.GOOS, c.GOARCH)
	ch := update.Stable
	if *pre {
		ch = update.Prerelease
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	rel, err := c.Source.Latest(ctx, ch)
	cancel()
	if err != nil {
		fmt.Fprintln(out, "Could not check for updates:", err)
		return 1
	}
	if rel.Version.Compare(cur) <= 0 {
		fmt.Fprintf(out, "TuiSense is up to date (latest %s release: v%s).\n", ch, rel.Version)
		return 0
	}
	fmt.Fprintf(out, "A new version is available: v%s\n", rel.Version)
	if *check {
		return 0
	}
	if update.Managed(c.Exe) {
		fmt.Fprintln(out, update.ErrManaged)
		return 1
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	st, err := c.Source.Stage(ctx, rel, c.Dir, c.GOOS, c.GOARCH, c.Keys, nil)
	if err != nil {
		fmt.Fprintln(out, "The update was not installed:", err)
		if errors.Is(err, update.ErrNoTrustedKey) {
			fmt.Fprintln(out, "Download the release manually and verify it as described in the README.")
		}
		return 1
	}
	fmt.Fprintf(out, "Downloaded and verified v%s (signed by %s).\n", st.Version, st.Signer)
	if err := update.Apply(st, c.Exe, c.Validate); err != nil {
		fmt.Fprintln(out, "The update was not installed:", err)
		fmt.Fprintln(out, "The installed version is unchanged.")
		return 1
	}
	_ = update.Discard(c.Dir)
	fmt.Fprintf(out, "TuiSense was updated to v%s. Previous version kept for `tuisense update --rollback`.\n", st.Version)
	return 0
}
