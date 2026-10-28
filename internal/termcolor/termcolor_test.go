package termcolor

import "testing"

func TestLegacyAppleTerminal(t *testing.T) {
	env := func(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }
	apple := map[string]string{"TERM_PROGRAM": "Apple_Terminal", "TERM": "xterm-256color"}
	cases := []struct {
		name  string
		goos  string
		vars  map[string]string
		major int
		want  bool
	}{
		{"Sonoma Terminal.app", "darwin", apple, 14, true},
		{"Sequoia Terminal.app", "darwin", apple, 15, true},
		{"unknown macOS version", "darwin", apple, 0, true},
		{"Tahoe Terminal.app", "darwin", apple, 26, false},
		{"Tahoe seen through the 16.x compatibility version", "darwin", apple, 16, false},
		{"iTerm2", "darwin", map[string]string{"TERM_PROGRAM": "iTerm.app"}, 15, false},
		{"Terminal.app announcing 24-bit colour", "darwin", map[string]string{"TERM_PROGRAM": "Apple_Terminal", "COLORTERM": "truecolor"}, 15, false},
		{"Linux", "linux", apple, 0, false},
	}
	for _, c := range cases {
		if got := LegacyAppleTerminal(c.goos, env(c.vars), c.major); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
