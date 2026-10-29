//go:build linux

package server

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SandboxMode selects how strictly session processes are confined.
type SandboxMode string

const (
	// SandboxAuto applies every confinement the kernel allows and logs
	// what it could not apply.
	SandboxAuto SandboxMode = "auto"
	// SandboxStrict refuses to start sessions unless both Linux namespaces
	// and Landlock are available.
	SandboxStrict SandboxMode = "strict"
	// SandboxLandlock requires Landlock (filesystem, network and signal
	// confinement) and uses namespaces when available. This is the mode
	// for container platforms whose seccomp profile blocks namespaces.
	SandboxLandlock SandboxMode = "landlock"
	// SandboxMinimal applies only resource limits and no_new_privs. For
	// local development only.
	SandboxMinimal SandboxMode = "minimal"
)

// Config holds every tunable of the web demo backend. All values come from
// the operator (environment variables); nothing in it is ever taken from a
// visitor's request.
type Config struct {
	Listen string // address to listen on, e.g. ":8080"
	// Binary is the absolute path of the tuisense executable to run.
	Binary string
	// SessionRoot holds one private temporary directory per session.
	SessionRoot string
	// StaticDir optionally serves the built frontend ("" = API only).
	StaticDir string
	// AllowedOrigins are the exact browser origins allowed to open a
	// terminal (scheme://host[:port]). Empty means same-origin only.
	AllowedOrigins []string
	// TrustProxy uses the last X-Forwarded-For hop as the client address;
	// enable it only behind a reverse proxy that sets the header.
	TrustProxy bool

	MaxSessions      int           // concurrent sessions on this server
	MaxSessionsPerIP int           // concurrent sessions per client address
	IdleTimeout      time.Duration // no keyboard or mouse input
	MaxDuration      time.Duration // hard cap per session
	ReconnectGrace   time.Duration // a session survives a dropped connection this long
	StartsPerMinute  int           // new sessions per client address per minute
	InputBytesPerSec int           // keyboard/mouse/paste bytes per session per second

	MemoryLimitMB  int // RLIMIT_AS (address space) of a session process
	SoftMemoryMB   int // GOMEMLIMIT of a session process (garbage-collector target)
	CPUSeconds     int // RLIMIT_CPU of a session process
	MaxOpenFiles   int // RLIMIT_NOFILE of a session process
	MaxFileSizeMB  int // RLIMIT_FSIZE of a session process
	GoMaxProcs     int // GOMAXPROCS of a session process (CPU parallelism)
	Nice           int // scheduling niceness of a session process
	FPS            int // TuiSense frame rate
	MinCols        int
	MinRows        int
	MaxCols        int
	MaxRows        int
	Sandbox        SandboxMode
	ShutdownWait   time.Duration
	AllowNoOrigin  bool // accept WebSocket requests without an Origin header (tests only)
	PublicGitHub   string
	PublicDownload string
}

// DefaultConfig returns the production defaults.
func DefaultConfig() Config {
	return Config{
		Listen:           ":8080",
		Binary:           "/usr/local/bin/tuisense",
		SessionRoot:      filepath.Join(os.TempDir(), "tuisense-web"),
		MaxSessions:      24,
		MaxSessionsPerIP: 3,
		IdleTimeout:      10 * time.Minute,
		MaxDuration:      30 * time.Minute,
		ReconnectGrace:   45 * time.Second,
		StartsPerMinute:  6,
		InputBytesPerSec: 32 * 1024,
		MemoryLimitMB:    1024,
		SoftMemoryMB:     160,
		CPUSeconds:       600,
		MaxOpenFiles:     64,
		MaxFileSizeMB:    8,
		GoMaxProcs:       1,
		Nice:             10,
		FPS:              30,
		MinCols:          40,
		MinRows:          12,
		MaxCols:          320,
		MaxRows:          120,
		Sandbox:          SandboxAuto,
		ShutdownWait:     5 * time.Second,
		PublicGitHub:     "https://github.com/Phobby/TuiSense",
		PublicDownload:   "https://github.com/Phobby/TuiSense/releases/latest",
	}
}

