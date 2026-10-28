// Package clipboard provides platform clipboard adapters independent of device drivers.
package clipboard

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Outcome distinguishes an acknowledged native copy from an unacknowledged terminal request.
type Outcome struct {
	Backend   string
	Confirmed bool
}
type ClipboardService interface {
	Copy(context.Context, string) (Outcome, error)
}
type candidate struct {
	name string
	args []string
}

// Service permits deterministic backend selection and failure tests without a desktop.
type Service struct {
	GOOS     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
	Run      func(context.Context, string, []string, string) error
	Terminal io.Writer
}

func New() *Service {
	var terminal io.Writer
	if stat, err := os.Stdout.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
		terminal = os.Stdout
	}
	return &Service{GOOS: runtime.GOOS, Getenv: os.Getenv, LookPath: exec.LookPath, Terminal: terminal,
		Run: func(ctx context.Context, path string, args []string, text string) error {
			cmd := exec.CommandContext(ctx, path, args...)
			cmd.Stdin = strings.NewReader(text)
			// Clipboard owners may fork and retain descriptors. Do not attach output pipes.
			return cmd.Run()
		}}
}

func (s *Service) candidates() []candidate {
	switch s.GOOS {
	case "darwin":
		return []candidate{{"pbcopy", nil}}
	case "windows":
		return []candidate{{"powershell.exe", []string{"-NoProfile", "-NonInteractive", "-STA", "-Command", "[Console]::InputEncoding = [Text.UTF8Encoding]::new(); $text = [Console]::In.ReadToEnd(); Set-Clipboard -Value $text -ErrorAction Stop"}}}
	case "linux", "freebsd", "openbsd", "netbsd":
		// Under SSH, prefer the local terminal clipboard over a remote X server.
		if s.Getenv("SSH_CONNECTION") != "" || s.Getenv("SSH_TTY") != "" {
			return nil
		}
		var out []candidate
		if s.Getenv("WAYLAND_DISPLAY") != "" {
			out = append(out, candidate{"wl-copy", []string{"--type", "text/plain;charset=utf-8"}})
		}
		if s.Getenv("DISPLAY") != "" {
			out = append(out, candidate{"xclip", []string{"-selection", "clipboard", "-in"}}, candidate{"xsel", []string{"--clipboard", "--input"}})
		}
		return out
	}
	return nil
}

func (s *Service) Copy(ctx context.Context, text string) (Outcome, error) {
	var failures []error
	for _, c := range s.candidates() {
		path, err := s.LookPath(c.name)
		if err != nil {
			continue
		}
		attempt, cancel := context.WithTimeout(ctx, 2*time.Second)
		err = s.Run(attempt, path, c.args, text)
		cancel()
		if err == nil {
			return Outcome{c.name, true}, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", c.name, err))
	}
	// OSC 52 cannot acknowledge clipboard writes. Require explicit opt-in rather
	// than claiming success from a write to an arbitrary stdout or terminal.
	if s.Getenv("TUISENSE_OSC52") == "1" && s.Getenv("TERM") != "dumb" && s.Terminal != nil {
		if len(text) > 64*1024 {
			return Outcome{}, errors.New("text exceeds the OSC 52 size limit; use a native clipboard backend")
		}
		seq := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
		if s.Getenv("TMUX") != "" {
			seq = "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
		}
		n, err := io.WriteString(s.Terminal, seq)
		if err != nil {
			return Outcome{}, err
		}
		if n != len(seq) {
			return Outcome{}, io.ErrShortWrite
		}
		return Outcome{"OSC 52", false}, nil
	}
	failures = append(failures, errors.New("clipboard unavailable: install wl-clipboard, xclip or xsel, or enable terminal clipboard support with TUISENSE_OSC52=1"))
	return Outcome{}, errors.Join(failures...)
}

// Available lists the clipboard tools found on this system (in preference
// order). Terminal clipboard support (OSC 52) is not detectable here.
func (s *Service) Available() []string {
	var out []string
	for _, c := range s.candidates() {
		if _, err := s.LookPath(c.name); err == nil {
			out = append(out, c.name)
		}
	}
	return out
}
