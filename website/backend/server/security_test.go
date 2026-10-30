//go:build linux

package server

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func procFile(t *testing.T, pid int, name string) string {
	t.Helper()
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/%s", pid, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func statusField(status, field string) string {
	for _, l := range strings.Split(status, "\n") {
		if k, v, ok := strings.Cut(l, ":"); ok && k == field {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// The session process is the real tuisense binary started without a shell,
// with a fixed argument vector, a scrubbed environment, no privileges, no
// capabilities and resource limits.
func TestSessionProcessIsConfined(t *testing.T) {
	srv, hs := newTestServer(t, func(c *Config) { c.MemoryLimitMB = 900; c.CPUSeconds = 321 })
	c := openTerm(t, hs, map[string]any{"cols": 120, "rows": 40})
	ready := c.waitMsg("ready", startTimeout)
	c.waitText("WEB DEMO", startTimeout)
	sess := sessionOf(t, srv, ready["session"].(string))
	pid := pidOf(sess)

	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil || exe != tuisenseBin {
		t.Fatalf("the session must run the tuisense binary directly, runs %q (%v)", exe, err)
	}
	argv := strings.Split(strings.TrimRight(procFile(t, pid, "cmdline"), "\x00"), "\x00")
	if len(argv) < 2 || argv[0] != "tuisense" || argv[1] != "--web-demo" {
		t.Fatalf("unexpected argv %q", argv)
	}
	for _, a := range argv {
		if strings.Contains(a, "sh") && !strings.Contains(a, "tuisense") {
			t.Fatalf("no shell may be involved: %q", argv)
		}
	}
	env := procFile(t, pid, "environ")
	if strings.Contains(env, "TUISENSE_WEB_TEST_SECRET") || strings.Contains(env, "do-not-leak") {
		t.Fatal("server environment leaked into the session")
	}
	if !strings.Contains(env, "HOME="+sess.dirs.Home) || !strings.Contains(env, "COLORTERM=truecolor") {
		t.Fatalf("session environment incomplete: %q", env)
	}

	status := procFile(t, pid, "status")
	if statusField(status, "NoNewPrivs") != "1" {
		t.Fatal("no_new_privs must be set")
	}
	for _, f := range []string{"CapEff", "CapPrm", "CapInh", "CapAmb"} {
		if v := statusField(status, f); strings.Trim(v, "0") != "" {
			t.Fatalf("%s must be empty, got %s", f, v)
		}
	}
	limits := procFile(t, pid, "limits")
	for _, want := range []string{
		fmt.Sprintf("Max address space         %d", 900<<20),
		"Max cpu time              321",
		"Max core file size        0",
		"Max open files            64",
	} {
		if !strings.Contains(limits, want) {
			t.Fatalf("missing limit %q in\n%s", want, limits)
		}
	}
	// No hardware: the process holds only its terminal, pipes and files of
	// its own session.
	fds, _ := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	for _, fd := range fds {
		target, _ := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, fd.Name()))
		for _, bad := range []string{"/dev/hidraw", "/dev/input", "/dev/bus/usb", "/dev/uinput", "/dev/snd", "/dev/rfkill"} {
			if strings.HasPrefix(target, bad) {
				t.Fatalf("session process opened hardware %s", target)
			}
		}
	}
	if srv.m.plan.Landlock > 0 {
		if v := statusField(status, "Seccomp"); v == "" {
			t.Log("seccomp status unavailable")
		}
	}
}

// runHelper runs the sandbox helper with the test binary as the target.
func runHelper(t *testing.T, plan sandboxPlan, rw, action string) (string, int) {
	t.Helper()
	self, _ := os.Executable()
	// No address-space limit here: the target is this cgo-linked test
	// binary, whose glibc thread stacks can exceed it (pthread_create
	// fails with EAGAIN). Limits are covered with the static tuisense in
	// TestSessionProcessIsConfined; this test is about Landlock.
	h := helperArgs{Landlock: plan.Landlock, NoFile: 64, RW: rw, Binary: self, TuiSense: []string{"probe"}}
	cmd := &exec.Cmd{Path: self, Args: h.argv(self), Env: []string{"TUISENSE_WEBTEST_ACTION=" + action},
		SysProcAttr: &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}}
	if plan.Namespaces {
		namespaceAttrs(cmd.SysProcAttr)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	r, w, _ := os.Pipe()
	defer r.Close()
	cmd.ExtraFiles = []*os.File{w}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	w.Close()
	if st, err := readStatus(r, 5*time.Second); err != nil || !strings.HasPrefix(st, "ok") {
		t.Fatalf("helper failed: %q %v", st, err)
	}
	_ = cmd.Wait()
	return out.String(), cmd.ProcessState.ExitCode()
}

// Landlock confines a session to its own directory, without TCP and
// without signals to other processes.
func TestLandlockConfinesTheProcess(t *testing.T) {
	self, _ := os.Executable()
	plan, err := probeSandbox(SandboxAuto, self)
	if err != nil || plan.Landlock == 0 {
		t.Skip("Landlock is not available on this kernel")
	}
	rw := t.TempDir()
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		action  string
		allowed bool
	}{
		{"write:" + filepath.Join(rw, "ok.txt"), true},
		{"read:" + filepath.Join(other, "secret"), false}, // another session's data
		{"write:" + filepath.Join(other, "x"), false},
		{"read:/etc/passwd", false},
		{"read:/proc/1/environ", false},
	}
	for _, c := range cases {
		out, code := runHelper(t, plan, rw, c.action)
		t.Logf("%s → %s", c.action, strings.TrimSpace(out))
		if (code == 0) != c.allowed {
			t.Errorf("%s: allowed=%v, want %v (%s)", c.action, code == 0, c.allowed, strings.TrimSpace(out))
		}
	}
	if plan.Landlock >= 4 || plan.Namespaces {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
		out, code := runHelper(t, plan, rw, "tcp:"+ln.Addr().String())
		t.Logf("tcp → %s", strings.TrimSpace(out))
		if code == 0 {
			t.Errorf("a session must not open network connections: %s", out)
		}
	}
	if plan.Landlock >= 6 || plan.Namespaces {
		out, code := runHelper(t, plan, rw, fmt.Sprintf("kill:%d", os.Getpid()))
		t.Logf("kill → %s", strings.TrimSpace(out))
		if code == 0 {
			t.Errorf("a session must not signal processes outside its sandbox: %s", out)
		}
	}

	// Container platforms often forbid namespaces; Landlock alone must
	// still confine files, TCP and signals.
	if plan.Landlock >= 6 {
		alone := sandboxPlan{Mode: SandboxLandlock, Landlock: plan.Landlock}
		if _, code := runHelper(t, alone, rw, "read:/etc/passwd"); code == 0 {
			t.Error("Landlock alone must deny files outside the session")
		}
		out, code := runHelper(t, alone, rw, fmt.Sprintf("kill:%d", os.Getpid()))
		t.Logf("kill without namespaces → %s", strings.TrimSpace(out))
		if code == 0 {
			t.Error("Landlock alone must deny signals to the server")
		}
	}
}

