package diag

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
)

// GuidedOutcome records how a guided button prompt ended.
type GuidedOutcome int

const (
	OutcomePending GuidedOutcome = iota
	OutcomeConfirmed
	OutcomeSkipped
)

// ButtonTest tracks live button state, press counts, stuck observations, and
// an optional guided "press each button" sequence.
type ButtonTest struct {
	Layout     device.InputLayout
	StuckAfter time.Duration

	Pressed     device.ButtonSet
	Seen        device.ButtonSet
	Counts      map[device.ButtonID]int
	heldSince   map[device.ButtonID]time.Time
	Stuck       map[device.ButtonID]time.Duration // longest continuous hold observed beyond StuckAfter
	HeldAtStart device.ButtonSet

	Guided    bool
	GuidedIdx int
	Outcomes  map[device.ButtonID]GuidedOutcome
	started   bool
	scenario  func(device.InputScenario)
}

// NewButtonTest creates a tester for the layout's declared buttons.
func NewButtonTest(layout device.InputLayout, scenario func(device.InputScenario)) *ButtonTest {
	t := &ButtonTest{Layout: layout, StuckAfter: 5 * time.Second, scenario: scenario}
	t.Reset()
	return t
}

// Reset clears counts and observations.
func (t *ButtonTest) Reset() {
	t.Counts = map[device.ButtonID]int{}
	t.heldSince = map[device.ButtonID]time.Time{}
	t.Stuck = map[device.ButtonID]time.Duration{}
	t.Outcomes = map[device.ButtonID]GuidedOutcome{}
	t.Seen, t.Pressed, t.HeldAtStart = 0, 0, 0
	t.Guided, t.GuidedIdx, t.started = false, 0, false
}

// Feed processes one sample. Presses that started and ended between two polls
// are still counted through ButtonsLatched.
func (t *ButtonTest) Feed(tel device.Telemetry, now time.Time) {
	if len(tel.Samples) > 0 {
		for _, sample := range tel.Samples {
			t.feedState(sample.Buttons, sample.Buttons, sample.At)
		}
		return
	}
	t.feedState(tel.Buttons, tel.ButtonsLatched|tel.Buttons, now)
}

func (t *ButtonTest) feedState(current, latched device.ButtonSet, now time.Time) {
	if !t.started {
		t.started = true
		t.HeldAtStart = current
	}
	for _, b := range t.Layout.Buttons {
		id := b.ID
		was := t.Pressed.Has(id)
		if latched.Has(id) && !was {
			t.Counts[id]++
			t.Seen = t.Seen.With(id)
			if t.Guided && t.GuidedIdx < len(t.Layout.Buttons) && t.Layout.Buttons[t.GuidedIdx].ID == id {
				t.Outcomes[id] = OutcomeConfirmed
				t.advance()
			}
		}
		if current.Has(id) {
			if _, ok := t.heldSince[id]; !ok {
				t.heldSince[id] = now
			}
			if held := now.Sub(t.heldSince[id]); held >= t.StuckAfter && held > t.Stuck[id] {
				t.Stuck[id] = held
			}
		} else {
			delete(t.heldSince, id)
		}
	}
	t.Pressed = current
}

// StartGuided begins the press-each-button sequence.
func (t *ButtonTest) StartGuided() {
	t.Reset()
	t.Guided = true
	if t.scenario != nil {
		t.scenario(device.ScenarioButtons)
	}
}

// SkipCurrent marks the prompted button as not confirmed and moves on.
func (t *ButtonTest) SkipCurrent() {
	if t.Guided && t.GuidedIdx < len(t.Layout.Buttons) {
		t.Outcomes[t.Layout.Buttons[t.GuidedIdx].ID] = OutcomeSkipped
		t.advance()
	}
}

func (t *ButtonTest) advance() {
	t.GuidedIdx++
	if t.GuidedIdx >= len(t.Layout.Buttons) && t.scenario != nil {
		t.scenario(device.ScenarioDemo)
	}
}

// Prompt returns the button the guided test is waiting for.
func (t *ButtonTest) Prompt() (device.ButtonDef, bool) {
	if !t.Guided || t.GuidedIdx >= len(t.Layout.Buttons) {
		return device.ButtonDef{}, false
	}
	return t.Layout.Buttons[t.GuidedIdx], true
}

// GuidedDone reports whether the guided sequence has finished.
func (t *ButtonTest) GuidedDone() bool { return t.Guided && t.GuidedIdx >= len(t.Layout.Buttons) }

// Result returns a result once the guided run is complete, or for a free test
// once every button has been seen. Otherwise nil (nothing verified yet).
func (t *ButtonTest) Result(now time.Time, simulated bool) *Result {
	total := len(t.Layout.Buttons)
	seen := 0
	for _, b := range t.Layout.Buttons {
		if t.Seen.Has(b.ID) {
			seen++
		}
	}
	if !t.GuidedDone() && seen < total {
		return nil
	}
	res := &Result{Category: CatButtons, Simulated: simulated, At: now}
	var missing []string
	for _, b := range t.Layout.Buttons {
		if !t.Seen.Has(b.ID) {
			missing = append(missing, i18n.T(b.Label))
		}
	}
	var stuck []string
	for id, d := range t.Stuck {
		if b, ok := t.Layout.Button(id); ok {
			stuck = append(stuck, i18n.Tf("%s (held %.0fs)", i18n.T(b.Label), d.Seconds()))
		}
	}
	res.Measurements = []Measurement{{i18n.T("Buttons registered"), float64(seen), i18n.Tf("of %d", total)}}
	switch {
	case len(missing) > 0:
		res.Status = Incomplete
		res.Summary = i18n.Tf("%d of %d buttons were not tested.", len(missing), total)
		res.Details = append(res.Details, i18n.T("Skipped — no conclusion about these buttons: ")+strings.Join(missing, ", "))
	case len(stuck) > 0:
		res.Status = Warning
		res.Summary = i18n.T("All buttons registered, but some stayed pressed for a long time.")
	default:
		res.Status = Passed
		res.Summary = i18n.Tf("All %d buttons registered presses.", total)
	}
	if len(stuck) > 0 {
		res.Details = append(res.Details, i18n.T("Long continuous presses (stuck, or simply held): ")+strings.Join(stuck, ", "))
	}
	res.Limitations = []string{
		i18n.T("Per-report transitions are counted when available. A latched-only source cannot distinguish multiple presses between polls; repeated key events alone do not prove switch bounce."),
		i18n.T("Analog travel of L2/R2 is tested separately in the trigger test."),
	}
	return res
}
