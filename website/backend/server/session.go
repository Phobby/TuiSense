//go:build linux

package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Errors returned when a session cannot be started.
var (
	ErrCapacity    = errors.New("the demo is at capacity, please try again in a few minutes")
	ErrPerIP       = errors.New("too many demo sessions from your network")
	ErrRateLimited = errors.New("too many new sessions, please wait a moment")
	ErrClosed      = errors.New("the server is shutting down")
	ErrTooSoon     = errors.New("please wait a moment before restarting again")
)

// End reasons sent to the browser.
const (
	ReasonExited       = "exited"       // TuiSense quit (the visitor may restart it)
	ReasonIdle         = "idle"         // no input for the idle timeout
	ReasonExpired      = "expired"      // the maximum session duration was reached
	ReasonDisconnected = "disconnected" // the browser did not come back in time
	ReasonReplaced     = "replaced"     // the session was opened in another tab
	ReasonShutdown     = "shutdown"     // the server is stopping
	ReasonClosed       = "closed"       // the visitor ended the session
	ReasonError        = "error"
)

var sessionIDRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // the kernel's CSPRNG never fails on Linux ≥ 3.17
	}
	return hex.EncodeToString(b)
}

// client is one attached browser WebSocket.
type client struct {
	ws      *websocket.Conn
	writeMu sync.Mutex
	closed  chan struct{}
	once    sync.Once
}

func newClient(ws *websocket.Conn) *client { return &client{ws: ws, closed: make(chan struct{})} }

const writeTimeout = 10 * time.Second

// writeLocked sends one message; the caller holds writeMu.
func (c *client) writeLocked(typ websocket.MessageType, b []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	return c.ws.Write(ctx, typ, b)
}

func (c *client) send(typ websocket.MessageType, b []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.writeLocked(typ, b)
}

func (c *client) sendJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.send(websocket.MessageText, b)
}

func (c *client) close(code websocket.StatusCode, reason string) {
	c.once.Do(func() {
		close(c.closed)
		_ = c.ws.Close(code, reason)
	})
}

// Session is one visitor's isolated TuiSense.
type Session struct {
	m       *Manager
	ID      string // public identifier (logs, directory name)
	token   string // secret that lets the same browser tab reconnect
	ip      string
	lang    string
	dirs    sessionDirs
	created time.Time

	mu          sync.Mutex
	proc        *Process
	conn        *client
	detachedAt  time.Time
	lastInput   time.Time
	cols, rows  uint16
	modes       *modeTracker
	budget      *byteBudget
	lastStart   time.Time
	ended       bool
	idleWarned  bool
	expWarned   bool
	inputDenied time.Time
}

// Token returns the reconnect secret (sent only to the session's browser).
func (s *Session) Token() string { return s.token }

func (s *Session) checkToken(token string) bool {
	return subtle.ConstantTimeCompare([]byte(s.token), []byte(token)) == 1
}

// Manager owns every session on this server.
type Manager struct {
	cfg  Config
	self string
	plan sandboxPlan
	log  *slog.Logger

	mu       sync.Mutex
	sessions map[string]*Session
	perIP    map[string]int
	closed   bool
	starts   *rateLimiter
	now      func() time.Time
}

