package installtest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

const testVersion = "9.9.1"

type release struct {
	root    string // repository root
	dist    string // served files
	pubKeys string // trusted keys file
	srv     *httptest.Server
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func run(t *testing.T, dir string, env []string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// newRelease builds the real tuisense binary, packages it with
// tools/release, signs SHA256SUMS with ssh-keygen and serves it.
func newRelease(t *testing.T) *release {
	t.Helper()
	if testing.Short() || runtime.GOOS != "linux" {
		t.Skip("end-to-end installer test (Linux, not -short)")
	}
	for _, tool := range []string{"sh", "ssh-keygen", "tar", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	r := &release{root: repoRoot(t), dist: t.TempDir()}
	env := os.Environ()
	bin := filepath.Join(t.TempDir(), "tuisense")
	if out, err := run(t, r.root, append(env, "CGO_ENABLED=0"), "go", "build", "-trimpath",
		"-ldflags", "-X github.com/Phobby/TuiSense/internal/version.Version="+testVersion, "-o", bin, "./cmd/tuisense"); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	if out, err := run(t, r.root, env, "go", "run", "./tools/release", "package", "-version", testVersion,
		"-os", "linux", "-arch", runtime.GOARCH, "-bin", bin, "-out", r.dist, "-files", "README.md"); err != nil {
		t.Fatalf("package: %v\n%s", err, out)
	}
	if out, err := run(t, r.root, env, "go", "run", "./tools/release", "sums", "-out", r.dist); err != nil {
		t.Fatalf("sums: %v\n%s", err, out)
	}
	keyDir := t.TempDir()
	key := filepath.Join(keyDir, "release")
	if out, err := run(t, keyDir, env, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "test-release", "-f", key); err != nil {
		t.Fatalf("keygen: %v\n%s", err, out)
	}
	if out, err := run(t, keyDir, env, "ssh-keygen", "-Y", "sign", "-f", key, "-n", "tuisense-release", filepath.Join(r.dist, "SHA256SUMS")); err != nil {
		t.Fatalf("sign: %v\n%s", err, out)
	}
	r.pubKeys = key + ".pub"
	// The Go verifier used by the updater accepts the same release.
	if out, err := run(t, r.root, env, "go", "run", "./tools/release", "verify", "-out", r.dist, "-keys", r.pubKeys); err != nil {
		t.Fatalf("verify: %v\n%s", err, out)
	}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/repos/Phobby/TuiSense/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name": "v` + testVersion + `", "prerelease": false}`))
		case strings.HasPrefix(req.URL.Path, "/download/v"+testVersion+"/"):
			http.ServeFile(w, req, filepath.Join(r.dist, filepath.Base(req.URL.Path)))
		default:
			http.NotFound(w, req)
		}
	}))
	t.Cleanup(r.srv.Close)
	return r
}

type home struct {
	dir string
	env []string
}

func (r *release) home(t *testing.T, keys bool) home {
	t.Helper()
	h := home{dir: t.TempDir()}
	h.env = []string{"HOME=" + h.dir, "SHELL=/bin/bash", "PATH=/usr/bin:/bin",
		"XDG_CONFIG_HOME=" + filepath.Join(h.dir, ".config"), "XDG_DATA_HOME=" + filepath.Join(h.dir, ".local", "share"),
		"TUISENSE_API_URL=" + r.srv.URL, "TUISENSE_DOWNLOAD_URL=" + r.srv.URL + "/download"}
	if keys {
		h.env = append(h.env, "TUISENSE_TRUSTED_KEYS_FILE="+r.pubKeys)
	}
	return h
}

func (r *release) install(t *testing.T, h home, args ...string) (string, error) {
	t.Helper()
	return run(t, h.dir, h.env, "sh", append([]string{filepath.Join(r.root, "scripts", "install.sh")}, args...)...)
}

