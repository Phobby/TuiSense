//go:build linux

package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const startTimeout = 20 * time.Second

// The browser terminal shows the real TuiSense binary built from this
// repository: its version, its web-demo indicator and its 3D controller.
func TestTerminalRunsTheRealTuiSense(t *testing.T) {
	srv, hs := newTestServer(t, nil)
	if srv.App().Version != testVersion || srv.App().Commit != testCommit[:12] {
		t.Fatalf("the server must report the binary's version and commit, got %+v", srv.App())
	}
	c := openTerm(t, hs, map[string]any{"cols": 140, "rows": 42, "lang": "en"})
	ready := c.waitMsg("ready", startTimeout)
	if ready["resumed"] != false || ready["running"] != true || ready["token"] == "" {
		t.Fatalf("unexpected ready message %v", ready)
	}
	c.waitText("WEB DEMO — SIMULATED CONTROLLERS · v"+testVersion, startTimeout)
	c.waitText("DualSense", startTimeout)
	c.waitText("Diagnostics", startTimeout)
	text := c.screen().Text()
	if !strings.ContainsAny(text, "⣿⣶⣤") {
		t.Fatalf("the 3D controller (braille renderer) must be drawn:\n%s", text)
	}
	scr := c.screen()
	for _, mode := range []int{1049, 1003, 1006, 1004, 2004} {
		if !scr.Mode(mode) {
			t.Fatalf("TuiSense must enable DEC mode %d (alternate screen, mouse, focus, paste)", mode)
		}
	}
	if !strings.Contains(c.rawOutput(), "\x1b[38;2;") {
		t.Fatal("TuiSense must render in true colour (COLORTERM=truecolor)")
	}

	resp, err := http.Get(hs.URL + "/api/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info struct {
		App      AppInfo        `json:"app"`
		Sessions map[string]int `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.App.Version != testVersion || info.Sessions["active"] != 1 {
		t.Fatalf("unexpected /api/info %+v", info)
	}
}

// Keyboard input reaches TuiSense: help, the controller selector, model
// switching and the quit dialog all use the native key bindings.
func TestKeyboardDrivesTheNativeInterface(t *testing.T) {
	_, hs := newTestServer(t, nil)
	c := openTerm(t, hs, map[string]any{"cols": 140, "rows": 42})
	c.waitMsg("ready", startTimeout)
	c.waitText("WEB DEMO", startTimeout)

	c.input("?")
	c.waitText("General navigation", 5*time.Second)
	c.input("?")
	c.waitGone("General navigation", 5*time.Second)

	c.input("c")
	c.waitText("Choose a controller", 5*time.Second)
	c.waitText("GC20 V2 · simulated", 5*time.Second)
	c.input("\x1b[B") // down: DualSense Edge
	c.input("\r")
	c.waitText("DualSense Edge", 5*time.Second)

	c.input("2")
	c.waitText("Stick", 5*time.Second)

	time.Sleep(400 * time.Millisecond) // a fresh Esc, not an auto-repeat
	c.input("\x1b")
	c.waitText("Quit TuiSense?", 5*time.Second)
}

// Mouse reports (SGR 1006) reach TuiSense at the right cells, before and
// after the terminal is resized.
func TestMouseAndResize(t *testing.T) {
	srv, hs := newTestServer(t, nil)
	c := openTerm(t, hs, map[string]any{"cols": 140, "rows": 42})
	ready := c.waitMsg("ready", startTimeout)
	c.waitText("WEB DEMO", startTimeout)

	x, y := c.waitText("ADVANCED", 5*time.Second)
	c.click(x+2, y)
	sess := sessionOf(t, srv, ready["session"].(string))
	settings := filepath.Join(sess.dirs.Config, "tuisense", "settings.json")
	waitFor(t, "Advanced mode saved by a mouse click", 5*time.Second, func() bool {
		b, _ := os.ReadFile(settings)
		return strings.Contains(string(b), `"advanced"`)
	})

	c.resize(100, 30)
	waitFor(t, "the PTY to follow the browser size", 3*time.Second, func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return sess.cols == 100 && sess.rows == 30
	})
	c.waitText("DualSense", 5*time.Second)
	x, y = c.waitText("EZ ", 5*time.Second)
	if x >= 100 || y != 0 {
		t.Fatalf("after resizing, the header must be laid out for 100 columns (EZ at %d,%d)", x, y)
	}
	c.click(x+1, y)
	waitFor(t, "EZ mode saved by a click after the resize", 5*time.Second, func() bool {
		b, _ := os.ReadFile(settings)
		return strings.Contains(string(b), `"ez"`)
	})

	// Out-of-range sizes are clamped to the configured bounds.
	c.resize(5000, 2)
	waitFor(t, "clamped size", 3*time.Second, func() bool {
		sess.mu.Lock()
		defer sess.mu.Unlock()
		return int(sess.cols) == srv.cfg.MaxCols && int(sess.rows) == srv.cfg.MinRows
	})
}

// TuiSense's copy action reaches the browser as OSC 52, which the
// frontend hands to the visitor's clipboard.
func TestClipboardUsesTheBrowserTerminal(t *testing.T) {
	_, hs := newTestServer(t, nil)
	c := openTerm(t, hs, map[string]any{"cols": 140, "rows": 42})
	c.waitMsg("ready", startTimeout)
	c.waitText("WEB DEMO", startTimeout)
	c.input("y")
	waitFor(t, "an OSC 52 clipboard write", 5*time.Second, func() bool {
		return strings.Contains(c.rawOutput(), "\x1b]52;c;")
	})
}

// Quitting TuiSense never exposes a shell: the session reports the exit,
// ignores further input and can be restarted.
func TestExitRestartAndReset(t *testing.T) {
	srv, hs := newTestServer(t, nil)
	c := openTerm(t, hs, map[string]any{"cols": 140, "rows": 42})
	ready := c.waitMsg("ready", startTimeout)
	c.waitText("WEB DEMO", startTimeout)
	sess := sessionOf(t, srv, ready["session"].(string))
	first := pidOf(sess)

	x, y := c.waitText("ADVANCED", 5*time.Second)
	c.click(x+2, y)
	settings := filepath.Join(sess.dirs.Config, "tuisense", "settings.json")
	waitFor(t, "settings saved", 5*time.Second, func() bool {
		b, _ := os.ReadFile(settings)
		return strings.Contains(string(b), `"advanced"`)
	})

	time.Sleep(400 * time.Millisecond)
	c.input("\x1b")
	c.waitText("Quit TuiSense?", 5*time.Second)
	c.input("\r")
	exit := c.waitMsg("exit", 5*time.Second)
	if exit["reason"] != ReasonExited {
		t.Fatalf("unexpected exit %v", exit)
	}
	waitFor(t, "the process to be gone", 3*time.Second, func() bool { return !alive(first) && pidOf(sess) == 0 })
	before := len(c.rawOutput())
	c.input("echo pwned; id\r")
	time.Sleep(300 * time.Millisecond)
	if len(c.rawOutput()) != before {
		t.Fatalf("input after exit must go nowhere, got %q", c.rawOutput()[before:])
	}

	// Restart keeps the session's settings.
	time.Sleep(2100 * time.Millisecond) // restarts are rate limited
	c.sendJSON(map[string]string{"type": "restart"})
	c.waitMsg("started", startTimeout)
	c.waitText("WEB DEMO", startTimeout)
	second := pidOf(sess)
	if second == 0 || second == first {
		t.Fatal("restart must start a new process")
	}
	if b, _ := os.ReadFile(settings); !strings.Contains(string(b), `"advanced"`) {
		t.Fatal("restart keeps the session's settings")
	}

	// Reset deletes them.
	time.Sleep(2100 * time.Millisecond) // restarts are rate limited
	c.sendJSON(map[string]string{"type": "reset"})
	m := c.waitMsg("started", startTimeout)
	if m["reset"] != true {
		t.Fatalf("unexpected %v", m)
	}
	waitFor(t, "the old process to be gone", 3*time.Second, func() bool { return !alive(second) })
	c.waitText("WEB DEMO", startTimeout)
	time.Sleep(300 * time.Millisecond)
	if b, _ := os.ReadFile(settings); strings.Contains(string(b), `"advanced"`) {
		t.Fatal("reset must delete the session's settings")
	}
}

// A reload reconnects to the same session; the terminal modes are replayed
// and the screen is redrawn.
func TestReconnectResumesTheSession(t *testing.T) {
	srv, hs := newTestServer(t, nil)
	c := openTerm(t, hs, map[string]any{"cols": 120, "rows": 40})
	ready := c.waitMsg("ready", startTimeout)
	c.waitText("WEB DEMO", startTimeout)
	c.input("c")
	c.waitText("Choose a controller", 5*time.Second)
	id, token := ready["session"].(string), ready["token"].(string)
	pid := pidOf(sessionOf(t, srv, id))
	c.ws.Close(websocket.StatusGoingAway, "reload")
	<-c.done

	c2 := openTerm(t, hs, map[string]any{"cols": 120, "rows": 40, "resume": map[string]string{"session": id, "token": token}})
	r2 := c2.waitMsg("ready", 5*time.Second)
	if r2["resumed"] != true || r2["session"] != id {
		t.Fatalf("expected the same session, got %v", r2)
	}
	if pidOf(sessionOf(t, srv, id)) != pid {
		t.Fatal("resuming must keep the running process")
	}
	c2.waitText("Choose a controller", 5*time.Second)
	if !c2.screen().Mode(1003) || !c2.screen().Mode(1006) {
		t.Fatal("mouse modes must be replayed for the new terminal")
	}

	// A wrong token never reaches another visitor's session.
	c3 := openTerm(t, hs, map[string]any{"cols": 120, "rows": 40, "resume": map[string]string{"session": id, "token": strings.Repeat("0", 64)}})
	r3 := c3.waitMsg("ready", startTimeout)
	if r3["session"] == id || r3["resumed"] == true {
		t.Fatal("a wrong token must start a separate session")
	}
}

// Every visitor gets a separate process and private data.
func TestConcurrentSessionsAreIsolated(t *testing.T) {
	srv, hs := newTestServer(t, func(c *Config) { c.MaxSessionsPerIP = 4 })
	terms := make([]*term, 3)
	ids := make([]string, 3)
	var wg sync.WaitGroup
	for i := range terms {
		terms[i] = openTerm(t, hs, map[string]any{"cols": 120, "rows": 40})
	}
	for i, c := range terms {
		wg.Add(1)
		go func(i int, c *term) {
			defer wg.Done()
			ids[i] = c.waitMsg("ready", startTimeout)["session"].(string)
		}(i, c)
	}
	wg.Wait()
	for _, c := range terms {
		c.waitText("WEB DEMO", startTimeout)
	}
	pids := map[int]bool{}
	dirs := map[string]bool{}
	for _, id := range ids {
		s := sessionOf(t, srv, id)
		pids[pidOf(s)] = true
		dirs[s.dirs.Root] = true
		if st, err := os.Stat(s.dirs.Root); err != nil || st.Mode().Perm() != 0o700 {
			t.Fatalf("session directory must be private (0700): %v", err)
		}
	}
	if len(pids) != 3 || len(dirs) != 3 {
		t.Fatal("each session needs its own process and directory")
	}

	// A change in one session is invisible to the others.
	x, y := terms[0].waitText("ADVANCED", 5*time.Second)
	terms[0].click(x+2, y)
	a := sessionOf(t, srv, ids[0])
	waitFor(t, "settings in session A", 5*time.Second, func() bool {
		b, _ := os.ReadFile(filepath.Join(a.dirs.Config, "tuisense", "settings.json"))
		return strings.Contains(string(b), `"advanced"`)
	})
	b := sessionOf(t, srv, ids[1])
	if data, _ := os.ReadFile(filepath.Join(b.dirs.Config, "tuisense", "settings.json")); strings.Contains(string(data), `"advanced"`) {
		t.Fatal("session B must not see session A's settings")
	}

	if srv.m.plan.Namespaces {
		ns := func(pid int) string { l, _ := os.Readlink("/proc/" + itoa(pid) + "/ns/pid"); return l }
		if ns(pidOf(a)) == ns(pidOf(b)) || ns(pidOf(a)) == ns(os.Getpid()) {
			t.Fatal("each session must run in its own PID namespace")
		}
		net := func(pid int) string { l, _ := os.Readlink("/proc/" + itoa(pid) + "/ns/net"); return l }
		if net(pidOf(a)) == net(os.Getpid()) {
			t.Fatal("sessions must have no host network")
		}
	}
}