// NewManager prepares the session root (removing what a previous run left).
func NewManager(cfg Config, self string, plan sandboxPlan, log *slog.Logger) (*Manager, error) {
	if err := os.MkdirAll(cfg.SessionRoot, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(cfg.SessionRoot, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(cfg.SessionRoot)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if sessionIDRe.MatchString(e.Name()) {
			_ = os.RemoveAll(filepath.Join(cfg.SessionRoot, e.Name()))
		}
	}
	return &Manager{cfg: cfg, self: self, plan: plan, log: log, sessions: map[string]*Session{}, perIP: map[string]int{},
		starts: newRateLimiter(cfg.StartsPerMinute, time.Minute), now: time.Now}, nil
}

// Active is the number of live sessions.
func (m *Manager) Active() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func (m *Manager) clampSize(cols, rows int) (uint16, uint16) {
	cols = min(max(cols, m.cfg.MinCols), m.cfg.MaxCols)
	rows = min(max(rows, m.cfg.MinRows), m.cfg.MaxRows)
	return uint16(cols), uint16(rows)
}

func normalizeLang(l string) string {
	if len(l) >= 2 && (l[:2] == "tr" || l[:2] == "TR") {
		return "tr"
	}
	return "en"
}

// Create starts a new session for a client address.
func (m *Manager) Create(ip, lang string, cols, rows int) (*Session, error) {
	m.mu.Lock()
	switch {
	case m.closed:
		m.mu.Unlock()
		return nil, ErrClosed
	case len(m.sessions) >= m.cfg.MaxSessions:
		m.mu.Unlock()
		return nil, ErrCapacity
	case m.perIP[ip] >= m.cfg.MaxSessionsPerIP:
		m.mu.Unlock()
		return nil, ErrPerIP
	case !m.starts.Allow(ip):
		m.mu.Unlock()
		return nil, ErrRateLimited
	}
	id := randomHex(16)
	now := m.now()
	s := &Session{m: m, ID: id, token: randomHex(32), ip: ip, lang: normalizeLang(lang),
		dirs: newSessionDirs(filepath.Join(m.cfg.SessionRoot, id)), created: now, lastInput: now,
		modes: newModeTracker(), budget: newByteBudget(m.cfg.InputBytesPerSec)}
	s.cols, s.rows = m.clampSize(cols, rows)
	// Reserve the slot before the (slow) process start.
	m.sessions[id] = s
	m.perIP[ip]++
	m.mu.Unlock()

	if err := s.dirs.create(); err != nil {
		m.remove(s)
		return nil, err
	}
	p, err := s.spawn()
	if err != nil {
		_ = os.RemoveAll(s.dirs.Root)
		m.remove(s)
		return nil, err
	}
	s.mu.Lock()
	s.proc, s.lastStart = p, now
	s.mu.Unlock()
	go s.pump(p)
	m.log.Info("session started", "session", id, "pid", p.Pid(), "active", m.Active())
	return s, nil
}

// Resume finds a live session by id and token.
func (m *Manager) Resume(id, token string) (*Session, bool) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok || !s.checkToken(token) {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s, !s.ended
}

func (m *Manager) remove(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.sessions[s.ID]; !ok {
		return
	}
	delete(m.sessions, s.ID)
	if m.perIP[s.ip]--; m.perIP[s.ip] <= 0 {
		delete(m.perIP, s.ip)
	}
}

func (s *Session) spawn() (*Process, error) {
	return startProcess(processSpec{
		Self: s.m.self, Binary: s.m.cfg.Binary, Dirs: s.dirs, Lang: s.lang,
		Cols: s.cols, Rows: s.rows, Plan: s.m.plan, Config: s.m.cfg,
	})
}

// pump copies terminal output to the attached browser until p exits.
func (s *Session) pump(p *Process) {
	buf := make([]byte, 32*1024)
	for {
		n, err := p.Read(buf)
		if n > 0 {
			s.mu.Lock()
			current := s.proc == p
			if current {
				s.modes.Feed(buf[:n])
			}
			c := s.conn
			s.mu.Unlock()
			if current && c != nil {
				if werr := c.send(websocket.MessageBinary, buf[:n]); werr != nil {
					s.detach(c)
					c.close(websocket.StatusGoingAway, "write failed")
				}
			}
		}
		if err != nil {
			break
		}
	}
	<-p.Done()
	s.mu.Lock()
	current := s.proc == p
	if current {
		s.proc = nil
	}
	c := s.conn
	s.mu.Unlock()
	if current {
		s.m.log.Info("session process exited", "session", s.ID, "code", p.ExitCode())
		p.Stop(0) // release the pseudo-terminal
		if c != nil {
			_ = c.sendJSON(map[string]any{"type": "exit", "reason": ReasonExited, "code": p.ExitCode()})
		}
	}
}

// readyMessage is the first message of every connection.
type readyMessage struct {
	Type      string `json:"type"`
	Session   string `json:"session"`
	Token     string `json:"token"`
	Resumed   bool   `json:"resumed"`
	Running   bool   `json:"running"`
	Cols      uint16 `json:"cols"`
	Rows      uint16 `json:"rows"`
	ExpiresIn int    `json:"expiresIn"` // seconds
	IdleAfter int    `json:"idleAfter"` // seconds
}

// attach makes c the session's browser, replacing a previous one. The
// ready message and the terminal mode replay are written before any
// further output, so the new terminal is in the right state.
func (s *Session) attach(c *client, resumed bool, cols, rows int) error {
	c.writeMu.Lock()
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		c.writeMu.Unlock()
		return ErrClosed
	}
	old := s.conn
	s.conn = c
	s.detachedAt = time.Time{}
	if cols > 0 && rows > 0 {
		s.cols, s.rows = s.m.clampSize(cols, rows)
	}
	p := s.proc
	replay := s.modes.Replay()
	msg := readyMessage{Type: "ready", Session: s.ID, Token: s.token, Resumed: resumed, Running: p != nil,
		Cols: s.cols, Rows: s.rows,
		ExpiresIn: int(s.m.cfg.MaxDuration.Seconds() - s.m.now().Sub(s.created).Seconds()),
		IdleAfter: int(s.m.cfg.IdleTimeout.Seconds())}
	cols16, rows16 := s.cols, s.rows
	s.mu.Unlock()

	b, _ := json.Marshal(msg)
	err := c.writeLocked(websocket.MessageText, b)
	if err == nil && resumed && len(replay) > 0 {
		err = c.writeLocked(websocket.MessageBinary, replay)
	}
	c.writeMu.Unlock()
	if old != nil && old != c {
		_ = old.sendJSON(map[string]any{"type": "exit", "reason": ReasonReplaced})
		old.close(websocket.StatusNormalClosure, ReasonReplaced)
	}
	if err != nil {
		return err
	}
	if p != nil {
		if resumed {
			// A one-row nudge makes TuiSense redraw the whole screen for
			// the fresh browser terminal.
			_ = p.Resize(cols16, rows16-1)
			time.Sleep(60 * time.Millisecond)
		}
		_ = p.Resize(cols16, rows16)
	}
	return nil
}

