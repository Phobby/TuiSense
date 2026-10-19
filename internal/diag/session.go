package diag

import (
	"time"

	"github.com/ygzztrk/tuisense/internal/audio"
	"github.com/ygzztrk/tuisense/internal/device"
)

// GuidedOrder is the order of the guided health check.
var GuidedOrder = []Category{
	CatConnection, CatBattery, CatSticks, CatButtons, CatTriggers,
	CatTouchpad, CatMotion, CatHaptics, CatAudioIn, CatAudioOut,
}

// Guided is a step-by-step run through every available category.
type Guided struct {
	Steps []Category
	Idx   int
}

// Current returns the active step.
func (g *Guided) Current() (Category, bool) {
	if g == nil || g.Idx >= len(g.Steps) {
		return "", false
	}
	return g.Steps[g.Idx], true
}

// Session holds all diagnostics state for one device. All methods run on the
// UI goroutine except the audio tests' internal capture/playback goroutines,
// which synchronize themselves and report through OnChange.
type Session struct {
	Ctx     DeviceContext
	Avail   map[Category]Availability
	Results map[Category]Result
	Active  Category // "" = health summary

	Sticks   *StickTest
	Buttons  *ButtonTest
	Triggers *TriggerTest
	Touch    *TouchTest
	Motion   *MotionTest
	Haptics  *HapticTest
	Battery  *BatteryMonitor
	Conn     *ConnMonitor
	Mic      *MicTest
	Speaker  *SpeakerTest
	Guided   *Guided

	Backend  audio.Backend
	OnChange func()
	act      Actuator
	sim      device.ScenarioSimulator
}

// NewSession creates the diagnostics session for a device.
func NewSession(dc DeviceContext, act Actuator, sim device.ScenarioSimulator, battery device.BatteryDetailsProvider, backend audio.Backend, now time.Time) *Session {
	s := &Session{Ctx: dc, Results: map[Category]Result{}, act: act, sim: sim, Backend: backend}
	in := dc.Desc.Inputs
	scn := s.setScenario
	if in.HasSticks() {
		s.Sticks = NewStickTest(in, scn)
	}
	if len(in.Buttons) > 0 {
		s.Buttons = NewButtonTest(in, scn)
	}
	if in.HasTriggers() {
		s.Triggers = NewTriggerTest(in, dc.Caps.Supports(device.CapAdaptiveTriggers), act, scn)
	}
	if in.TouchpadContacts > 0 {
		s.Touch = NewTouchTest(in)
	}
	if dc.Caps.Supports(device.CapMotion) {
		s.Motion = NewMotionTest(in, scn)
	}
	if dc.Caps.Supports(device.CapHaptics) {
		s.Haptics = NewHapticTest(dc.Caps, act)
	}
	if dc.Caps.Supports(device.CapBattery) {
		s.Battery = NewBatteryMonitor(in.BatteryStep, battery)
	}
	s.Conn = NewConnMonitor(now)
	s.Conn.Info, s.Conn.connected, s.Conn.seen = dc.Info, dc.Info.Connected, true
	s.SetContext(dc)
	return s
}

func (s *Session) setScenario(scn device.InputScenario) {
	if s.sim != nil {
		s.sim.SetInputScenario(scn)
	}
}

func (s *Session) changed() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// SetContext re-plans availability (connection/transport/audio changes) and
// (re)creates audio tests for the controller's own endpoint.
func (s *Session) SetContext(dc DeviceContext) {
	prevEP, prevFound := s.Ctx.Audio.Endpoint, s.Ctx.Audio.Found
	s.Ctx = dc
	s.Avail = Plan(dc)
	if s.Mic != nil && (!dc.Audio.Found || dc.Audio.Endpoint != prevEP || !prevFound) {
		s.Mic.Stop()
		s.Mic = nil
	}
	if s.Speaker != nil && (!dc.Audio.Found || dc.Audio.Endpoint != prevEP || !prevFound) {
		s.Speaker.Stop()
		s.Speaker = nil
	}
	if s.Avail[CatAudioIn].Available() && s.Mic == nil {
		ep := dc.Audio.Endpoint
		s.Mic = NewMicTest(s.Backend, ep, ep, ep.CanPlay())
		s.Mic.OnChange = s.changed
	}
	if s.Avail[CatAudioOut].Available() && s.Speaker == nil {
		s.Speaker = NewSpeakerTest(s.Backend, dc.Audio.Endpoint)
		s.Speaker.OnChange = s.changed
	}
}