// ConfigFromEnv reads TUISENSE_WEB_* variables over the defaults.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	c := DefaultConfig()
	var errs []error
	str := func(name string, dst *string) {
		if v := strings.TrimSpace(getenv("TUISENSE_WEB_" + name)); v != "" {
			*dst = v
		}
	}
	num := func(name string, dst *int) {
		if v := strings.TrimSpace(getenv("TUISENSE_WEB_" + name)); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("TUISENSE_WEB_%s: %w", name, err))
				return
			}
			*dst = n
		}
	}
	dur := func(name string, dst *time.Duration) {
		if v := strings.TrimSpace(getenv("TUISENSE_WEB_" + name)); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("TUISENSE_WEB_%s: %w", name, err))
				return
			}
			*dst = d
		}
	}
	boolean := func(name string, dst *bool) {
		if v := strings.TrimSpace(getenv("TUISENSE_WEB_" + name)); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("TUISENSE_WEB_%s: %w", name, err))
				return
			}
			*dst = b
		}
	}
	str("LISTEN", &c.Listen)
	str("BINARY", &c.Binary)
	str("SESSION_ROOT", &c.SessionRoot)
	str("STATIC_DIR", &c.StaticDir)
	if v := strings.TrimSpace(getenv("TUISENSE_WEB_ALLOWED_ORIGINS")); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				c.AllowedOrigins = append(c.AllowedOrigins, o)
			}
		}
	}
	boolean("TRUST_PROXY", &c.TrustProxy)
	num("MAX_SESSIONS", &c.MaxSessions)
	num("MAX_SESSIONS_PER_IP", &c.MaxSessionsPerIP)
	dur("IDLE_TIMEOUT", &c.IdleTimeout)
	dur("MAX_DURATION", &c.MaxDuration)
	dur("RECONNECT_GRACE", &c.ReconnectGrace)
	num("STARTS_PER_MINUTE", &c.StartsPerMinute)
	num("INPUT_BYTES_PER_SEC", &c.InputBytesPerSec)
	num("MEMORY_LIMIT_MB", &c.MemoryLimitMB)
	num("SOFT_MEMORY_MB", &c.SoftMemoryMB)
	num("CPU_SECONDS", &c.CPUSeconds)
	num("MAX_OPEN_FILES", &c.MaxOpenFiles)
	num("MAX_FILE_SIZE_MB", &c.MaxFileSizeMB)
	num("GOMAXPROCS", &c.GoMaxProcs)
	num("NICE", &c.Nice)
	num("FPS", &c.FPS)
	var sandbox string
	str("SANDBOX", &sandbox)
	if sandbox != "" {
		c.Sandbox = SandboxMode(strings.ToLower(sandbox))
	}
	dur("SHUTDOWN_WAIT", &c.ShutdownWait)
	str("PUBLIC_GITHUB_URL", &c.PublicGitHub)
	str("PUBLIC_DOWNLOAD_URL", &c.PublicDownload)
	if len(errs) > 0 {
		return c, errors.Join(errs...)
	}
	return c, c.Validate()
}

// Validate checks the configuration and the TuiSense binary.
func (c Config) Validate() error {
	var errs []error
	if !filepath.IsAbs(c.Binary) {
		errs = append(errs, fmt.Errorf("binary %q must be an absolute path", c.Binary))
	} else if st, err := os.Stat(c.Binary); err != nil {
		errs = append(errs, fmt.Errorf("binary: %w", err))
	} else if !st.Mode().IsRegular() || st.Mode().Perm()&0o111 == 0 {
		errs = append(errs, fmt.Errorf("binary %q is not an executable file", c.Binary))
	}
	if !filepath.IsAbs(c.SessionRoot) {
		errs = append(errs, fmt.Errorf("session root %q must be an absolute path", c.SessionRoot))
	}
	for _, o := range c.AllowedOrigins {
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != "" || u.RawQuery != "" {
			errs = append(errs, fmt.Errorf("allowed origin %q must look like https://demo.example.com", o))
		}
	}
	positive := map[string]int{
		"MAX_SESSIONS": c.MaxSessions, "MAX_SESSIONS_PER_IP": c.MaxSessionsPerIP, "STARTS_PER_MINUTE": c.StartsPerMinute,
		"INPUT_BYTES_PER_SEC": c.InputBytesPerSec, "MEMORY_LIMIT_MB": c.MemoryLimitMB, "SOFT_MEMORY_MB": c.SoftMemoryMB, "CPU_SECONDS": c.CPUSeconds,
		"MAX_OPEN_FILES": c.MaxOpenFiles, "MAX_FILE_SIZE_MB": c.MaxFileSizeMB, "GOMAXPROCS": c.GoMaxProcs, "FPS": c.FPS,
	}
	for name, v := range positive {
		if v <= 0 {
			errs = append(errs, fmt.Errorf("TUISENSE_WEB_%s must be positive", name))
		}
	}
	// RLIMIT_AS limits address space, not memory in use: the Go runtime
	// reserves several hundred MiB of address space at start-up.
	if c.MemoryLimitMB < 768 {
		errs = append(errs, errors.New("TUISENSE_WEB_MEMORY_LIMIT_MB below 768 cannot start the Go runtime (it limits address space)"))
	}
	if c.SoftMemoryMB >= c.MemoryLimitMB {
		errs = append(errs, errors.New("TUISENSE_WEB_SOFT_MEMORY_MB must be below TUISENSE_WEB_MEMORY_LIMIT_MB"))
	}
	if c.Nice < 0 || c.Nice > 19 {
		errs = append(errs, errors.New("TUISENSE_WEB_NICE must be between 0 and 19"))
	}
	if c.IdleTimeout <= 0 || c.MaxDuration <= 0 || c.ReconnectGrace <= 0 {
		errs = append(errs, errors.New("timeouts must be positive"))
	}
	if c.MinCols < 20 || c.MinRows < 10 || c.MaxCols < c.MinCols || c.MaxRows < c.MinRows || c.MaxCols > 1000 || c.MaxRows > 500 {
		errs = append(errs, errors.New("invalid terminal size bounds"))
	}
	switch c.Sandbox {
	case SandboxAuto, SandboxStrict, SandboxLandlock, SandboxMinimal:
	default:
		errs = append(errs, fmt.Errorf("unknown sandbox mode %q (auto, strict, landlock, minimal)", c.Sandbox))
	}
	return errors.Join(errs...)
}
