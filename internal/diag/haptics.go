package diag

import (
	"fmt"
	"strings"
	"time"

	"github.com/ygzztrk/tuisense/internal/device"
)

// HapticIntensity is a bounded test strength.
type HapticIntensity struct {
	Name  string
	Level uint8
}

// HapticIntensities are the only strengths the test can use (max ≈ 90%).
var HapticIntensities = []HapticIntensity{{"Low", 90}, {"Medium", 165}, {"High", 230}}

type hapticStep struct {
	at          time.Duration
	left, right uint8
	triggers    bool // drive impulse-trigger motors instead of grip motors
	dur         time.Duration
	restore     bool
}

// HapticPattern is a short, bounded vibration sequence.
type HapticPattern struct {
	ID, Label string
	steps     []hapticStep
	Total     time.Duration
}

// HapticTest plays user-triggered patterns and collects "did you feel it" feedback.
type HapticTest struct {
	Patterns   []HapticPattern
	Intensity  int // index into HapticIntensities
	Separate   bool
	Running    *HapticPattern
	started    time.Time
	next       int
	Feedback   map[string]bool
	LastPlayed string
	act        Actuator
}

// NewHapticTest derives the available patterns from the haptics features.
func NewHapticTest(caps device.CapabilitySet, act Actuator) *HapticTest {
	t := &HapticTest{Intensity: 1, Feedback: map[string]bool{}, act: act}
	t.Separate = caps.HasFeature(device.CapHaptics, device.FeatDualMotors)
	ms := time.Millisecond
	if t.Separate {
		t.Patterns = append(t.Patterns,
			HapticPattern{ID: "left", Label: "Left motor", steps: []hapticStep{{left: 1, dur: 350 * ms}}},
			HapticPattern{ID: "right", Label: "Right motor", steps: []hapticStep{{right: 1, dur: 350 * ms}}},
			HapticPattern{ID: "both", Label: "Both", steps: []hapticStep{{left: 1, right: 1, dur: 350 * ms}}},
			HapticPattern{ID: "pulse", Label: "Pulse ×3", steps: []hapticStep{
				{at: 0, left: 1, right: 1, dur: 140 * ms}, {at: 300 * ms, left: 1, right: 1, dur: 140 * ms}, {at: 600 * ms, left: 1, right: 1, dur: 140 * ms}}},
		)
	} else {
		t.Patterns = append(t.Patterns, HapticPattern{ID: "rumble", Label: "Rumble", steps: []hapticStep{{left: 1, right: 1, dur: 350 * ms}}})
	}
	if caps.HasFeature(device.CapHaptics, device.FeatTriggerMotors) {
		t.Patterns = append(t.Patterns, HapticPattern{ID: "triggers", Label: "Trigger motors", steps: []hapticStep{
			{triggers: true, left: 1, right: 1, dur: 350 * ms}, {at: 350 * ms, restore: true}}})
	}
	for i := range t.Patterns {
		p := &t.Patterns[i]
		for _, s := range p.steps {
			if end := s.at + s.dur; end > p.Total {
				p.Total = end
			}
		}
	}
	return t
}

// Play starts a pattern (explicit user action). Any running pattern is stopped first.
func (t *HapticTest) Play(id string, now time.Time) bool {
	t.Stop()
	for i := range t.Patterns {
		if t.Patterns[i].ID == id {
			t.Running, t.started, t.next, t.LastPlayed = &t.Patterns[i], now, 0, id
			t.Tick(now)
			return true
		}
	}
	return false
}

// Tick fires due steps and ends the pattern.
func (t *HapticTest) Tick(now time.Time) {
	if t.Running == nil || t.act == nil {
		return
	}
	level := HapticIntensities[t.Intensity].Level
	elapsed := now.Sub(t.started)
	for t.next < len(t.Running.steps) && t.Running.steps[t.next].at <= elapsed {
		s := t.Running.steps[t.next]
		switch {
		case s.restore:
			t.act.Dispatch(device.Command{Type: device.CmdSetHaptics, Config: t.act.CurrentConfig()})
		case s.triggers:
			cfg := t.act.CurrentConfig()
			cfg.Haptics.LeftTriggerMotor, cfg.Haptics.RightTriggerMotor = s.left*level, s.right*level
			t.act.Dispatch(device.Command{Type: device.CmdSetHaptics, Config: cfg})
		default:
			t.act.Dispatch(device.Command{Type: device.CmdPulseHaptics, PulseLeft: s.left * level, PulseRight: s.right * level, PulseDur: s.dur, PulseAbsolute: true})
		}
		t.next++
	}
	if elapsed >= t.Running.Total && t.next >= len(t.Running.steps) {
		t.Running = nil
	}
}

// Stop cancels the pattern and restores the user's steady-state haptics
// (motors off unless the profile set them). Safe to call anytime.
func (t *HapticTest) Stop() {
	if t.Running == nil {
		return
	}
	t.Running = nil
	if t.act != nil {
		t.act.Dispatch(device.Command{Type: device.CmdSetHaptics, Config: t.act.CurrentConfig()})
	}
}

// Result is produced once the user gave feedback on at least one pattern.
func (t *HapticTest) Result(now time.Time, simulated bool) *Result {
	if len(t.Feedback) == 0 {
		return nil
	}
	res := &Result{Category: CatHaptics, Simulated: simulated, UserConfirmed: true, At: now}
	var felt, notFelt []string
	for _, p := range t.Patterns {
		if f, ok := t.Feedback[p.ID]; ok {
			if f {
				felt = append(felt, p.Label)
			} else {
				notFelt = append(notFelt, p.Label)
			}
		}
	}
	if len(notFelt) > 0 {
		res.Status = Warning
		res.Summary = "Some vibration patterns were not felt: " + strings.Join(notFelt, ", ")
		res.Details = append(res.Details, "If a single side is never felt, that motor may be faulty. Also check that the controller is not muted/in power-saving mode.")
	} else {
		res.Status = Passed
		res.Summary = fmt.Sprintf("Vibration felt for: %s.", strings.Join(felt, ", "))
	}
	res.Measurements = []Measurement{{"Patterns confirmed", float64(len(felt)), fmt.Sprintf("of %d tried", len(t.Feedback))}}
	res.Limitations = []string{
		"Vibration can only be confirmed by feel; controllers report no motor feedback.",
		"Uses the compatibility rumble mode. DualSense voice-coil 'advanced haptics' are driven through USB audio channels 3–4 and are not tested here.",
		"Test pulses are bounded to ≤ 0.35 s at ≤ 90% strength.",
	}
	return res
}