// Feed processes one telemetry sample (UI goroutine).
func (s *Session) Feed(tel device.Telemetry, info device.Info, now time.Time) {
	if info.Connected != s.Ctx.Info.Connected || info.Connection != s.Ctx.Info.Connection {
		dc := s.Ctx
		dc.Info = info
		if !info.Connected {
			s.Suspend()
		}
		s.SetContext(dc)
	}
	s.Ctx.Info = info
	s.Conn.Feed(tel, info, now)
	if info.Connected {
		if s.Sticks != nil {
			s.Sticks.Feed(tel, now)
		}
		if s.Buttons != nil {
			s.Buttons.Feed(tel, now)
		}
		if s.Triggers != nil {
			s.Triggers.Feed(tel, now)
		}
		if s.Touch != nil {
			s.Touch.Feed(tel)
		}
		if s.Motion != nil {
			s.Motion.Feed(tel, now)
		}
		if s.Battery != nil {
			s.Battery.Feed(tel, now)
		}
	}
	s.collect(now)
}

// Tick advances timers (pattern steps, phase ends, safety timeouts).
func (s *Session) Tick(now time.Time) {
	if s.Sticks != nil {
		s.Sticks.Tick(now)
	}
	if s.Triggers != nil {
		s.Triggers.Tick(now)
	}
	if s.Motion != nil {
		s.Motion.Tick(now)
	}
	if s.Haptics != nil {
		s.Haptics.Tick(now)
	}
	s.collect(now)
}

// collect refreshes results from every test that has produced one.
func (s *Session) collect(now time.Time) {
	sim := s.Ctx.Simulated
	put := func(r *Result) {
		if r != nil {
			s.Results[r.Category] = *r
		}
	}
	if s.Sticks != nil {
		put(s.Sticks.Result(now, sim))
	}
	if s.Buttons != nil {
		put(s.Buttons.Result(now, sim))
	}
	if s.Triggers != nil {
		put(s.Triggers.Result(now, sim))
	}
	if s.Touch != nil {
		put(s.Touch.Result(now, sim))
	}
	if s.Motion != nil {
		put(s.Motion.Result(now, sim))
	}
	if s.Haptics != nil {
		put(s.Haptics.Result(now, sim))
	}
	if s.Battery != nil {
		put(s.Battery.Result(now, sim))
	}
	put(s.Conn.Result(now, sim))
	if s.Mic != nil {
		put(s.Mic.Result(now, sim, s.MicMuteWarning()))
	}
	if s.Speaker != nil {
		put(s.Speaker.Result(now, sim, s.SpeakerVolumeNote()))
	}
}

// MicMuteWarning explains when the user's own settings mute the controller mic.
// Diagnostics never change these settings.
func (s *Session) MicMuteWarning() string {
	if s.act == nil {
		return ""
	}
	cfg := s.act.CurrentConfig()
	if cfg.Audio.ForceMicMute || cfg.Lighting.MicLED != device.MicLEDOff {
		return "Your current profile mutes the controller microphone (mute LED on). Turn the mute LED off in Advanced › Audio to hear input."
	}
	return ""
}

