package updatecmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/update"
)

func server(t *testing.T, body string) update.Source {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	t.Cleanup(srv.Close)
	return update.Source{API: srv.URL, AllowHTTP: true, Hosts: []string{"127.0.0.1"}}
}

func run(t *testing.T, c Config, args ...string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := RunWith(args, &out, c)
	return code, out.String()
}

func TestCheckAndRefusals(t *testing.T) {
	src := server(t, `[{"tag_name":"v0.9.0","assets":[]}]`)
	c := Config{Current: "0.8.0", Source: src, Dir: t.TempDir(), Exe: filepath.Join(t.TempDir(), "tuisense"), GOOS: "linux", GOARCH: "amd64"}
	if code, out := run(t, c, "--check"); code != 0 || !strings.Contains(out, "v0.9.0") {
		t.Fatalf("check: %d %s", code, out)
	}
	// Without a pinned key nothing is downloaded or installed.
	if code, out := run(t, c); code != 1 || !strings.Contains(out, "not installed") {
		t.Fatalf("unverifiable updates must be refused: %d %s", code, out)
	}
	c.Exe = "/usr/bin/tuisense"
	if code, out := run(t, c); code != 1 || !strings.Contains(out, "package manager") {
		t.Fatalf("managed installs are not overwritten: %d %s", code, out)
	}
	c.Current = "dev"
	if code, out := run(t, c); code != 1 || !strings.Contains(out, "development build") {
		t.Fatalf("dev builds: %d %s", code, out)
	}
	c.Current = "0.9.0"
	if code, out := run(t, c); code != 0 || !strings.Contains(out, "up to date") {
		t.Fatalf("up to date: %d %s", code, out)
	}
}

func TestRollbackRestoresThePreviousExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tuisense")
	_ = os.WriteFile(exe, []byte("new"), 0o755)
	_ = os.WriteFile(update.Backup(exe), []byte("old"), 0o755)
	if code, _ := run(t, Config{Exe: exe}, "--rollback"); code != 0 {
		t.Fatal("rollback failed")
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Fatal("previous version not restored")
	}
	if code, _ := run(t, Config{Exe: exe}, "--rollback"); code != 1 {
		t.Fatal("a second rollback has nothing to restore")
	}
}
