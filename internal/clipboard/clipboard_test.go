package clipboard

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func fakeService(goos string, env map[string]string, installed map[string]bool, run func(context.Context, string, []string, string) error) *Service {
	return &Service{GOOS: goos, Getenv: func(k string) string { return env[k] }, LookPath: func(k string) (string, error) {
		if installed[k] {
			return k, nil
		}
		return "", errors.New("missing")
	}, Run: run}
}
func TestBackendSelectionAndExactPayload(t *testing.T) {
	for _, tc := range []struct {
		name, os  string
		env       map[string]string
		installed map[string]bool
		want      string
	}{
		{"Wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, map[string]bool{"wl-copy": true, "xclip": true}, "wl-copy"},
		{"X11", "linux", map[string]string{"DISPLAY": ":0"}, map[string]bool{"xclip": true}, "xclip"},
		{"Xsel", "linux", map[string]string{"DISPLAY": ":0"}, map[string]bool{"xsel": true}, "xsel"},
		{"macOS", "darwin", nil, map[string]bool{"pbcopy": true}, "pbcopy"},
		{"Windows", "windows", nil, map[string]bool{"powershell.exe": true}, "powershell.exe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := "0x0630 3.200%\n$(touch /tmp/never-execute); `id`\nTürkçe 🕹"
			s := fakeService(tc.os, tc.env, tc.installed, func(_ context.Context, name string, args []string, text string) error {
				if name != tc.want || text != payload {
					t.Fatalf("name=%s text=%q", name, text)
				}
				if strings.Contains(strings.Join(args, " "), payload) {
					t.Fatal("payload interpolated into arguments")
				}
				return nil
			})
			out, err := s.Copy(context.Background(), payload)
			if err != nil || out.Backend != tc.want || !out.Confirmed {
				t.Fatalf("%+v %v", out, err)
			}
		})
	}
}
func TestFailureFallsBackToNextNativeBackend(t *testing.T) {
	var tried []string
	s := fakeService("linux", map[string]string{"WAYLAND_DISPLAY": "w", "DISPLAY": ":0"}, map[string]bool{"wl-copy": true, "xclip": true, "xsel": true}, func(_ context.Context, n string, _ []string, _ string) error {
		tried = append(tried, n)
		if n != "xsel" {
			return errors.New("permission denied")
		}
		return nil
	})
	out, err := s.Copy(context.Background(), "text")
	if err != nil || out.Backend != "xsel" || len(tried) != 3 {
		t.Fatalf("%v %+v %v", tried, out, err)
	}
}
func TestUnavailableAndNativeFailureNeverConfirm(t *testing.T) {
	for _, installed := range []map[string]bool{nil, {"xclip": true}} {
		s := fakeService("linux", map[string]string{"DISPLAY": ":0"}, installed, func(context.Context, string, []string, string) error { return errors.New("blocked") })
		out, err := s.Copy(context.Background(), "message")
		if err == nil || out.Confirmed {
			t.Fatalf("%+v %v", out, err)
		}
	}
}
func TestOSC52RequiresOptInAndDoesNotClaimConfirmation(t *testing.T) {
	for _, tmux := range []string{"", "/tmp/tmux,0,0"} {
		env := map[string]string{"SSH_CONNECTION": "remote", "DISPLAY": ":10", "TERM": "xterm-256color", "TMUX": tmux}
		var b bytes.Buffer
		s := fakeService("linux", env, map[string]bool{"xclip": true}, func(context.Context, string, []string, string) error {
			t.Fatal("remote desktop should not be used")
			return nil
		})
		s.Terminal = &b
		if _, err := s.Copy(context.Background(), "hello"); err == nil || b.Len() != 0 {
			t.Fatal("OSC 52 must be opt-in")
		}
		env["TUISENSE_OSC52"] = "1"
		out, err := s.Copy(context.Background(), "hello")
		if err != nil || out.Confirmed || !strings.Contains(b.String(), "aGVsbG8=") {
			t.Fatalf("%+v %v %q", out, err, b.String())
		}
		if tmux != "" && !strings.HasPrefix(b.String(), "\x1bPtmux;\x1b\x1b]52;") {
			t.Fatal("missing tmux passthrough")
		}
		if _, err := s.Copy(context.Background(), strings.Repeat("x", 65537)); err == nil {
			t.Fatal("oversize payload silently accepted")
		}
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("closed terminal") }
func TestOSC52WriteFailure(t *testing.T) {
	s := fakeService("linux", map[string]string{"TUISENSE_OSC52": "1", "TERM": "xterm"}, nil, nil)
	s.Terminal = brokenWriter{}
	if out, err := s.Copy(context.Background(), "text"); err == nil || out.Confirmed {
		t.Fatalf("%+v %v", out, err)
	}
}
