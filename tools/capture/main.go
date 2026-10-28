// Command capture records README demonstrations from the real TuiSense UI.
//
// It runs the application headlessly with the built-in simulated DualSense
// (and, where a scene needs it, the simulated Xbox test device), feeds it
// scripted keyboard and mouse input, and records every drawn frame — the
// exact characters and colours TuiSense produced — as JSON lines. render.py
// turns them into GIFs. The simulated device is labelled "MOCK" in every
// frame; these captures are not hardware demonstrations.
//
//	go run ./tools/capture -out bin/frames       # all scenes (or: make media)
//	python3 tools/capture/render.py bin/frames docs/media
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/app"
	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/firmware"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/version"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/testkit"
)

const fps = 12

// Default capture size (cells); scenes may use their own.
const defCols, defRows = 110, 32

// segment is a run of cells with the same colours.
type segment struct {
	Text string `json:"t"`
	Fg   string `json:"f"`
	Bg   string `json:"b"`
	Bold bool   `json:"o,omitempty"`
}

type frameRec struct {
	Rows [][]segment `json:"rows"`
}

// session drives one application instance.
type session struct {
	cols   uint16
	rows   uint16
	a      *app.Application
	term   *testkit.Terminal
	out    *bufio.Writer
	frames int
	cancel context.CancelFunc
}

func newSession(testDevices bool, mode state.UXMode) *session {
	return newSized(defCols, defRows, testDevices, mode)
}

func newSized(cols, rows uint16, testDevices bool, mode state.UXMode) *session {
	// No firmware providers: scenes stay offline unless they opt in.
	return newWith(cols, rows, testDevices, mode, []firmware.Provider{})
}

// newWith builds a session; nil providers selects TuiSense's real, read-only
// firmware metadata checkers (network access).
func newWith(cols, rows uint16, testDevices bool, mode state.UXMode, providers []firmware.Provider) *session {
	tmp, _ := os.MkdirTemp("", "tuisense-capture-")
	reg := drivers.NewBuiltinRegistry(drivers.RegistryOptions{Mock: true, TestDevices: testDevices})
	a := app.New(app.Options{ForceMock: true, AllowMock: true, Registry: reg, Language: "en",
		ProfileDir: filepath.Join(tmp, "profiles"), ConfigDir: filepath.Join(tmp, "config"), ReportDir: filepath.Join(tmp, "reports"),
		Version: version.String(), FirmwareProviders: providers,
		CopyText: func(string) error { return nil }})
	a.State.SetMode(mode)
	a.State.AutoRotate = false
	ctx, cancel := context.WithCancel(context.Background())
	go a.Manager.Start(ctx)
	go a.Dispatcher.Start(ctx)
	return &session{cols: cols, rows: rows, a: a, term: testkit.NewTerminal(cols, rows), cancel: cancel}
}

func (s *session) draw(ev *driver.Event) {
	s.term.Draw(func(f *terminal.Frame) { s.a.Draw(f, ev) })
}

func hex(c cell.Color, def string) string {
	if c.Type() != cell.ColorRGB {
		return def
	}
	r, g, b := c.RGB()
	return fmt.Sprintf("%02x%02x%02x", r, g, b)
}

// snapshot records the current buffer.
func (s *session) snapshot() frameRec {
	f := s.term.Frame()
	var rec frameRec
	for y := uint16(0); y < s.rows; y++ {
		var line []segment
		for x := uint16(0); x < s.cols; x++ {
			c := f.Buffer.Get(x, y)
			ch, fg, bg, bold := " ", "dcdfe6", "11131a", false
			if c != nil {
				if c.Content != 0 {
					ch = string(c.Content)
				} else {
					ch = ""
				}
				fg, bg = hex(c.Style.Fg, fg), hex(c.Style.Bg, bg)
				bold = c.Style.Modifier&cell.ModifierBold != 0
			}
			if n := len(line); n > 0 && line[n-1].Fg == fg && line[n-1].Bg == bg && line[n-1].Bold == bold {
				line[n-1].Text += ch
			} else {
				line = append(line, segment{ch, fg, bg, bold})
			}
		}
		rec.Rows = append(rec.Rows, line)
	}
	return rec
}

// wait keeps drawing at the capture frame rate, recording frames.
func (s *session) wait(d time.Duration, rec func(frameRec)) {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		s.draw(nil)
		rec(s.snapshot())
		time.Sleep(time.Second / fps)
	}
}

func (s *session) key(ch rune) {
	s.draw(&driver.Event{Type: driver.EventKey, Key: driver.KeyEvent{Type: driver.KeyRune, Ch: ch}})
}
func (s *session) special(t driver.KeyType) {
	s.draw(&driver.Event{Type: driver.EventKey, Key: driver.KeyEvent{Type: t}})
}

// node finds an accessibility node's position in the last frame.
func (s *session) node(id string) (uint16, uint16, bool) {
	for _, n := range s.term.AccessibilityTree() {
		if n.ID == id {
			return n.Bounds.X + n.Bounds.Width/2, n.Bounds.Y, true
		}
	}
	return 0, 0, false
}

func (s *session) hover(id string) {
	if x, y, ok := s.node(id); ok {
		// A real terminal routes pointer motion like any mouse event.
		s.term.Mouse(driver.MouseEvent{Button: driver.MouseNone, X: x, Y: y})
		s.draw(nil)
	}
}

func (s *session) click(id string) {
	if x, y, ok := s.node(id); ok {
		s.term.Click(x, y)
		s.draw(nil)
	}
}

// drag rotates the stage with the mouse, step by step.
func (s *session) drag(fromX, fromY uint16, dx int, rec func(frameRec)) {
	s.term.Mouse(driver.MouseEvent{Button: driver.MouseLeft, X: fromX, Y: fromY})
	x := int(fromX)
	step := 2
	if dx < 0 {
		step = -2
	}
	for moved := 0; moved != dx; moved += step {
		x += step
		s.term.Mouse(driver.MouseEvent{Button: driver.MouseLeft, Drag: true, X: uint16(x), Y: fromY})
		s.draw(nil)
		rec(s.snapshot())
		time.Sleep(time.Second / fps)
	}
	s.term.Mouse(driver.MouseEvent{Button: driver.MouseRelease, X: uint16(x), Y: fromY})
}

type recorder struct {
	w *bufio.Writer
	n int
}

func (r *recorder) add(f frameRec) {
	b, _ := json.Marshal(f)
	r.w.Write(b)
	r.w.WriteByte('\n')
	r.n++
}

func record(dir, name string, scene func(rec func(frameRec))) {
	path := filepath.Join(dir, name+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	r := &recorder{w: bufio.NewWriter(f)}
	scene(r.add)
	r.w.Flush()
	f.Close()
	fmt.Printf("%s: %d frames\n", path, r.n)
}

func main() {
	out := flag.String("out", "bin/frames", "output directory for frame files")
	only := flag.String("scene", "", "record only this scene")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		panic(err)
	}
	for _, sc := range scenes {
		if *only != "" && !strings.EqualFold(*only, sc.name) {
			continue
		}
		record(*out, sc.name, sc.run)
	}
}