func TestOriginValidation(t *testing.T) {
	_, hs := newTestServer(t, nil)
	for _, origin := range []string{"", "https://evil.example", "null", "http://127.0.0.1.evil.example"} {
		_, resp, err := dialRaw(hs, origin)
		if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Fatalf("origin %q must be refused (resp %v, err %v)", origin, resp, err)
		}
	}
	_, hs2 := newTestServer(t, func(c *Config) { c.AllowedOrigins = []string{"https://demo.tuisense.example"} })
	if _, resp, err := dialRaw(hs2, hs2.URL); err == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatal("with an allow-list, only the listed origins may connect")
	}
	ws, _, err := dialRaw(hs2, "https://demo.tuisense.example")
	if err != nil {
		t.Fatalf("a listed origin must connect: %v", err)
	}
	ws.CloseNow()
}

func TestCapacityPerAddressAndStartLimits(t *testing.T) {
	srv, hs := newTestServer(t, func(c *Config) { c.MaxSessions = 2; c.MaxSessionsPerIP = 5; c.StartsPerMinute = 100 })
	a := openTerm(t, hs, map[string]any{"cols": 80, "rows": 24})
	b := openTerm(t, hs, map[string]any{"cols": 80, "rows": 24})
	a.waitMsg("ready", startTimeout)
	b.waitMsg("ready", startTimeout)
	c := openTerm(t, hs, map[string]any{"cols": 80, "rows": 24})
	if m := c.waitMsg("error", 5*time.Second); m["code"] != "capacity" {
		t.Fatalf("the third session must be refused for capacity, got %v", m)
	}
	if srv.Manager().Active() != 2 {
		t.Fatal("a refused session must not hold resources")
	}

	_, hs2 := newTestServer(t, func(c *Config) { c.MaxSessionsPerIP = 1 })
	d := openTerm(t, hs2, map[string]any{"cols": 80, "rows": 24})
	d.waitMsg("ready", startTimeout)
	e := openTerm(t, hs2, map[string]any{"cols": 80, "rows": 24})
	if m := e.waitMsg("error", 5*time.Second); m["code"] != "per_ip" {
		t.Fatalf("a second session from one address must be refused, got %v", m)
	}

	_, hs3 := newTestServer(t, func(c *Config) { c.StartsPerMinute = 1 })
	f := openTerm(t, hs3, map[string]any{"cols": 80, "rows": 24})
	f.waitMsg("ready", startTimeout)
	f.sendJSON(map[string]string{"type": "close"})
	f.waitMsg("exit", 5*time.Second)
	g := openTerm(t, hs3, map[string]any{"cols": 80, "rows": 24})
	if m := g.waitMsg("error", 5*time.Second); m["code"] != "rate_limited" {
		t.Fatalf("starts must be rate limited per address, got %v", m)
	}
}

