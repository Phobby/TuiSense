package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSemverOrdering(t *testing.T) {
	order := []string{"0.9.0-alpha", "0.9.0-alpha.1", "0.9.0-alpha.beta", "0.9.0-beta.2", "0.9.0-beta.11", "0.9.0-rc.1", "0.9.0", "0.9.1", "0.10.0", "1.0.0"}
	for i := 1; i < len(order); i++ {
		a, _ := ParseSemver(order[i-1])
		b, err := ParseSemver(order[i])
		if err != nil {
			t.Fatal(err)
		}
		if a.Compare(b) >= 0 || b.Compare(a) <= 0 {
			t.Errorf("%s must sort before %s", a, b)
		}
	}
	for _, bad := range []string{"", "1.2", "1.2.3.4", "01.2.3", "1.2.x", "v1.2.3-"} {
		if _, err := ParseSemver(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
	if v, _ := ParseSemver("v1.2.3+build.5"); v.String() != "1.2.3" {
		t.Fatal("build metadata is ignored")
	}
}

func newKey(t *testing.T) (ed25519.PrivateKey, []TrustedKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ParseAuthorizedKey(AuthorizedKey(pub, "test-release"))
	if err != nil {
		t.Fatal(err)
	}
	return priv, []TrustedKey{k}
}

func TestSSHSigRoundTripAndRejection(t *testing.T) {
	priv, keys := newKey(t)
	msg := []byte("abc  tuisense_1.0.0_linux_amd64.tar.gz\n")
	sig := SignSSHSig(msg, priv)
	if _, err := VerifySSHSig(msg, sig, keys); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySSHSig(append(msg, 'x'), sig, keys); !errors.Is(err, ErrUntrustedSignature) {
		t.Fatalf("modified metadata must fail: %v", err)
	}
	_, other := newKey(t)
	if _, err := VerifySSHSig(msg, sig, other); !errors.Is(err, ErrUntrustedSignature) {
		t.Fatalf("another key must fail: %v", err)
	}
	if _, err := VerifySSHSig(msg, sig, nil); !errors.Is(err, ErrNoTrustedKey) {
		t.Fatal("no pinned key means no verification")
	}
	if _, err := VerifySSHSig(msg, []byte("garbage"), keys); err == nil {
		t.Fatal("garbage signatures must fail")
	}
}

// The Go verifier and OpenSSH's ssh-keygen -Y (used by the installers and
// the release workflow) must agree in both directions.
func TestSSHSigInteroperatesWithSSHKeygen(t *testing.T) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not installed")
	}
	dir := t.TempDir()
	key := filepath.Join(dir, "release")
	if out, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-C", "release", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	msg := filepath.Join(dir, SumsName)
	if err := os.WriteFile(msg, []byte("deadbeef  file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(keygen, "-Y", "sign", "-f", key, "-n", SignatureNamespace, msg).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	pubLine, _ := os.ReadFile(key + ".pub")
	keys, err := ParseTrustedKeys(string(pubLine))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(msg)
	sig, _ := os.ReadFile(msg + ".sig")
	if _, err := VerifySSHSig(data, sig, keys); err != nil {
		t.Fatalf("Go must verify ssh-keygen signatures: %v", err)
	}
	// And ssh-keygen verifies signatures made by the release tool.
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := os.WriteFile(msg+".sig", SignSSHSig(data, priv), 0o644); err != nil {
		t.Fatal(err)
	}
	signers := filepath.Join(dir, "allowed_signers")
	line := "release " + AuthorizedKey(priv.Public().(ed25519.PublicKey), "")
	if err := os.WriteFile(signers, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(keygen, "-Y", "verify", "-f", signers, "-I", "release", "-n", SignatureNamespace, "-s", msg+".sig")
	cmd.Stdin = bytes.NewReader(data)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen must verify Go signatures: %v %s", err, out)
	}
}

// fakeRelease is a local release server with signed metadata.
type fakeRelease struct {
	srv      *httptest.Server
	priv     ed25519.PrivateKey
	keys     []TrustedKey
	mu       sync.Mutex
	files    map[string][]byte
	releases []map[string]any
	hits     map[string]int
	truncate string // file served truncated
}

func archiveFor(t *testing.T, goos, payload string) []byte {
	t.Helper()
	var b bytes.Buffer
	name := BinaryName(goos)
	if goos == "windows" {
		zw := zip.NewWriter(&b)
		w, _ := zw.Create("tuisense/" + name)
		_, _ = w.Write([]byte(payload))
		_ = zw.Close()
		return b.Bytes()
	}
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, f := range []struct{ name, body string }{{"README.md", "readme"}, {"tuisense/" + name, payload}} {
		_ = tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(f.body))
	}
	_ = tw.Close()
	_ = gz.Close()
	return b.Bytes()
}

func newFakeRelease(t *testing.T) *fakeRelease {
	t.Helper()
	f := &fakeRelease{files: map[string][]byte{}, hits: map[string]int{}}
	f.priv, f.keys = newKey(t)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.hits[r.URL.Path]++
		if r.URL.Path == "/repos/Phobby/TuiSense/releases" {
			_ = json.NewEncoder(w).Encode(f.releases)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/download/")
		body, ok := f.files[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(name, f.truncate) && f.truncate != "" {
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body[:len(body)/2])
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
			}
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// publish adds a signed release with packages for the given platforms.
func (f *fakeRelease) publish(t *testing.T, tag string, pre bool, platforms ...string) {
	t.Helper()
	v, err := ParseSemver(tag)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var sums []string
	var assets []map[string]any
	for _, p := range platforms {
		goos, goarch, _ := strings.Cut(p, "/")
		name := ArchiveName(v, goos, goarch)
		body := archiveFor(t, goos, "#!/bin/sh\necho \"TuiSense v"+v.String()+"\"\n")
		f.files[tag+"/"+name] = body
		h := sha256.Sum256(body)
		sums = append(sums, hex.EncodeToString(h[:])+"  "+name)
		assets = append(assets, map[string]any{"name": name, "browser_download_url": f.srv.URL + "/download/" + tag + "/" + name, "size": len(body)})
	}
	sort.Strings(sums)
	sumsData := []byte(strings.Join(sums, "\n") + "\n")
	f.files[tag+"/"+SumsName] = sumsData
	f.files[tag+"/"+SigName] = SignSSHSig(sumsData, f.priv)
	for _, n := range []string{SumsName, SigName} {
		assets = append(assets, map[string]any{"name": n, "browser_download_url": f.srv.URL + "/download/" + tag + "/" + n})
	}
	f.releases = append(f.releases, map[string]any{"tag_name": tag, "prerelease": pre, "body": "Release " + tag + "\n\n- change", "assets": assets})
}

func (f *fakeRelease) source() Source {
	return Source{API: f.srv.URL, AllowHTTP: true, Hosts: []string{"127.0.0.1"}}
}

func TestLatestRespectsChannelsAndDrafts(t *testing.T) {
	f := newFakeRelease(t)
	f.publish(t, "v0.8.0", false, "linux/amd64")
	f.publish(t, "v0.9.0-rc.1", true, "linux/amd64")
	f.releases = append(f.releases, map[string]any{"tag_name": "v1.0.0", "draft": true})
	f.releases = append(f.releases, map[string]any{"tag_name": "not-a-version"})
	stable, err := f.source().Latest(context.Background(), Stable)
	if err != nil || stable.Version.String() != "0.8.0" {
		t.Fatalf("stable channel: %v %v", stable.Version, err)
	}
	pre, err := f.source().Latest(context.Background(), Prerelease)
	if err != nil || pre.Version.String() != "0.9.0-rc.1" {
		t.Fatalf("pre-release channel: %v %v", pre.Version, err)
	}
}

func TestInvalidMetadataAndNetworkFailures(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{not json")) }))
	defer bad.Close()
	if _, err := (Source{API: bad.URL, AllowHTTP: true, Hosts: []string{"127.0.0.1"}}).Latest(context.Background(), Stable); err == nil {
		t.Fatal("invalid metadata must fail")
	}
	missing := httptest.NewServer(http.NotFoundHandler())
	defer missing.Close()
	if _, err := (Source{API: missing.URL, AllowHTTP: true, Hosts: []string{"127.0.0.1"}}).Latest(context.Background(), Stable); err == nil || !strings.Contains(err.Error(), "no published releases") {
		t.Fatalf("a repository without releases must say so, got %v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	if _, err := (Source{API: url, AllowHTTP: true, Hosts: []string{"127.0.0.1"}}).Latest(context.Background(), Stable); err == nil {
		t.Fatal("unreachable servers must fail")
	}
	// Plain HTTP and foreign hosts are refused outright.
	if _, err := (Source{API: "http://api.github.com"}).Latest(context.Background(), Stable); !errors.Is(err, ErrUntrustedHost) {
		t.Fatalf("plain HTTP must be refused: %v", err)
	}
	if err := (Source{}).checkURL("https://evil.example/tuisense.tar.gz"); !errors.Is(err, ErrUntrustedHost) {
		t.Fatal("foreign hosts must be refused")
	}
}

func stage(t *testing.T, f *fakeRelease, keys []TrustedKey, goos, goarch string) (Staged, string, error) {
	t.Helper()
	rel, err := f.source().Latest(context.Background(), Stable)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	st, err := f.source().Stage(context.Background(), rel, dir, goos, goarch, keys, nil)
	return st, dir, err
}

func TestStageVerifiesSignatureChecksumAndPlatform(t *testing.T) {
	f := newFakeRelease(t)
	f.publish(t, "v0.9.0", false, "linux/amd64", "darwin/arm64", "windows/amd64")
	for _, p := range []string{"linux/amd64", "darwin/arm64", "windows/amd64"} {
		goos, goarch, _ := strings.Cut(p, "/")
		st, dir, err := stage(t, f, f.keys, goos, goarch)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if got, ok := LoadStaged(dir); !ok || got.Version != "0.9.0" || filepath.Base(got.Binary) != BinaryName(goos) || st.Signer != "test-release" {
			t.Fatalf("%s: staged %+v", p, got)
		}
	}
	if _, dir, err := stage(t, f, f.keys, "linux", "riscv64"); !errors.Is(err, ErrNoAsset) || hasStaged(dir) {
		t.Fatalf("wrong architecture must be refused: %v", err)
	}
	_, other := newKey(t)
	if _, dir, err := stage(t, f, other, "linux", "amd64"); !errors.Is(err, ErrUntrustedSignature) || hasStaged(dir) {
		t.Fatalf("an unknown signer must be refused: %v", err)
	}
	if _, _, err := stage(t, f, nil, "linux", "amd64"); !errors.Is(err, ErrNoTrustedKey) {
		t.Fatalf("builds without a pinned key never stage: %v", err)
	}
	// A package swapped after signing fails its checksum and leaves nothing behind.
	f.mu.Lock()
	f.files["v0.9.0/"+ArchiveName(Semver{0, 9, 0, ""}, "linux", "amd64")] = archiveFor(t, "linux", "#!/bin/sh\necho evil\n")
	f.mu.Unlock()
	if _, dir, err := stage(t, f, f.keys, "linux", "amd64"); !errors.Is(err, ErrChecksum) || hasStaged(dir) {
		t.Fatalf("tampered package must be refused: %v", err)
	}
}

func TestInterruptedDownloadLeavesNothingStaged(t *testing.T) {
	f := newFakeRelease(t)
	f.publish(t, "v0.9.0", false, "linux/amd64")
	f.truncate = ".tar.gz"
	if _, dir, err := stage(t, f, f.keys, "linux", "amd64"); err == nil || hasStaged(dir) {
		t.Fatalf("an interrupted download must fail cleanly: %v", err)
	}
}

func hasStaged(dir string) bool {
	_, ok := LoadStaged(dir)
	entries, _ := os.ReadDir(dir)
	return ok || len(entries) > 0
}

// fakeExe writes an executable that prints a version, like tuisense --version.
func fakeExe(t *testing.T, dir, version string) string {
	t.Helper()
	exe := filepath.Join(dir, "tuisense")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho \"TuiSense v"+version+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestApplyReplacesAtomicallyAndKeepsBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell-script executables")
	}
	f := newFakeRelease(t)
	f.publish(t, "v0.9.0", false, runtime.GOOS+"/"+runtime.GOARCH)
	st, _, err := stage(t, f, f.keys, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	install := filepath.Join(home, "bin")
	_ = os.MkdirAll(install, 0o755)
	exe := fakeExe(t, install, "0.8.0")
	// User data lives elsewhere and must be untouched.
	profile := filepath.Join(home, "config", "profiles", "mine.json")
	_ = os.MkdirAll(filepath.Dir(profile), 0o755)
	_ = os.WriteFile(profile, []byte(`{"name":"mine"}`), 0o600)

	if err := Apply(st, exe, RunVersion); err != nil {
		t.Fatal(err)
	}
	if out, _ := exec.Command(exe, "--version").Output(); !strings.Contains(string(out), "0.9.0") {
		t.Fatalf("new version not installed: %s", out)
	}
	if out, _ := exec.Command(Backup(exe), "--version").Output(); !strings.Contains(string(out), "0.8.0") {
		t.Fatal("the previous version must be kept for recovery")
	}
	if b, _ := os.ReadFile(profile); string(b) != `{"name":"mine"}` {
		t.Fatal("profiles must be preserved")
	}
	if err := Rollback(exe); err != nil {
		t.Fatal(err)
	}
	if out, _ := exec.Command(exe, "--version").Output(); !strings.Contains(string(out), "0.8.0") {
		t.Fatal("rollback must restore the previous version")
	}
}

func TestFailedValidationNeverReplacesTheInstallation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell-script executables")
	}
	f := newFakeRelease(t)
	f.publish(t, "v0.9.0", false, runtime.GOOS+"/"+runtime.GOARCH)
	st, _, err := stage(t, f, f.keys, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	exe := fakeExe(t, t.TempDir(), "0.8.0")
	broken := func(string, string) error { return ErrInvalid }
	if err := Apply(st, exe, broken); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected a validation failure, got %v", err)
	}
	if out, _ := exec.Command(exe, "--version").Output(); !strings.Contains(string(out), "0.8.0") {
		t.Fatal("a failed update must leave the installed version working")
	}
	for _, p := range []string{exe + ".new", Backup(exe)} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s left behind", p)
		}
	}
	// A validator that fails only after the swap triggers an automatic rollback.
	calls := 0
	late := func(string, string) error {
		calls++
		if calls == 2 {
			return ErrInvalid
		}
		return nil
	}
	if err := Apply(st, exe, late); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if out, _ := exec.Command(exe, "--version").Output(); !strings.Contains(string(out), "0.8.0") {
		t.Fatal("a post-swap failure must roll back")
	}
}

func TestManagedAndReadOnlyInstallationsAreNotOverwritten(t *testing.T) {
	for _, p := range []string{"/usr/bin/tuisense", "/opt/homebrew/bin/tuisense", "/nix/store/abc-tuisense/bin/tuisense", `C:\Program Files\WindowsApps\TuiSense\tuisense.exe`, `C:\Users\me\scoop\apps\tuisense\current\tuisense.exe`} {
		if !Managed(p) {
			t.Errorf("%s is package-manager owned", p)
		}
	}
	for _, p := range []string{"/home/me/.local/bin/tuisense", `C:\Users\me\AppData\Local\Programs\TuiSense\bin\tuisense.exe`} {
		if Managed(p) {
			t.Errorf("%s is a user install", p)
		}
	}
	if err := Apply(Staged{}, "/usr/bin/tuisense", RunVersion); !errors.Is(err, ErrManaged) {
		t.Fatalf("managed installs are never replaced: %v", err)
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	f := newFakeRelease(t)
	f.publish(t, "v0.9.0", false, runtime.GOOS+"/"+runtime.GOARCH)
	st, _, err := stage(t, f, f.keys, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	exe := fakeExe(t, dir, "0.8.0")
	_ = os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)
	if err := Apply(st, exe, RunVersion); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("read-only installs must report it: %v", err)
	}
}

func waitFor(t *testing.T, m *Manager, want ...Status) Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s := m.Snapshot()
		m.mu.Lock()
		busy := m.busy
		m.mu.Unlock()
		for _, w := range want {
			if s.Status == w && !busy {
				return s
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("status %s, want %v", m.Snapshot().Status, want)
	return Snapshot{}
}

func newManager(t *testing.T, f *fakeRelease, current string, s Settings) *Manager {
	t.Helper()
	m := NewManager(current, s, time.Time{}, t.TempDir(), filepath.Join(t.TempDir(), "tuisense"))
	m.Source, m.Keys = f.source(), f.keys
	m.snap.CanStage = m.canStage()
	return m
}

func TestManagerCheckDownloadAndApplyOnRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell-script executables")
	}
	f := newFakeRelease(t)
	f.publish(t, "v0.9.0", false, runtime.GOOS+"/"+runtime.GOARCH)

	// Automatic downloads off: the update is announced, not downloaded.
	m := newManager(t, f, "0.8.0", Settings{Check: true, Channel: Stable})
	if !m.CheckAsync(false) {
		t.Fatal("first automatic check must run")
	}
	s := waitFor(t, m, StatusAvailable)
	if s.Latest != "0.9.0" || s.Ready != nil {
		t.Fatalf("expected an available, not staged, update: %+v", s)
	}
	if m.CheckAsync(false) {
		t.Fatal("automatic checks are throttled")
	}
	if !m.DownloadAsync() {
		t.Fatal("a manual download must start")
	}
	s = waitFor(t, m, StatusReady)
	if s.Ready == nil || s.Ready.Version != "0.9.0" {
		t.Fatalf("expected a staged update: %+v", s)
	}

	// Automatic downloads on: check and stage in one go.
	m2 := newManager(t, f, "0.8.0", Settings{Check: true, AutoDownload: true, Channel: Stable})
	m2.CheckAsync(false)
	s = waitFor(t, m2, StatusReady)

	// Restart: the staged update is applied and the backup kept.
	exe := fakeExe(t, t.TempDir(), "0.8.0")
	m2.Exe = exe
	installed, err := m2.ApplyPending(RunVersion)
	if err != nil || installed != "0.9.0" {
		t.Fatalf("apply on restart: %q %v", installed, err)
	}
	if _, ok := LoadStaged(m2.Dir); ok {
		t.Fatal("the staged update is consumed")
	}
	// A new manager on the updated version sees nothing pending.
	again := NewManager("0.9.0", s.Settings, time.Now(), m2.Dir, exe)
	if again.Snapshot().Status == StatusReady {
		t.Fatal("an installed update must not be offered again")
	}
}

