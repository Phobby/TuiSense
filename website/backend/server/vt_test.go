//go:build linux

package server

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/thebanri/limoni/core/cell"
)

// screen is a minimal VT emulator for tests: it understands the subset
// Limoni writes (cursor position, repeat, erase, save/restore, SGR, DEC
// modes) well enough to read the text the real TuiSense draws.
type screen struct {
	mu         sync.Mutex
	cols, rows int
	cells      [][]rune
	x, y       int
	sx, sy     int
	last       rune
	pending    []byte
	modes      map[int]bool
}

func newScreen(cols, rows int) *screen {
	s := &screen{cols: cols, rows: rows, modes: map[int]bool{}}
	s.clear()
	return s
}

func (s *screen) clear() {
	s.cells = make([][]rune, s.rows)
	for i := range s.cells {
		s.cells[i] = []rune(strings.Repeat(" ", s.cols))
	}
}

func (s *screen) put(r rune) {
	w := cell.StringWidth(string(r))
	if w <= 0 {
		return
	}
	if s.y >= 0 && s.y < s.rows && s.x >= 0 && s.x < s.cols {
		s.cells[s.y][s.x] = r
		if w == 2 && s.x+1 < s.cols {
			s.cells[s.y][s.x+1] = 0
		}
	}
	s.x += w
	s.last = r
}

func params(p string, def int) []int {
	if p == "" {
		return []int{def}
	}
	var out []int
	for _, f := range strings.Split(p, ";") {
		n, err := strconv.Atoi(f)
		if err != nil {
			n = def
		}
		out = append(out, n)
	}
	return out
}

// Write feeds terminal output.
func (s *screen) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf := append(s.pending, b...)
	s.pending = nil
	i := 0
	for i < len(buf) {
		c := buf[i]
		switch {
		case c == 0x1b:
			n, ok := s.escape(buf[i:])
			if !ok {
				s.pending = append([]byte(nil), buf[i:]...)
				return len(b), nil
			}
			i += n
			continue
		case c == '\r':
			s.x = 0
		case c == '\n':
			s.y++
		case c == '\b':
			s.x = max(0, s.x-1)
		case c < 0x20:
		default:
			r, n := utf8.DecodeRune(buf[i:])
			if r == utf8.RuneError && !utf8.FullRune(buf[i:]) {
				s.pending = append([]byte(nil), buf[i:]...)
				return len(b), nil
			}
			s.put(r)
			i += n
			continue
		}
		i++
	}
	return len(b), nil
}

// escape handles one escape sequence; ok=false means it is incomplete.
func (s *screen) escape(b []byte) (int, bool) {
	if len(b) < 2 {
		return 0, false
	}
	switch b[1] {
	case '7':
		s.sx, s.sy = s.x, s.y
		return 2, true
	case '8':
		s.x, s.y = s.sx, s.sy
		return 2, true
	case ']', 'P', '_': // OSC / DCS / APC: skip to BEL or ST
		for j := 2; j < len(b); j++ {
			if b[j] == 0x07 {
				return j + 1, true
			}
			if b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\' {
				return j + 2, true
			}
		}
		return 0, false
	case '[':
	default:
		return 2, true
	}
	j := 2
	for j < len(b) && b[j] >= 0x30 && b[j] <= 0x3f {
		j++
	}
	for j < len(b) && b[j] >= 0x20 && b[j] <= 0x2f {
		j++
	}
	if j >= len(b) {
		return 0, false
	}
	raw := string(b[2:j])
	final := b[j]
	priv := strings.HasPrefix(raw, "?")
	p := strings.TrimLeft(raw, "?>=<")
	p = strings.TrimRight(p, " !\"#$%&'()*+,-./")
	switch {
	case priv && (final == 'h' || final == 'l'):
		for _, m := range params(p, 0) {
			s.modes[m] = final == 'h'
			if m == 1049 {
				s.clear()
			}
		}
	case final == 'H' || final == 'f':
		ps := params(p, 1)
		row, col := ps[0], 1
		if len(ps) > 1 {
			col = ps[1]
		}
		s.y, s.x = max(row, 1)-1, max(col, 1)-1
	case final == 'b':
		for n := params(p, 1)[0]; n > 0; n-- {
			s.put(s.last)
		}
	case final == 'K':
		if s.y >= 0 && s.y < s.rows {
			from, to := s.x, s.cols
			switch params(p, 0)[0] {
			case 1:
				from, to = 0, s.x+1
			case 2:
				from = 0
			}
			for x := max(0, from); x < min(to, s.cols); x++ {
				s.cells[s.y][x] = ' '
			}
		}
	case final == 'X':
		for x := s.x; x < min(s.x+params(p, 1)[0], s.cols); x++ {
			if s.y >= 0 && s.y < s.rows {
				s.cells[s.y][x] = ' '
			}
		}
	case final == 'J':
		if params(p, 0)[0] >= 2 {
			s.clear()
		}
	case final == 'A':
		s.y = max(0, s.y-params(p, 1)[0])
	case final == 'B':
		s.y = min(s.rows-1, s.y+params(p, 1)[0])
	case final == 'C':
		s.x = min(s.cols-1, s.x+params(p, 1)[0])
	case final == 'D':
		s.x = max(0, s.x-params(p, 1)[0])
	}
	return j + 1, true
}

// Text returns the screen as lines.
func (s *screen) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sb strings.Builder
	for _, row := range s.cells {
		for _, r := range row {
			if r != 0 {
				sb.WriteRune(r)
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Find returns the cell position (0-based) of the first occurrence of text.
func (s *screen) Find(text string) (x, y int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for y, row := range s.cells {
		var line []rune
		for _, r := range row {
			if r != 0 {
				line = append(line, r)
			}
		}
		if i := strings.Index(string(line), text); i >= 0 {
			return utf8.RuneCountInString(string(line)[:i]), y, true
		}
	}
	return 0, 0, false
}

// Mode reports a DEC private mode the application set.
func (s *screen) Mode(m int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.modes[m]
}
