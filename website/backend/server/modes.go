//go:build linux

package server

import (
	"strconv"
	"strings"
)

// replayModes are the DEC private modes TuiSense (Limoni) sets at start-up:
// alternate screen, cursor, auto-wrap, mouse reporting, focus events and
// bracketed paste. A browser that reconnects to a running session gets a
// fresh terminal, so these are replayed before the screen is redrawn.
var replayModes = []int{1049, 25, 7, 1000, 1002, 1003, 1005, 1006, 1015, 1004, 2004}

// modeTracker follows DECSET/DECRST (CSI ? Pm h / CSI ? Pm l) in the
// terminal output, across read boundaries.
type modeTracker struct {
	state map[int]bool
	tail  []byte // an unfinished escape sequence from the previous chunk
}

func newModeTracker() *modeTracker { return &modeTracker{state: map[int]bool{}} }

// Feed scans a chunk of terminal output.
func (m *modeTracker) Feed(b []byte) {
	buf := b
	if len(m.tail) > 0 {
		buf = append(m.tail, b...)
		m.tail = nil
	}
	for i := 0; i < len(buf); i++ {
		if buf[i] != 0x1b {
			continue
		}
		if i+1 >= len(buf) {
			m.keep(buf[i:])
			return
		}
		if buf[i+1] != '[' {
			continue
		}
		if i+2 >= len(buf) {
			m.keep(buf[i:])
			return
		}
		if buf[i+2] != '?' {
			continue
		}
		j := i + 3
		for j < len(buf) && (buf[j] >= '0' && buf[j] <= '9' || buf[j] == ';') {
			j++
		}
		if j >= len(buf) {
			m.keep(buf[i:])
			return
		}
		if final := buf[j]; final == 'h' || final == 'l' {
			for _, f := range strings.Split(string(buf[i+3:j]), ";") {
				if n, err := strconv.Atoi(f); err == nil {
					m.state[n] = final == 'h'
				}
			}
		}
		i = j
	}
}

func (m *modeTracker) keep(b []byte) {
	if len(b) <= 64 { // longer is not a mode sequence
		m.tail = append([]byte(nil), b...)
	}
}

// Replay returns the sequence that puts a fresh terminal into the modes
// seen so far.
func (m *modeTracker) Replay() []byte {
	var sb strings.Builder
	for _, mode := range replayModes {
		on, seen := m.state[mode]
		if !seen {
			continue
		}
		sb.WriteString("\x1b[?" + strconv.Itoa(mode))
		if on {
			sb.WriteByte('h')
		} else {
			sb.WriteByte('l')
		}
	}
	return []byte(sb.String())
}
