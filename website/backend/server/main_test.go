//go:build linux

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// The integration tests run the real TuiSense, built from this repository.
var tuisenseBin string

const (
	testVersion = "0.0.0-webtest"
	testCommit  = "0123456789abcdef0123456789abcdef01234567"
)

func TestMain(m *testing.M) {
	// The test binary doubles as the sandbox helper, exactly like
	// tuisense-webd, and as a probe program for the sandbox tests.
	if len(os.Args) > 1 && os.Args[1] == SandboxArg {
		os.Exit(SandboxMain(os.Args[2:]))
	}
	if a := os.Getenv("TUISENSE_WEBTEST_ACTION"); a != "" {
		os.Exit(probeAction(a))
	}
	dir, err := os.MkdirTemp("", "tuisense-webtest-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	tuisenseBin = filepath.Join(dir, "tuisense")
	build := exec.Command("go", "build", "-trimpath", "-ldflags",
		"-X github.com/Phobby/TuiSense/internal/version.Version="+testVersion+
			" -X github.com/Phobby/TuiSense/internal/version.Commit="+testCommit,
		"-o", tuisenseBin, "github.com/Phobby/TuiSense/cmd/tuisense")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building tuisense:", err)
		os.Exit(1)
	}
	// A server secret that must never reach a session process.
	os.Setenv("TUISENSE_WEB_TEST_SECRET", "do-not-leak")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// probeAction runs inside the sandbox helper for the sandbox tests.
func probeAction(a string) int {
	kind, arg, _ := strings.Cut(a, ":")
	var err error
	switch kind {
	case "read":
		_, err = os.ReadFile(arg)
	case "write":
		err = os.WriteFile(arg, []byte("x"), 0o600)
	case "tcp":
		var c net.Conn
		c, err = net.DialTimeout("tcp", arg, 2*time.Second)
		if err == nil {
			c.Close()
		}
	case "kill":
		var pid int
		fmt.Sscanf(arg, "%d", &pid)
		err = syscall.Kill(pid, syscall.SIGURG)
	case "env":
		fmt.Println(strings.Join(os.Environ(), "\n"))
	}
	if err != nil {
		fmt.Println("denied:", err)
		return 3
	}
	fmt.Println("allowed")
	return 0
}

func testConfig(t *testing.T) Config {
	cfg := DefaultConfig()
	cfg.Binary = tuisenseBin
	cfg.SessionRoot = filepath.Join(t.TempDir(), "sessions")
	cfg.Listen = "127.0.0.1:0"
	return cfg
}

func newTestServer(t *testing.T, mutate func(*Config)) (*Server, *httptest.Server) {
	t.Helper()
	cfg := testConfig(t)
	if mutate != nil {
		mutate(&cfg)
	}
	srv, err := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		srv.Manager().Shutdown()
		hs.Close()
	})
	return srv, hs
}

// term is a test browser: a WebSocket plus a screen emulator.
type term struct {
	t    *testing.T
	ws   *websocket.Conn
	mu   sync.Mutex
	scr  *screen
	msgs chan map[string]any
	raw  []byte
	done chan struct{}
}

func wsURL(hs *httptest.Server) string {
	return "ws" + strings.TrimPrefix(hs.URL, "http") + "/api/terminal"
}

func dialRaw(hs *httptest.Server, origin string) (*websocket.Conn, *http.Response, error) {
	h := http.Header{}
	if origin != "" {
		h.Set("Origin", origin)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return websocket.Dial(ctx, wsURL(hs), &websocket.DialOptions{HTTPHeader: h})
}

func openTerm(t *testing.T, hs *httptest.Server, start map[string]any) *term {
	t.Helper()
	ws, _, err := dialRaw(hs, hs.URL)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetReadLimit(1 << 22)
	start["type"] = "start"
	cols, _ := start["cols"].(int)
	rows, _ := start["rows"].(int)
	c := &term{t: t, ws: ws, scr: newScreen(cols, rows), msgs: make(chan map[string]any, 64), done: make(chan struct{})}
	go c.read()
	c.sendJSON(start)
	t.Cleanup(func() { ws.Close(websocket.StatusNormalClosure, "") })
	return c
}

func (c *term) read() {
	defer close(c.done)
	for {
		typ, data, err := c.ws.Read(context.Background())
		if err != nil {
			return
		}
		if typ == websocket.MessageBinary {
			c.mu.Lock()
			c.raw = append(c.raw, data...)
			scr := c.scr
			c.mu.Unlock()
			scr.Write(data)
			continue
		}
		var m map[string]any
		if json.Unmarshal(data, &m) == nil {
			c.msgs <- m
		}
	}
}

func (c *term) screen() *screen {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.scr
}

func (c *term) rawOutput() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.raw)
}

func (c *term) sendJSON(v any) {
	b, _ := json.Marshal(v)
	if err := c.ws.Write(context.Background(), websocket.MessageText, b); err != nil {
		c.t.Fatalf("send: %v", err)
	}
}

func (c *term) input(s string) {
	if err := c.ws.Write(context.Background(), websocket.MessageBinary, []byte(s)); err != nil {
		c.t.Fatalf("input: %v", err)
	}
}

// click sends an SGR mouse press and release at a 0-based cell, as
// xterm.js does with mouse reporting mode 1006.
func (c *term) click(x, y int) {
	c.input(fmt.Sprintf("\x1b[<0;%d;%dM", x+1, y+1))
	c.input(fmt.Sprintf("\x1b[<0;%d;%dm", x+1, y+1))
}

func (c *term) resize(cols, rows int) {
	c.mu.Lock()
	c.scr = newScreen(cols, rows)
	c.mu.Unlock()
	c.sendJSON(map[string]any{"type": "resize", "cols": cols, "rows": rows})
}

func (c *term) waitMsg(typ string, timeout time.Duration) map[string]any {
	c.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case m := <-c.msgs:
			if m["type"] == typ {
				return m
			}
		case <-deadline:
			c.t.Fatalf("no %q message within %s", typ, timeout)
		}
	}
}

func (c *term) waitText(text string, timeout time.Duration) (int, int) {
	c.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if x, y, ok := c.screen().Find(text); ok {
			return x, y
		}
		time.Sleep(30 * time.Millisecond)
	}
	c.t.Fatalf("%q did not appear within %s; screen:\n%s", text, timeout, c.screen().Text())
	return 0, 0
}

func (c *term) waitGone(text string, timeout time.Duration) {
	c.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, _, ok := c.screen().Find(text); !ok {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	c.t.Fatalf("%q still on screen after %s", text, timeout)
}

func sessionOf(t *testing.T, srv *Server, id string) *Session {
	t.Helper()
	m := srv.Manager()
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		t.Fatalf("no session %s", id)
	}
	return s
}

func pidOf(s *Session) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc == nil {
		return 0
	}
	return s.proc.Pid()
}

func alive(pid int) bool {
	return pid > 0 && !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