func TestHTTPRateLimit(t *testing.T) {
	_, hs := newTestServer(t, nil)
	limited := false
	for i := 0; i < 130 && !limited; i++ {
		resp, err := http.Get(hs.URL + "/api/info")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		limited = resp.StatusCode == http.StatusTooManyRequests
	}
	if !limited {
		t.Fatal("API requests must be rate limited")
	}
	resp, err := http.Get(hs.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatal("health checks are never rate limited")
	}
	if resp.Header.Get("Content-Security-Policy") == "" || resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("security headers missing")
	}
}

func TestInputFloodIsDropped(t *testing.T) {
	_, hs := newTestServer(t, func(c *Config) { c.InputBytesPerSec = 1024 })
	c := openTerm(t, hs, map[string]any{"cols": 80, "rows": 24})
	c.waitMsg("ready", startTimeout)
	chunk := strings.Repeat("a", 4096)
	for i := 0; i < 4; i++ {
		c.input(chunk)
	}
	if m := c.waitMsg("notice", 5*time.Second); !strings.Contains(m["message"].(string), "too fast") {
		t.Fatalf("unexpected notice %v", m)
	}
}

// Idle, expired and abandoned sessions are ended: process killed, PTY
// released, data deleted, slot freed.
func TestTimeoutsCleanUpEverything(t *testing.T) {
	srv, hs := newTestServer(t, func(c *Config) {
		c.IdleTimeout = 1500 * time.Millisecond
		c.ReconnectGrace = 500 * time.Millisecond
		c.MaxDuration = time.Hour
	})
	m := srv.Manager()

	// Idle.
	c := openTerm(t, hs, map[string]any{"cols": 80, "rows": 24})
	ready := c.waitMsg("ready", startTimeout)
	s := sessionOf(t, srv, ready["session"].(string))
	pid, dir := pidOf(s), s.dirs.Root
	time.Sleep(1600 * time.Millisecond)
	m.Reap()
	if msg := c.waitMsg("exit", 3*time.Second); msg["reason"] != ReasonIdle || msg["final"] != true {
		t.Fatalf("expected an idle end, got %v", msg)
	}
	waitFor(t, "idle cleanup", 3*time.Second, func() bool { return !alive(pid) && m.Active() == 0 })
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("session data must be deleted")
	}

	// Abandoned (the browser never came back).
	c2 := openTerm(t, hs, map[string]any{"cols": 80, "rows": 24})
	s2 := sessionOf(t, srv, c2.waitMsg("ready", startTimeout)["session"].(string))
	pid2 := pidOf(s2)
	c2.ws.CloseNow()
	<-c2.done
	waitFor(t, "detach", 2*time.Second, func() bool { s2.mu.Lock(); defer s2.mu.Unlock(); return s2.conn == nil })
	time.Sleep(600 * time.Millisecond)
	m.Reap()
	waitFor(t, "abandoned cleanup", 3*time.Second, func() bool { return !alive(pid2) && m.Active() == 0 })

	// Expired.
	srv3, hs3 := newTestServer(t, func(c *Config) { c.MaxDuration = time.Second })
	c3 := openTerm(t, hs3, map[string]any{"cols": 80, "rows": 24})
	c3.waitMsg("ready", startTimeout)
	time.Sleep(1100 * time.Millisecond)
	c3.input("x") // activity does not extend the maximum duration
	srv3.Manager().Reap()
	if msg := c3.waitMsg("exit", 3*time.Second); msg["reason"] != ReasonExpired {
		t.Fatalf("expected an expired end, got %v", msg)
	}
}

