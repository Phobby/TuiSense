package installtest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// winHome runs install.ps1 against the local release server with every
// Windows profile directory redirected to a temporary folder, so the test
// never touches the real user's settings. The install path contains spaces
// and non-ASCII characters on purpose.
type winHome struct {
	dir, installDir string
	env             []string
}

func (r *release) winHome(t *testing.T, keys bool) winHome {
	t.Helper()
	h := winHome{dir: t.TempDir()}
	h.installDir = filepath.Join(h.dir, "Programs", "TuiSense Ğüş Test", "bin")
	h.env = append(os.Environ(),
		"APPDATA="+filepath.Join(h.dir, "Roaming"), "LOCALAPPDATA="+filepath.Join(h.dir, "Local"), "USERPROFILE="+h.dir,
		"TUISENSE_API_URL="+r.srv.URL, "TUISENSE_DOWNLOAD_URL="+r.srv.URL+"/download", "TUISENSE_INSTALL_DIR="+h.installDir)
	if keys {
		h.env = append(h.env, "TUISENSE_TRUSTED_KEYS_FILE="+r.pubKeys)
	}
	return h
}

func (r *release) installPS(t *testing.T, h winHome, args ...string) (string, error) {
	t.Helper()
	// Never edit the developer's real user PATH; CI runners are disposable.
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		args = append(args, "-NoModifyPath")
	}
	base := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(r.root, "scripts", "install.ps1")}
	return run(t, h.dir, h.env, "powershell.exe", append(base, args...)...)
}

func newWindowsRelease(t *testing.T) *release {
	t.Helper()
	if testing.Short() || runtime.GOOS != "windows" {
		t.Skip("end-to-end PowerShell installer test (Windows, not -short)")
	}
	return newReleaseFor(t, "powershell.exe", "ssh-keygen")
}

func TestPowerShellInstallReinstallAndUninstall(t *testing.T) {
	r := newWindowsRelease(t)
	h := r.winHome(t, true)
	profile := filepath.Join(h.dir, "Roaming", "tuisense", "profiles", "mine.json")
	_ = os.MkdirAll(filepath.Dir(profile), 0o755)
	_ = os.WriteFile(profile, []byte(`{"name":"mine"}`), 0o600)

	out, err := r.installPS(t, h)
	if err != nil || !strings.Contains(out, "Signature and checksum verified") {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	exe := filepath.Join(h.installDir, "tuisense.exe")
	if v, err := run(t, h.dir, h.env, exe, "--version"); err != nil || !strings.Contains(v, "TuiSense v"+testVersion) || !strings.Contains(v, "windows/"+runtime.GOARCH) {
		t.Fatalf("installed executable: %v %s", err, v)
	}
	// The diagnostics a user is asked to share run without administrator rights.
	if v, err := run(t, h.dir, h.env, exe, "doctor", "--windows"); err != nil || !strings.Contains(v, "Controller details") {
		t.Fatalf("doctor --windows: %v\n%s", err, v)
	}
	if out, err := r.installPS(t, h, "-Version", testVersion); err != nil {
		t.Fatalf("reinstall: %v\n%s", err, out)
	}
	if _, err := os.Stat(exe + ".old"); err != nil {
		t.Fatal("the previous executable is kept for recovery")
	}
	if out, err := r.installPS(t, h, "-Uninstall"); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}
	if _, err := os.Stat(exe); err == nil {
		t.Fatal("executable not removed")
	}
	if b, _ := os.ReadFile(profile); string(b) != `{"name":"mine"}` {
		t.Fatal("uninstalling must keep profiles")
	}
	// Reinstalling after an uninstall finds the profiles again.
	if out, err := r.installPS(t, h); err != nil {
		t.Fatalf("install after uninstall: %v\n%s", err, out)
	}
	if out, err := r.installPS(t, h, "-Uninstall", "-Purge"); err != nil {
		t.Fatalf("purge: %v\n%s", err, out)
	}
	if _, err := os.Stat(profile); err == nil {
		t.Fatal("-Purge removes settings")
	}
}

func TestPowerShellInstallerRefusesBadReleases(t *testing.T) {
	r := newWindowsRelease(t)
	exe := func(h winHome) string { return filepath.Join(h.installDir, "tuisense.exe") }

	// Signed by a key that is not pinned.
	h := r.winHome(t, false)
	if out, err := r.installPS(t, h); err == nil || !strings.Contains(out, "signature could not be verified") {
		t.Fatalf("expected a signature refusal: %v\n%s", err, out)
	}
	// A version that was never published (missing artifact).
	h = r.winHome(t, true)
	if out, err := r.installPS(t, h, "-Version", "9.9.0"); err == nil || !strings.Contains(out, "could not download") {
		t.Fatalf("expected a download refusal: %v\n%s", err, out)
	}
	// A corrupt archive fails its signed checksum.
	h = r.winHome(t, true)
	matches, _ := filepath.Glob(filepath.Join(r.dist, "tuisense_*.zip"))
	if len(matches) != 1 {
		t.Fatalf("expected one zip, got %v", matches)
	}
	data, _ := os.ReadFile(matches[0])
	data[len(data)/2] ^= 0xff
	_ = os.WriteFile(matches[0], data, 0o644)
	if out, err := r.installPS(t, h); err == nil || !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("expected a checksum refusal: %v\n%s", err, out)
	}
	if _, err := os.Stat(exe(h)); err == nil {
		t.Fatal("nothing may be installed after a failed verification")
	}
}
