package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Errors returned when applying an update.
var (
	ErrManaged  = errors.New("TuiSense was installed by a package manager; update it with that package manager")
	ErrReadOnly = errors.New("the installation directory is not writable; reinstall with the official installer")
	ErrInvalid  = errors.New("the new executable failed its start-up check")
)

// managedPrefixes are locations owned by system package managers.
var managedPrefixes = []string{
	"/usr/bin/", "/usr/sbin/", "/usr/lib/", "/usr/libexec/", "/usr/share/", "/bin/", "/sbin/",
	"/nix/store/", "/snap/", "/opt/homebrew/", "/usr/local/Cellar/", "/home/linuxbrew/", "/var/lib/flatpak/",
}

var managedWindows = []string{`\windowsapps\`, `\scoop\apps\`, `\chocolatey\`, `\winget\packages\`}

// Managed reports whether exe lives in a package-manager-owned location,
// which the self-updater must never overwrite.
func Managed(exe string) bool {
	p := filepath.ToSlash(exe)
	for _, pre := range managedPrefixes {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	low := strings.ToLower(exe)
	for _, s := range managedWindows {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

// Validator runs a candidate executable and checks it reports want.
type Validator func(exe, want string) error

// RunVersion is the default validator: `exe --version` must exit 0 within
// 10 s and print the expected version.
func RunVersion(exe, want string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "--version").Output()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if !strings.Contains(string(out), want) {
		return fmt.Errorf("%w: reported %q", ErrInvalid, strings.TrimSpace(string(out)))
	}
	return nil
}

// Backup is the previous executable kept for rollback.
func Backup(exe string) string { return exe + ".old" }

// Apply replaces exe with the staged binary:
//  1. the staged file is re-hashed and copied next to exe (same filesystem);
//  2. the copy must pass the validator before anything is replaced;
//  3. exe is renamed to exe.old (allowed for a running file on Linux, macOS
//     and Windows) and the copy is renamed into place;
//  4. the result is validated again, and exe.old restored on failure.
//
// The previous version stays as exe.old until Cleanup after a good start.
func Apply(st Staged, exe string, validate Validator) error {
	if Managed(exe) {
		return ErrManaged
	}
	if sum, err := fileSHA256(st.Binary); err != nil || sum != st.BinarySHA256 {
		return fmt.Errorf("staged update is damaged: %w", ErrChecksum)
	}
	if st.GOOS != runtime.GOOS || st.GOARCH != runtime.GOARCH {
		return fmt.Errorf("staged update is for %s/%s", st.GOOS, st.GOARCH)
	}
	dir := filepath.Dir(exe)
	candidate := exe + ".new"
	if err := copyFile(st.Binary, candidate); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return ErrReadOnly
		}
		return err
	}
	if err := validate(candidate, st.Version); err != nil {
		_ = os.Remove(candidate)
		return err
	}
	old := Backup(exe)
	_ = os.Remove(old) // a previous backup; on Windows it may still be locked
	if _, err := os.Stat(old); err == nil {
		old = fmt.Sprintf("%s.%d.old", exe, time.Now().Unix())
	}
	if err := os.Rename(exe, old); err != nil {
		_ = os.Remove(candidate)
		if errors.Is(err, os.ErrPermission) {
			return ErrReadOnly
		}
		return err
	}
	if err := os.Rename(candidate, exe); err != nil {
		_ = os.Rename(old, exe)
		_ = os.Remove(candidate)
		return err
	}
	if err := validate(exe, st.Version); err != nil {
		_ = os.Remove(exe)
		_ = os.Rename(old, exe)
		return err
	}
	if old != Backup(exe) {
		_ = os.Rename(old, Backup(exe))
	}
	_ = syncDir(dir)
	return nil
}

// Rollback restores the previous executable kept by Apply.
func Rollback(exe string) error {
	old := Backup(exe)
	if _, err := os.Stat(old); err != nil {
		return errors.New("no previous version is available")
	}
	broken := exe + ".failed"
	_ = os.Remove(broken)
	if err := os.Rename(exe, broken); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(old, exe); err != nil {
		_ = os.Rename(broken, exe)
		return err
	}
	_ = os.Remove(broken)
	return nil
}

// Cleanup removes leftovers once the running version has started
// successfully (a running process proves the new executable works).
func Cleanup(exe string) {
	for _, p := range []string{exe + ".new", exe + ".failed"} {
		_ = os.Remove(p)
	}
	matches, _ := filepath.Glob(exe + ".*.old")
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