// SpeakerVolumeNote warns about a zero speaker volume in the user's profile.
func (s *Session) SpeakerVolumeNote() string {
	if s.act == nil {
		return ""
	}
	if v := s.act.CurrentConfig().Audio.SpeakerVolume; v == 0 {
		return "Your profile sets the controller speaker volume to 0%."
	}
	return ""
}

// Running reports whether category c has an active measurement or effect.
func (s *Session) Running(c Category) bool {
	switch c {
	case CatSticks:
		return s.Sticks != nil && s.Sticks.Running()
	case CatTriggers:
		return s.Triggers != nil && (s.Triggers.Running() || s.Triggers.ResistanceActive)
	case CatMotion:
		return s.Motion != nil && s.Motion.Running()
	case CatHaptics:
		return s.Haptics != nil && s.Haptics.Running != nil
	case CatButtons:
		return s.Buttons != nil && s.Buttons.Guided && !s.Buttons.GuidedDone()
	case CatAudioIn:
		if s.Mic != nil {
			running, _, _, _, _, _, _ := s.Mic.Snapshot()
			return running
		}
	case CatAudioOut:
		return s.Speaker != nil && s.Speaker.Playing()
	}
	return false
}

// Status returns the display status of a category.
func (s *Session) Status(c Category) Status {
	if a, ok := s.Avail[c]; ok && !a.Available() {
		return a.Status
	}
	if s.Running(c) {
		return Testing
	}
	if r, ok := s.Results[c]; ok {
		return r.Status
	}
	return NotTested
}

// Visible lists categories to show: available or temporarily unavailable ones
// (with a reason). Categories the model does not have at all are omitted.
func (s *Session) Visible() []Category {
	var out []Category
	for _, c := range AllCategories {
		if s.Avail[c].Status != Unsupported {
			out = append(out, c)
		}
	}
	return out
}

// Suspend stops every active effect and measurement: haptics off, triggers
// restored, audio stopped, recordings discarded. Called when leaving the
// diagnostics view, switching devices, disconnecting, or quitting.
func (s *Session) Suspend() {
	if s.Sticks != nil {
		s.Sticks.Cancel()
	}
	if s.Triggers != nil {
		s.Triggers.Cancel()
		s.Triggers.StopResistance()
	}
	if s.Motion != nil {
		s.Motion.Cancel()
	}
	if s.Haptics != nil {
		s.Haptics.Stop()
	}
	if s.Mic != nil {
		s.Mic.Stop()
	}
	if s.Speaker != nil {
		s.Speaker.Stop()
	}
	if s.Buttons != nil && s.Buttons.Guided && !s.Buttons.GuidedDone() {
		s.Buttons.Guided = false
	}
	s.setScenario(device.ScenarioDemo)
}

// Reset clears all results and observations (a fresh health check).
func (s *Session) Reset(now time.Time) {
	s.Suspend()
	act, sim, backend, onChange := s.act, s.sim, s.Backend, s.OnChange
	var battery device.BatteryDetailsProvider
	if s.Battery != nil {
		battery = s.Battery.provider
	}
	*s = *NewSession(s.Ctx, act, sim, battery, backend, now)
	s.OnChange = onChange
}

// StartGuided begins the guided health check over available categories.
func (s *Session) StartGuided() {
	g := &Guided{}
	for _, c := range GuidedOrder {
		if s.Avail[c].Available() {
			g.Steps = append(g.Steps, c)
		}
	}
	s.Guided = g
	if c, ok := g.Current(); ok {
		s.Active = c
	}
}

// NextGuided advances (or finishes) the guided run, stopping the current step's effects.
func (s *Session) NextGuided() {
	if s.Guided == nil {
		return
	}
	s.Suspend()
	s.Guided.Idx++
	if c, ok := s.Guided.Current(); ok {
		s.Active = c
	} else {
		s.Guided = nil
		s.Active = ""
	}
}

// StopGuided ends the guided run.
func (s *Session) StopGuided() {
	s.Suspend()
	s.Guided = nil
}
