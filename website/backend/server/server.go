//go:build linux

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/version"
	"github.com/coder/websocket"
)

// AppInfo identifies the TuiSense build the sessions run.
type AppInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

var versionLineRe = regexp.MustCompile(`^TuiSense (\S+) \(([^)]*)\)`)

// ReadAppInfo asks the configured binary for its version and commit.
func ReadAppInfo(binary string) (AppInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	cmd.Env = []string{}
	out, err := cmd.Output()
	if err != nil {
		return AppInfo{}, fmt.Errorf("%s --version: %w", binary, err)
	}
	m := versionLineRe.FindStringSubmatch(strings.TrimSpace(string(out)))
	if m == nil {
		return AppInfo{}, fmt.Errorf("unexpected version output %q", out)
	}
	info := AppInfo{Version: strings.TrimPrefix(m[1], "v")}
	if _, c, ok := strings.Cut(m[2], "commit "); ok {
		info.Commit = strings.TrimSpace(c)
	}
	return info, nil
}

// Server is the HTTP and WebSocket front of the session manager.
type Server struct {
	cfg     Config
	m       *Manager
	log     *slog.Logger
	app     AppInfo
	httpRL  *rateLimiter
	handler http.Handler
}

// New probes the sandbox, checks the binary and builds the server.
func New(cfg Config, log *slog.Logger) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return nil, err
	}
	plan, err := probeSandbox(cfg.Sandbox, self)
	if err != nil {
		return nil, err
	}
	log.Info("sandbox", "plan", plan.String())
	if !plan.Namespaces || plan.Landlock == 0 {
		log.Warn("session confinement is partial; see docs/WEB_DEMO.md (Security)", "namespaces", plan.Namespaces, "landlock_abi", plan.Landlock)
	}
	app, err := ReadAppInfo(cfg.Binary)
	if err != nil {
		return nil, err
	}
	if app.Commit != "" && version.ShortCommit() != "" && !strings.HasPrefix(app.Commit, strings.TrimSuffix(version.ShortCommit(), "-dirty")) {
		log.Warn("the TuiSense binary and the web backend were built from different commits", "app", app.Commit, "backend", version.ShortCommit())
	}
	m, err := NewManager(cfg, self, plan, log)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, m: m, log: log, app: app, httpRL: newRateLimiter(120, time.Minute)}
	s.handler = s.routes()
	return s, nil
}

// Manager exposes the session manager (tests, metrics).
func (s *Server) Manager() *Manager { return s.m }

// App is the TuiSense build served.
func (s *Server) App() AppInfo { return s.app }

// Handler is the complete HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// Run serves until ctx is cancelled, then ends every session.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	go s.m.Run(ctx)
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	s.log.Info("listening", "addr", s.cfg.Listen, "tuisense", s.app.Version, "commit", s.app.Commit)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	s.m.Shutdown()
	sctx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownWait)
	defer cancel()
	err := srv.Shutdown(sctx)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(s.cfg.Binary); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "binary missing"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "active": s.m.Active(), "max": s.cfg.MaxSessions})
	})
	mux.HandleFunc("GET /api/info", s.handleInfo)
	mux.HandleFunc("GET /api/terminal", s.handleTerminal)
	if s.cfg.StaticDir != "" {
		mux.Handle("/", spaHandler(s.cfg.StaticDir))
	}
	return s.secure(mux)
}

// secure adds security headers and a per-address request limit.
func (s *Server) secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), usb=(), hid=(), serial=(), bluetooth=(), gamepad=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self' ws: wss:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") && !s.httpRL.Allow(s.clientIP(r)) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"app":     s.app,
		"backend": AppInfo{Version: version.String(), Commit: version.CommitHash()},
		"sessions": map[string]int{
			"active": s.m.Active(),
			"max":    s.cfg.MaxSessions,
		},
		"limits": map[string]any{
			"idleTimeout": int(s.cfg.IdleTimeout.Seconds()),
			"maxDuration": int(s.cfg.MaxDuration.Seconds()),
			"minCols":     s.cfg.MinCols,
			"minRows":     s.cfg.MinRows,
			"maxCols":     s.cfg.MaxCols,
			"maxRows":     s.cfg.MaxRows,
		},
		"links": map[string]string{"github": s.cfg.PublicGitHub, "download": s.cfg.PublicDownload},
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// clientIP is the visitor's address: the TCP peer, or the last
// X-Forwarded-For hop when a trusted reverse proxy sets it.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// originAllowed accepts only the configured browser origins, or the same
// origin when none are configured. Requests without an Origin header (not
// from a browser page) are refused unless explicitly allowed for tests.
func (s *Server) originAllowed(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return s.cfg.AllowNoOrigin
	}
	u, err := url.Parse(o)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if len(s.cfg.AllowedOrigins) == 0 {
		return strings.EqualFold(u.Host, r.Host)
	}
	origin := strings.ToLower(u.Scheme + "://" + u.Host)
	for _, a := range s.cfg.AllowedOrigins {
		if strings.ToLower(strings.TrimRight(a, "/")) == origin {
			return true
		}
	}
	return false
}

