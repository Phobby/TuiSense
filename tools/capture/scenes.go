package main

import (
	"time"

	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/driver"
)

type scene struct {
	name string
	run  func(rec func(frameRec))
}

const sec = time.Second

var scenes = []scene{
	{"dashboard", func(rec func(frameRec)) {
		s := newSession(false, state.ModeEZ)
		defer s.cancel()
		s.a.State.AutoRotate = true
		s.wait(3*sec, rec)
		s.hover("component_btn_cross")
		s.wait(1500*time.Millisecond, rec)
		s.hover("component_left_stick")
		s.wait(1500*time.Millisecond, rec)
		s.hover("component_touchpad")
		s.wait(2*sec, rec)
	}},
	{"controller-3d", func(rec func(frameRec)) {
		s := newSession(false, state.ModeAdvanced)
		defer s.cancel()
		s.wait(800*time.Millisecond, rec)
		s.drag(55, 16, 24, rec)
		s.drag(79, 16, -36, rec)
		s.key('r')
		s.wait(600*time.Millisecond, rec)
		s.key('G') // Controller Gallery
		for s.a.State.StageModelID() != "xbox_elite2" {
			s.special(driver.KeyArrowRight)
			s.wait(500*time.Millisecond, rec)
		}
		s.key('v') // rear view
		s.wait(1500*time.Millisecond, rec)
		s.hover("component_paddle_p1")
		s.wait(1200*time.Millisecond, rec)
		s.click("component_paddle_p1")
		s.wait(2*sec, rec)
		s.key('v')
		s.wait(1200*time.Millisecond, rec)
	}},
	{"customization", func(rec func(frameRec)) {
		// Wide enough for the controller to stay beside the settings.
		s := newSized(144, 32, false, state.ModeEZ)
		defer s.cancel()
		s.wait(700*time.Millisecond, rec)
		s.click("home_customize")
		s.wait(700*time.Millisecond, rec)
		s.click("nav_lighting")
		s.wait(900*time.Millisecond, rec)
		for _, id := range []string{"ez_light_1", "ez_light_3", "ez_light_5", "ez_light_2"} {
			s.click(id)
			s.wait(800*time.Millisecond, rec)
		}
		s.click("nav_triggers")
		s.wait(900*time.Millisecond, rec)
		s.click("ez_trig_Weapon")
		s.wait(1200*time.Millisecond, rec)
		s.click("nav_haptics")
		s.wait(900*time.Millisecond, rec)
		s.click("ez_test")
		s.wait(1200*time.Millisecond, rec)
	}},
	{"diagnostics", func(rec func(frameRec)) {
		s := newSession(false, state.ModeAdvanced)
		defer s.cancel()
		s.key('2')
		s.wait(800*time.Millisecond, rec)
		s.click("diag_tab_sticks")
		s.wait(600*time.Millisecond, rec)
		s.click("stick_start")
		s.wait(4*sec, rec)
		s.click("diag_tab_triggers")
		s.wait(500*time.Millisecond, rec)
		s.click("triggers_start")
		s.wait(3*sec, rec)
		s.click("diag_tab_buttons")
		s.wait(2500*time.Millisecond, rec)
	}},
	{"controller-switching", func(rec func(frameRec)) {
		s := newSession(true, state.ModeEZ)
		defer s.cancel()
		s.wait(1200*time.Millisecond, rec)
		s.key('c')
		s.wait(1000*time.Millisecond, rec)
		s.special(driver.KeyArrowDown)
		s.wait(600*time.Millisecond, rec)
		s.special(driver.KeyEnter)
		s.wait(2*sec, rec)
		s.key('.')
		s.wait(2*sec, rec)
		s.key('G')
		s.wait(1500*time.Millisecond, rec)
		s.special(driver.KeyArrowRight)
		s.wait(1500*time.Millisecond, rec)
		s.special(driver.KeyEsc)
		s.wait(1200*time.Millisecond, rec)
	}},
	{"health-report", func(rec func(frameRec)) {
		s := newSession(false, state.ModeEZ)
		defer s.cancel()
		s.key('2')
		s.wait(800*time.Millisecond, rec)
		s.click("diag_tab_sticks")
		s.click("stick_start")
		s.wait(3500*time.Millisecond, rec)
		s.click("diag_tab_health_report")
		s.wait(3*sec, rec)
		s.click("copy_full_report")
		s.wait(2*sec, rec)
	}},
	{"ez-vs-advanced", func(rec func(frameRec)) {
		// Two sessions driven by the same script; render.py places the
		// EZ recording (left) beside the Advanced one (right).
		ez := newSized(80, 26, false, state.ModeEZ)
		adv := newSized(80, 26, false, state.ModeAdvanced)
		defer ez.cancel()
		defer adv.cancel()
		both := func(fn func(*session)) { fn(ez); fn(adv) }
		tick := func(d time.Duration) {
			end := time.Now().Add(d)
			for time.Now().Before(end) {
				ez.draw(nil)
				adv.draw(nil)
				rec(pair(ez.snapshot(), adv.snapshot()))
				time.Sleep(time.Second / fps)
			}
		}
		tick(1500 * time.Millisecond)
		both(func(s *session) { s.key('2') })
		tick(2500 * time.Millisecond)
		both(func(s *session) { s.key('3') })
		tick(2 * sec)
		both(func(s *session) { s.key('5') })
		tick(2500 * time.Millisecond)
	}},
	{"firmware-check", func(rec func(frameRec)) {
		// Real, read-only check against Sony's published updater metadata.
		s := newWith(defCols, defRows, false, state.ModeAdvanced, nil)
		defer s.cancel()
		s.key('F')
		s.wait(1500*time.Millisecond, rec)
		s.click("btn_firmware_check")
		s.wait(4*sec, rec)
		s.click("btn_firmware_howto")
		s.wait(3*sec, rec)
	}},
}

// pair joins two frames horizontally with a two-cell gap.
func pair(a, b frameRec) frameRec {
	gap := segment{Text: "  ", Fg: "11131a", Bg: "11131a"}
	out := frameRec{}
	for i := range a.Rows {
		row := append(append(append([]segment{}, a.Rows[i]...), gap), b.Rows[i]...)
		out.Rows = append(out.Rows, row)
	}
	return out
}
