//go:build linux

package server

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// sessionDirs are the private directories of one session.
type sessionDirs struct {
	Root, Home, Config, Data, Cache, Tmp string
}

func newSessionDirs(root string) sessionDirs {
	return sessionDirs{
		Root:   root,
		Home:   filepath.Join(root, "home"),
		Config: filepath.Join(root, "config"),
		Data:   filepath.Join(root, "data"),
		Cache:  filepath.Join(root, "cache"),
		Tmp:    filepath.Join(root, "tmp"),
	}
}

// create makes every directory, private to the server's user.
func (d sessionDirs) create() error {
	for _, p := range []string{d.Root, d.Home, d.Config, d.Data, d.Cache, d.Tmp} {
		if err := os.Mkdir(p, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return nil
}

// processSpec describes one TuiSense run.
type processSpec struct {
	Self    string // the backend binary (sandbox helper)
	Binary  string // tuisense
	Dirs    sessionDirs
	Lang    string // "en" or "tr"
	Cols    uint16
	Rows    uint16
	Plan    sandboxPlan
	Config  Config
	Timeout time.Duration // how long the helper may take to report
}

// env is the complete environment of a session process: nothing of the
// server's own environment (secrets, credentials, paths) is passed on.
func (s processSpec) env() []string {
	locale := "C.UTF-8"
	if s.Lang == "tr" {
		locale = "tr_TR.UTF-8"
	}
	return []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=" + locale,
		"HOME=" + s.Dirs.Home,
		"XDG_CONFIG_HOME=" + s.Dirs.Config,
		"XDG_DATA_HOME=" + s.Dirs.Data,
		"XDG_CACHE_HOME=" + s.Dirs.Cache,
		"XDG_STATE_HOME=" + s.Dirs.Data,
		"TMPDIR=" + s.Dirs.Tmp,
		"GOMAXPROCS=" + strconv.Itoa(s.Config.GoMaxProcs),
		// The collector's target for memory in use; RLIMIT_AS is the hard
		// ceiling on address space.
		"GOMEMLIMIT=" + strconv.Itoa(s.Config.SoftMemoryMB) + "MiB",
		"GOTRACEBACK=none",
	}
}

func (s processSpec) argv() []string {
	return []string{
		"tuisense", "--web-demo",
		"--config-dir", filepath.Join(s.Dirs.Config, "tuisense"),
		"--profile-dir", filepath.Join(s.Dirs.Config, "tuisense", "profiles"),
		"--fps", strconv.Itoa(s.Config.FPS),
	}
}

// Process is one running TuiSense attached to a pseudo-terminal.
type Process struct {
	cmd  *exec.Cmd
	pty  *os.File
	done chan struct{}

	mu       sync.Mutex
	exitCode int
	exitErr  error
	stopped  bool
}

// startProcess launches tuisense through the sandbox helper. There is no
// shell anywhere: the helper is the backend binary itself, and it executes
// the configured tuisense path with a fixed argument vector.
func startProcess(spec processSpec) (*Process, error) {
	h := helperArgs{
		Landlock: spec.Plan.Landlock,
		MemBytes: uint64(spec.Config.MemoryLimitMB) << 20,
		CPUSec:   uint64(spec.Config.CPUSeconds),
		NoFile:   uint64(spec.Config.MaxOpenFiles),
		FSize:    uint64(spec.Config.MaxFileSizeMB) << 20,
		Nice:     spec.Config.Nice,
		RW:       spec.Dirs.Root,
		Binary:   spec.Binary,
		TuiSense: spec.argv(),
	}
	cmd := &exec.Cmd{
		Path: spec.Self,
		Args: h.argv(spec.Self),
		Env:  spec.env(),
		Dir:  spec.Dirs.Home,
		SysProcAttr: &syscall.SysProcAttr{
			Setsid:    true,
			Setctty:   true,
			Pdeathsig: syscall.SIGKILL, // never outlive the server
		},
	}
	if spec.Plan.Namespaces {
		namespaceAttrs(cmd.SysProcAttr)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	cmd.ExtraFiles = []*os.File{w}
	ptmx, err := pty.StartWithAttrs(cmd, &pty.Winsize{Cols: spec.Cols, Rows: spec.Rows}, cmd.SysProcAttr)
	w.Close()
	if err != nil {
		return nil, fmt.Errorf("start session process: %w", err)
	}
	p := &Process{cmd: cmd, pty: ptmx, done: make(chan struct{})}
	go p.wait()
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	status, err := readStatus(r, timeout)
	if err == nil && len(status) >= 2 && status[:2] != "ok" {
		err = errors.New(status)
	}
	if err != nil {
		p.Stop(0)
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	return p, nil
}

func (p *Process) wait() {
	err := p.cmd.Wait()
	p.mu.Lock()
	p.exitErr = err
	if p.cmd.ProcessState != nil {
		p.exitCode = p.cmd.ProcessState.ExitCode()
	}
	p.mu.Unlock()
	close(p.done)
}

// Pid is the process id in the server's PID namespace.
func (p *Process) Pid() int { return p.cmd.Process.Pid }

// Done is closed when the process has exited and been reaped.
func (p *Process) Done() <-chan struct{} { return p.done }

// ExitCode is the exit status once Done is closed (-1 = killed by a signal).
func (p *Process) ExitCode() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode
}

// Read reads terminal output.
func (p *Process) Read(b []byte) (int, error) { return p.pty.Read(b) }

// Write sends keyboard and mouse input to the terminal.
func (p *Process) Write(b []byte) (int, error) { return p.pty.Write(b) }

// Resize changes the terminal size; the kernel signals SIGWINCH to TuiSense.
func (p *Process) Resize(cols, rows uint16) error {
	return pty.Setsize(p.pty, &pty.Winsize{Cols: cols, Rows: rows})
}

// Stop terminates the whole process group: SIGTERM, then SIGKILL after
// grace. In a PID namespace the session process is its init, so every
// process it could have started dies with it. Stop waits for the exit and
// releases the pseudo-terminal; calling it again is a no-op.
func (p *Process) Stop(grace time.Duration) {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		<-p.done
		return
	}
	p.stopped = true
	p.mu.Unlock()
	pid := p.cmd.Process.Pid
	select {
	case <-p.done:
	default:
		if grace > 0 {
			_ = syscall.Kill(-pid, syscall.SIGTERM)
			select {
			case <-p.done:
			case <-time.After(grace):
			}
		}
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = p.cmd.Process.Kill()
		<-p.done
	}
	_ = p.pty.Close()
}