// startMessage is the browser's first message on a terminal connection.
type startMessage struct {
	Type   string `json:"type"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	Lang   string `json:"lang"`
	Resume *struct {
		Session string `json:"session"`
		Token   string `json:"token"`
	} `json:"resume,omitempty"`
}

// controlMessage is any later text message from the browser.
type controlMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

const maxMessage = 64 << 10

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	if !s.originAllowed(r) {
		s.log.Warn("websocket origin refused", "origin", r.Header.Get("Origin"))
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // the origin was verified above
		CompressionMode:    websocket.CompressionContextTakeover,
	})
	if err != nil {
		return
	}
	ws.SetReadLimit(maxMessage)
	c := newClient(ws)
	defer c.close(websocket.StatusNormalClosure, "")
	ip := s.clientIP(r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	typ, data, err := ws.Read(ctx)
	cancel()
	var start startMessage
	if err != nil || typ != websocket.MessageText || json.Unmarshal(data, &start) != nil || start.Type != "start" {
		c.close(websocket.StatusPolicyViolation, "expected a start message")
		return
	}

	var sess *Session
	resumed := false
	if start.Resume != nil && sessionIDRe.MatchString(start.Resume.Session) {
		if found, ok := s.m.Resume(start.Resume.Session, start.Resume.Token); ok {
			sess, resumed = found, true
		}
	}
	if sess == nil {
		sess, err = s.m.Create(ip, start.Lang, start.Cols, start.Rows)
		if err != nil {
			code := "error"
			switch {
			case errors.Is(err, ErrCapacity):
				code = "capacity"
			case errors.Is(err, ErrPerIP):
				code = "per_ip"
			case errors.Is(err, ErrRateLimited):
				code = "rate_limited"
			case errors.Is(err, ErrClosed):
				code = "shutdown"
			default:
				s.log.Error("session start failed", "err", err)
				err = errors.New("the demo could not be started")
			}
			_ = c.sendJSON(map[string]string{"type": "error", "code": code, "message": err.Error()})
			c.close(websocket.StatusTryAgainLater, code)
			return
		}
	}
	if err := sess.attach(c, resumed, start.Cols, start.Rows); err != nil {
		return
	}
	defer sess.detach(c)

	// Keep-alive: a dead connection is detected within a minute.
	go func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-c.closed:
				return
			case <-t.C:
				pctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				err := ws.Ping(pctx)
				cancel()
				if err != nil {
					c.close(websocket.StatusGoingAway, "ping timeout")
					return
				}
			}
		}
	}()

	for {
		typ, data, err := ws.Read(context.Background())
		if err != nil {
			return
		}
		switch typ {
		case websocket.MessageBinary:
			sess.Input(data)
		case websocket.MessageText:
			var msg controlMessage
			if json.Unmarshal(data, &msg) != nil {
				continue
			}
			switch msg.Type {
			case "resize":
				sess.Resize(msg.Cols, msg.Rows)
			case "restart", "reset":
				if err := sess.Restart(msg.Type == "reset"); err != nil {
					_ = c.sendJSON(map[string]string{"type": "notice", "message": err.Error()})
				}
			case "close":
				sess.End(ReasonClosed)
				return
			}
		}
	}
}

// spaHandler serves the built frontend; unknown paths without an
// extension get index.html (client-side routes such as /demo).
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Clean("/" + r.URL.Path)
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil && filepath.Ext(p) == "" {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		if strings.HasPrefix(p, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fs.ServeHTTP(w, r)
	})
}