func TestInstallReinstallAndUninstall(t *testing.T) {
	r := newRelease(t)
	h := r.home(t, true)
	profile := filepath.Join(h.dir, ".config", "tuisense", "profiles", "mine.json")
	_ = os.MkdirAll(filepath.Dir(profile), 0o755)
	_ = os.WriteFile(profile, []byte(`{"name":"mine"}`), 0o600)

	out, err := r.install(t, h)
	if err != nil || !strings.Contains(out, "Signature and checksum verified") {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	bin := filepath.Join(h.dir, ".local", "bin", "tuisense")
	if v, err := run(t, h.dir, h.env, bin, "--version"); err != nil || !strings.Contains(v, "TuiSense v"+testVersion) {
		t.Fatalf("installed binary: %v %s", err, v)
	}
	if v, _ := run(t, h.dir, h.env, bin, "--help"); !strings.Contains(v, "tuisense update") {
		t.Fatalf("--help must list the update command: %s", v)
	}
	// Reinstalling is safe: PATH is configured once and the old binary kept.
	if out, err := r.install(t, h, "--version", testVersion); err != nil {
		t.Fatalf("reinstall: %v\n%s", err, out)
	}
	rc, _ := os.ReadFile(filepath.Join(h.dir, ".bashrc"))
	if strings.Count(string(rc), "# tuisense path") != 1 {
		t.Fatalf("PATH must be added exactly once:\n%s", rc)
	}
	if _, err := os.Stat(bin + ".old"); err != nil {
		t.Fatal("the previous binary is kept for recovery")
	}
	if b, _ := os.ReadFile(profile); string(b) != `{"name":"mine"}` {
		t.Fatal("profiles must survive installation")
	}
	// Uninstall keeps settings; --purge removes them.
	if out, err := r.install(t, h, "--uninstall"); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}
	if _, err := os.Stat(bin); err == nil {
		t.Fatal("binary not removed")
	}
	if _, err := os.Stat(profile); err != nil {
		t.Fatal("uninstall must keep profiles")
	}
	if _, err := r.install(t, h, "--uninstall", "--purge"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(profile); err == nil {
		t.Fatal("--purge removes settings")
	}
}

func TestInstallerRefusesUnverifiedReleases(t *testing.T) {
	r := newRelease(t)
	bin := func(h home) string { return filepath.Join(h.dir, ".local", "bin", "tuisense") }

	// Only the key pinned in the repository: the test release is signed by
	// another key, so it is refused.
	h := r.home(t, false)
	if out, err := r.install(t, h); err == nil || !strings.Contains(out, "not signed by a trusted") {
		t.Fatalf("expected a refusal for a release not signed by the pinned key: %v\n%s", err, out)
	}
	// An installer without any pinned key refuses before downloading.
	h = r.home(t, false)
	src, err := os.ReadFile(filepath.Join(r.root, "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	unpinned := regexp.MustCompile(`(?m)^TRUSTED_KEYS=".*"$`).ReplaceAll(src, []byte(`TRUSTED_KEYS=""`))
	script := filepath.Join(t.TempDir(), "install.sh")
	_ = os.WriteFile(script, unpinned, 0o755)
	if out, err := run(t, h.dir, h.env, "sh", script); err == nil || !strings.Contains(out, "no pinned release signing key") {
		t.Fatalf("expected a refusal without a pinned key: %v\n%s", err, out)
	}
	// A different signer.
	h = r.home(t, false)
	other := filepath.Join(t.TempDir(), "other")
	_, _ = run(t, h.dir, os.Environ(), "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", other)
	h.env = append(h.env, "TUISENSE_TRUSTED_KEYS_FILE="+other+".pub")
	if out, err := r.install(t, h); err == nil || !strings.Contains(out, "not signed by a trusted") {
		t.Fatalf("expected a signature refusal: %v\n%s", err, out)
	}
	// Wrong architecture: no package, nothing installed.
	h = r.home(t, true)
	h.env = append(h.env, "TUISENSE_ARCH=riscv64")
	if out, err := r.install(t, h); err == nil || !strings.Contains(out, "unsupported CPU architecture") {
		t.Fatalf("expected an architecture refusal: %v\n%s", err, out)
	}
	h = r.home(t, true)
	other64 := "arm64"
	if runtime.GOARCH == "arm64" {
		other64 = "amd64"
	}
	h.env = append(h.env, "TUISENSE_ARCH="+other64)
	if out, err := r.install(t, h); err == nil || !strings.Contains(out, "no release package for linux/"+other64) {
		t.Fatalf("expected a missing-package refusal: %v\n%s", err, out)
	}
	// A tampered archive fails its signed checksum.
	h = r.home(t, true)
	matches, _ := filepath.Glob(filepath.Join(r.dist, "tuisense_*.tar.gz"))
	data, _ := os.ReadFile(matches[0])
	data[len(data)-10] ^= 0xff
	_ = os.WriteFile(matches[0], data, 0o644)
	if out, err := r.install(t, h); err == nil || !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("expected a checksum refusal: %v\n%s", err, out)
	}
	for _, hh := range []home{h} {
		if _, err := os.Stat(bin(hh)); err == nil {
			t.Fatal("nothing may be installed after a failed verification")
		}
	}
}