// detach forgets c if it is still the attached browser.
func (s *Session) detach(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == c {
		s.conn = nil
		s.detachedAt = s.m.now()
	}
}

// Input forwards keyboard, mouse and paste bytes, within the input budget.
func (s *Session) Input(b []byte) {
	now := s.m.now()
	s.mu.Lock()
	p, c := s.proc, s.conn
	ok := s.budget.Take(len(b), now)
	if ok {
		s.lastInput, s.idleWarned = now, false
	}
	notify := !ok && now.Sub(s.inputDenied) > 5*time.Second
	if notify {
		s.inputDenied = now
	}
	s.mu.Unlock()
	if !ok {
		if notify && c != nil {
			_ = c.sendJSON(map[string]any{"type": "notice", "message": "Input is arriving too fast; some of it was dropped."})
		}
		return
	}
	if p != nil {
		_, _ = p.Write(b)
	}
}

// Resize changes the terminal size within the configured bounds.
func (s *Session) Resize(cols, rows int) {
	s.mu.Lock()
	s.cols, s.rows = s.m.clampSize(cols, rows)
	p, c, r := s.proc, s.cols, s.rows
	s.mu.Unlock()
	if p != nil {
		_ = p.Resize(c, r)
	}
}

// Restart replaces the TuiSense process. With reset, the session's
// settings, profiles and reports are deleted first.
func (s *Session) Restart(reset bool) error {
	now := s.m.now()
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return ErrClosed
	}
	if now.Sub(s.lastStart) < 2*time.Second {
		s.mu.Unlock()
		return ErrTooSoon
	}
	old := s.proc
	s.proc, s.lastStart, s.lastInput = nil, now, now
	s.modes = newModeTracker()
	s.mu.Unlock()
	if old != nil {
		old.Stop(time.Second)
	}
	if reset {
		if err := os.RemoveAll(s.dirs.Root); err != nil {
			return err
		}
		if err := s.dirs.create(); err != nil {
			return err
		}
	}
	p, err := s.spawn()
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		p.Stop(0)
		return ErrClosed
	}
	c := s.conn
	s.mu.Unlock()
	// Tell the browser to clear its terminal before any new output.
	if c != nil {
		c.writeMu.Lock()
	}
	s.mu.Lock()
	s.proc = p
	s.mu.Unlock()
	if c != nil {
		b, _ := json.Marshal(map[string]any{"type": "started", "reset": reset})
		_ = c.writeLocked(websocket.MessageText, b)
		c.writeMu.Unlock()
	}
	go s.pump(p)
	s.m.log.Info("session restarted", "session", s.ID, "reset", reset, "pid", p.Pid())
	return nil
}