func TestShutdownEndsEverySession(t *testing.T) {
	srv, hs := newTestServer(t, nil)
	c := openTerm(t, hs, map[string]any{"cols": 80, "rows": 24})
	s := sessionOf(t, srv, c.waitMsg("ready", startTimeout)["session"].(string))
	pid := pidOf(s)
	srv.Manager().Shutdown()
	if msg := c.waitMsg("exit", 3*time.Second); msg["reason"] != ReasonShutdown {
		t.Fatalf("unexpected %v", msg)
	}
	if alive(pid) || srv.Manager().Active() != 0 {
		t.Fatal("shutdown must kill every session process")
	}
	if _, err := srv.Manager().Create("127.0.0.1", "en", 80, 24); err != ErrClosed {
		t.Fatal("no session may start during shutdown")
	}
}

func TestConfigValidation(t *testing.T) {
	good := testConfig(t)
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []func(*Config){
		func(c *Config) { c.Binary = "tuisense" },
		func(c *Config) { c.Binary = "/nonexistent/tuisense" },
		func(c *Config) { c.Binary = "/etc/passwd" },
		func(c *Config) { c.AllowedOrigins = []string{"demo.example.com"} },
		func(c *Config) { c.AllowedOrigins = []string{"https://demo.example.com/path"} },
		func(c *Config) { c.Sandbox = "off" },
		func(c *Config) { c.MemoryLimitMB = 512 },
		func(c *Config) { c.SoftMemoryMB = 4096 },
		func(c *Config) { c.MaxSessions = 0 },
		func(c *Config) { c.MaxCols = 10 },
	}
	for i, mutate := range bad {
		c := good
		mutate(&c)
		if c.Validate() == nil {
			t.Errorf("bad config %d accepted", i)
		}
	}
	env := map[string]string{
		"TUISENSE_WEB_BINARY":          tuisenseBin,
		"TUISENSE_WEB_MAX_SESSIONS":    "7",
		"TUISENSE_WEB_IDLE_TIMEOUT":    "90s",
		"TUISENSE_WEB_ALLOWED_ORIGINS": "https://a.example, https://b.example",
		"TUISENSE_WEB_SANDBOX":         "LANDLOCK",
	}
	c, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil || c.MaxSessions != 7 || c.IdleTimeout != 90*time.Second || len(c.AllowedOrigins) != 2 || c.Sandbox != SandboxLandlock {
		t.Fatalf("env config: %+v %v", c, err)
	}
	env["TUISENSE_WEB_MAX_SESSIONS"] = "many"
	if _, err := ConfigFromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("a malformed number must be rejected")
	}
}

func TestModeTrackerAcrossChunks(t *testing.T) {
	m := newModeTracker()
	seq := "\x1b[?1049h\x1b[?25l\x1b[?1003h\x1b[?1006h\x1b[?1004;2004h text \x1b[?1003l\x1b[?1002h"
	for i := 0; i < len(seq); i += 3 { // split everywhere
		m.Feed([]byte(seq[i:min(i+3, len(seq))]))
	}
	got := string(m.Replay())
	want := "\x1b[?1049h\x1b[?25l\x1b[?1002h\x1b[?1003l\x1b[?1006h\x1b[?1004h\x1b[?2004h"
	if got != want {
		t.Fatalf("replay %q, want %q", got, want)
	}
}

func TestRateLimiterRefills(t *testing.T) {
	now := time.Unix(0, 0)
	r := newRateLimiter(2, time.Minute)
	r.now = func() time.Time { return now }
	if !r.Allow("a") || !r.Allow("a") || r.Allow("a") {
		t.Fatal("burst of 2 expected")
	}
	if !r.Allow("b") {
		t.Fatal("keys are independent")
	}
	now = now.Add(30 * time.Second)
	if !r.Allow("a") || r.Allow("a") {
		t.Fatal("one token refills every 30 s")
	}
	now = now.Add(time.Hour)
	r.Prune()
	if len(r.buckets) != 0 {
		t.Fatal("idle buckets are forgotten")
	}
	b := newByteBudget(100)
	t0 := time.Now()
	if !b.Take(200, t0) || b.Take(1, t0) || !b.Take(50, t0.Add(500*time.Millisecond)) {
		t.Fatal("byte budget: burst 2×rate, then rate per second")
	}
}