func TestManagerOfflineDisabledAndDevBuilds(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL
	closed.Close()
	m := NewManager("0.8.0", Settings{Check: true}, time.Time{}, t.TempDir(), "/home/u/.local/bin/tuisense")
	m.Source = Source{API: url, AllowHTTP: true, Hosts: []string{"127.0.0.1"}}
	m.CheckAsync(false)
	if s := waitFor(t, m, StatusOffline); s.Err == "" {
		t.Fatal("offline checks report quietly with a reason")
	}
	f := newFakeRelease(t)
	f.publish(t, "v0.9.0", false, "linux/amd64")
	off := newManager(t, f, "0.8.0", Settings{Check: false})
	if off.Snapshot().Status != StatusDisabled || off.CheckAsync(false) {
		t.Fatal("disabled checks must not contact the server")
	}
	if !off.CheckAsync(true) {
		t.Fatal("a manual check still works when automatic checks are off")
	}
	waitFor(t, off, StatusAvailable)
	dev := newManager(t, f, "dev", Settings{Check: true})
	if dev.Snapshot().Status != StatusDev || dev.CheckAsync(true) {
		t.Fatal("development builds have no version to compare")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hits["/repos/Phobby/TuiSense/releases"] != 1 {
		t.Fatalf("expected exactly one request (the manual check), got %d", f.hits["/repos/Phobby/TuiSense/releases"])
	}
}

func TestPrereleaseNeedsTheChannel(t *testing.T) {
	f := newFakeRelease(t)
	f.publish(t, "v0.9.0-rc.1", true, "linux/amd64")
	m := newManager(t, f, "0.8.0", Settings{Check: true, Channel: Stable})
	m.CheckAsync(true)
	if s := waitFor(t, m, StatusOffline, StatusUpToDate); s.Status == StatusAvailable {
		t.Fatal("a pre-release is never offered on the stable channel")
	}
	m = newManager(t, f, "0.8.0", Settings{Check: true, Channel: Prerelease})
	m.CheckAsync(true)
	if s := waitFor(t, m, StatusAvailable); s.Latest != "0.9.0-rc.1" {
		t.Fatalf("pre-release channel: %+v", s)
	}
}

func TestPinnedKeysParse(t *testing.T) {
	if _, err := ParseTrustedKeys(trustedKeysText); err != nil {
		t.Fatalf("trusted_keys.txt must parse: %v", err)
	}
}

// A start-up shortly after the last check shows that check's result. A
// different running version (just updated), a recorded newer release, or no
// record makes the check run at once instead of waiting for Interval.
func TestStartupRestoresOrRechecks(t *testing.T) {
	f := newFakeRelease(t)
	f.publish(t, "v0.8.1", false, runtime.GOOS+"/"+runtime.GOARCH)
	recent := time.Now().Add(-5 * time.Minute)
	start := func(current, latest string) *Manager {
		m := NewManager(current, Settings{Check: true}, recent, t.TempDir(), filepath.Join(t.TempDir(), "tuisense"))
		m.Source, m.Keys = f.source(), f.keys
		m.RestoreLatest(latest)
		return m
	}

	m := start("0.8.1", "0.8.1")
	if s := m.Snapshot(); s.Status != StatusUpToDate || s.Latest != "0.8.1" {
		t.Fatalf("the last result must be shown: %+v", s)
	}
	if m.CheckAsync(false) {
		t.Fatal("a recent check of the same version is not repeated")
	}

	for _, tc := range []struct{ current, latest string }{
		{"0.8.1", "0.8.0"}, // updated since the last check
		{"0.8.0", "0.8.0"}, // the recorded check predates the 0.8.1 release: still throttled…
		{"0.8.0", "0.8.1"}, // …but a recorded newer release is announced at once
		{"0.8.0", ""},      // no record (older settings file)
	} {
		m := start(tc.current, tc.latest)
		ran := m.CheckAsync(false)
		if want := tc.current != tc.latest; ran != want {
			t.Fatalf("current %s, recorded %q: check ran %v, want %v", tc.current, tc.latest, ran, want)
		}
		if ran {
			want := StatusUpToDate
			if tc.current == "0.8.0" {
				want = StatusAvailable
			}
			waitFor(t, m, want)
		}
	}
	if NewManager("0.8.0", Settings{Check: true}, time.Time{}, t.TempDir(), "x").Interval > time.Hour {
		t.Fatal("automatic checks run at least hourly while TuiSense is started")
	}
}