// End stops the session for good: the process is killed, the pseudo-
// terminal released and the private directory deleted.
func (s *Session) End(reason string) {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	p, c := s.proc, s.conn
	s.proc, s.conn = nil, nil
	s.mu.Unlock()
	if c != nil {
		_ = c.sendJSON(map[string]any{"type": "exit", "reason": reason, "final": true})
		c.close(websocket.StatusNormalClosure, reason)
	}
	if p != nil {
		p.Stop(time.Second)
	}
	if err := os.RemoveAll(s.dirs.Root); err != nil {
		s.m.log.Warn("could not remove session data", "session", s.ID, "err", err)
	}
	s.m.remove(s)
	s.m.log.Info("session ended", "session", s.ID, "reason", reason, "active", s.m.Active())
}

// Reap ends sessions that are idle, expired or abandoned, and warns
// browsers shortly before their session ends.
func (m *Manager) Reap() {
	now := m.now()
	m.mu.Lock()
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.Unlock()
	m.starts.Prune()
	for _, s := range list {
		s.mu.Lock()
		age, idle := now.Sub(s.created), now.Sub(s.lastInput)
		detached := s.conn == nil && !s.detachedAt.IsZero() && now.Sub(s.detachedAt) > m.cfg.ReconnectGrace
		c := s.conn
		warnIdle := c != nil && !s.idleWarned && idle > m.cfg.IdleTimeout-time.Minute && m.cfg.IdleTimeout > 2*time.Minute
		warnExp := c != nil && !s.expWarned && age > m.cfg.MaxDuration-2*time.Minute && m.cfg.MaxDuration > 4*time.Minute
		s.idleWarned = s.idleWarned || warnIdle
		s.expWarned = s.expWarned || warnExp
		s.mu.Unlock()
		switch {
		case age > m.cfg.MaxDuration:
			s.End(ReasonExpired)
		case idle > m.cfg.IdleTimeout:
			s.End(ReasonIdle)
		case detached:
			s.End(ReasonDisconnected)
		default:
			if warnIdle {
				_ = c.sendJSON(map[string]any{"type": "warning", "reason": ReasonIdle, "seconds": int((m.cfg.IdleTimeout - idle).Seconds())})
			}
			if warnExp {
				_ = c.sendJSON(map[string]any{"type": "warning", "reason": ReasonExpired, "seconds": int((m.cfg.MaxDuration - age).Seconds())})
			}
		}
	}
}

// Run reaps sessions until ctx ends.
func (m *Manager) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.Reap()
		}
	}
}

// Shutdown refuses new sessions and ends every live one.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	m.closed = true
	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range list {
		wg.Add(1)
		go func(s *Session) { defer wg.Done(); s.End(ReasonShutdown) }(s)
	}
	wg.Wait()
}
