package diag

import (
	"fmt"
	"math"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
)

const ReportSchemaVersion = "2.2"

// Completion is protocol coverage, independent of whether the observed result
// is normal. A completed test can contain a warning or inconclusive evidence.
type Completion string

const (
	CompletionDone        Completion = "Completed"
	CompletionPartial     Completion = "Incomplete"
	CompletionNone        Completion = "Not Tested"
	CompletionUnsupported Completion = "Unsupported"
	CompletionUnavailable Completion = "Unavailable"
	CompletionRunning     Completion = "In progress"
)

type ComponentCompletion struct {
	Component  string     `json:"component"`
	Completion Completion `json:"completion"`
}

// Assessment explains an observation, not a diagnosis of a defective device.
// All rules consume typed engine outcomes; no thresholds are inferred by parsing
// translated measurement names or summaries.
type Assessment struct {
	Component      string        `json:"component"`
	Test           string        `json:"test"`
	Status         Status        `json:"status"`
	EvidenceKind   string        `json:"evidence_kind"`
	Observation    string        `json:"observation"`
	Evidence       []Measurement `json:"supporting_evidence"`
	Basis          string        `json:"comparison_basis"`
	Reason         string        `json:"reason"`
	PossibleCauses []string      `json:"possible_causes,omitempty"`
	GameplayImpact string        `json:"gameplay_impact"`
	Actions        []string      `json:"recommended_actions"`
	Limitations    []string      `json:"evidence_limitations"`
	At             time.Time     `json:"observed_at,omitempty"`
}

type Coverage struct {
	Inconclusive int    `json:"inconclusive"`
	Completed    int    `json:"completed"`
	Incomplete   int    `json:"incomplete"`
	NotTested    int    `json:"not_tested"`
	Unsupported  int    `json:"unsupported"`
	Unavailable  int    `json:"unavailable"`
	Warnings     int    `json:"observed_warnings"`
	Partial      bool   `json:"partial"`
	Summary      string `json:"summary"`
}

func resultCompletion(r *Result) Completion {
	if r == nil {
		return CompletionNone
	}
	switch r.Status {
	case Testing:
		return CompletionRunning
	case Unsupported:
		return CompletionUnsupported
	case Unavailable:
		return CompletionUnavailable
	case Incomplete:
		return CompletionPartial
	case NotTested:
		return CompletionNone
	default:
		if len(r.Measurements) == 0 && !r.UserConfirmed {
			return CompletionPartial
		}
		return CompletionDone
	}
}

func assessEntry(s *Session, e *ReportEntry) {
	e.Completion = resultCompletion(e.Result)
	switch e.Status {
	case Unsupported:
		e.Completion = CompletionUnsupported
	case Unavailable:
		e.Completion = CompletionUnavailable
	case Testing:
		e.Completion = CompletionRunning
	case Incomplete:
		e.Completion = CompletionPartial
	}
	if e.Result == nil {
		e.Assessments = []Assessment{emptyAssessment(*e)}
		return
	}
	if e.Completion == CompletionUnsupported || e.Completion == CompletionUnavailable {
		e.Assessments = []Assessment{emptyAssessment(*e)}
		return
	}
	switch e.Category {
	case CatSticks:
		if s.Sticks != nil && (s.Sticks.CircularityOnly || s.Sticks.Analysis[0].RestA.N == 0) {
			e.Completion = CompletionPartial
		}
		if s.Sticks != nil && !s.Sticks.CircularityOnly && s.Sticks.Analysis[0].RestA.N > 0 {
			e.Completion = CompletionDone
			for i, a := range s.Sticks.Analysis {
				completion := CompletionDone
				if !a.HasRestB || !a.TravelDone || len(a.Missed) > 0 || a.Travel.Coverage() < rimSectorsGoal {
					completion = CompletionPartial
				}
				e.Components = append(e.Components, ComponentCompletion{s.Ctx.Desc.Inputs.StickLabels[i], completion})
				if completion != CompletionDone {
					e.Completion = CompletionPartial
				}
				e.Assessments = append(e.Assessments, assessStick(s.Sticks, i, a, completion, e.Result.At))
			}
		}
		if s.Snapback != nil && s.Snapback.done {
			for i := 0; i < 2; i++ {
				a := s.Snapback.Analyze(i)
				if a.Events == 0 {
					continue
				}
				e.Assessments = append(e.Assessments, Assessment{
					Component: s.Ctx.Desc.Inputs.StickLabels[i], Test: "snapback", Status: a.Status(),
					EvidenceKind: "confirmed_observation", Observation: a.Headline,
					Evidence:    []Measurement{{"Maximum overshoot", a.MaxOvershootPct, "%"}, {"Average overshoot", a.MeanOvershootPct, "%"}, {"Releases beyond preview", float64(a.OverDeadzone), ""}, {"Sudden releases", float64(a.Events), ""}, {"Preview deadzone", s.Snapback.PreviewDeadzone, "%"}},
					Basis:       "Overshoot compared with the selected software deadzone preview; not a hardware failure threshold.",
					Limitations: []string{"Sampling frequency can miss short overshoot. Snapback alone does not prove a hardware defect."}, At: e.Result.At,
				})
			}
		}
	case CatTriggers:
		if s.Triggers != nil && s.Triggers.Step == TriggerDone {
			e.Completion = CompletionDone
			for i, a := range s.Triggers.Analysis {
				completion := CompletionDone
				if a.IncompleteTest || a.RestSamples == 0 || a.ReturnSamples == 0 || !a.Detected {
					completion = CompletionPartial
				}
				if completion != CompletionDone {
					e.Completion = CompletionPartial
				}
				e.Components = append(e.Components, ComponentCompletion{s.Ctx.Desc.Inputs.TriggerLabels[i], completion})
				assessment := assessTrigger(s.Triggers, i, a, completion, e.Result.At)
				if len(s.Ctx.Desc.ModeGuide) > 0 && assessment.Status != Passed {
					assessment.Actions = append(assessment.Actions, "Check the controller's documented physical mode indicators, then use Retest Input Mapping. TuiSense cannot switch its mode remotely.")
				}
				e.Assessments = append(e.Assessments, assessment)
			}
		}
	case CatFirmware:
		if s.Ctx.Info.FirmwareVersion != "" {
			e.Completion = CompletionDone
		}
	case CatTouchpad:
		if s.Touch != nil {
			covered, total := s.Touch.Coverage()
			if covered < total || s.Touch.Clicks == 0 {
				e.Completion = CompletionPartial
			}
		}
	case CatMotion:
		if s.Motion != nil {
			for i := 0; i < 3; i++ {
				if s.Motion.stillGyro[i].N < 2 {
					e.Completion = CompletionPartial
					e.Result.Measurements = nil
					e.Result.Limitations = append(e.Result.Limitations, "Insufficient stationary sensor samples; numeric sensor statistics are unavailable.")
				}
				if s.Motion.moveGyro[i] <= s.Motion.axisThreshold(i) {
					e.Completion = CompletionPartial
				}
			}
		}
	case CatButtons:
		if s.Buttons != nil && s.Buttons.started {
			e.Completion = CompletionDone
			for _, b := range s.Buttons.Layout.Buttons {
				completion := CompletionDone
				if !s.Buttons.Seen.Has(b.ID) {
					completion = CompletionPartial
					e.Completion = CompletionPartial
				}
				e.Components = append(e.Components, ComponentCompletion{b.Label, completion})
				if duration := s.Buttons.Stuck[b.ID]; duration > 0 {
					e.Assessments = append(e.Assessments, Assessment{Component: b.Label, Test: "Buttons and D-pad", Status: Warning, EvidenceKind: "confirmed_observation", Observation: "A continuous pressed state was observed for a long time.", Evidence: []Measurement{{"Continuous hold", duration.Seconds(), "s"}}, PossibleCauses: []string{"The button may have been intentionally held; no switch defect is established."}, Basis: "A long press can be intentional; it is not a verified stuck-switch test.", Reason: "The input remained pressed; the test cannot tell whether a finger was holding it.", GameplayImpact: "If this happens without touching the button, an action may repeat or remain active in a game.", Actions: []string{"Release the button completely and repeat the test without touching it."}, Limitations: []string{"A long hold alone does not establish a stuck or defective button."}, At: e.Result.At})
				}
			}
		}
	}
	if len(e.Assessments) == 0 {
		e.Assessments = []Assessment{genericAssessment(*e)}
	}
	if e.Completion == CompletionPartial && e.Status == Passed {
		e.Status = Incomplete
	}
}

func emptyAssessment(e ReportEntry) Assessment {
	status := e.Status
	kind, observation := "not_tested", "This test has not produced an observation. Opening its tab does not complete it."
	switch e.Completion {
	case CompletionUnsupported:
		status = Unsupported
		kind = "unsupported_measurement"
		observation = e.Reason
	case CompletionUnavailable:
		status = Unavailable
		kind = "unsupported_measurement"
		observation = e.Reason
	case CompletionPartial:
		status = Incomplete
		kind = "test_incomplete"
		observation = "The test was started but its completion requirements were not met."
	case CompletionRunning:
		status = Incomplete
		kind = "test_incomplete"
		observation = "The test is still running; no final conclusion is available."
	}
	actions := []string{}
	if e.Completion != CompletionUnsupported {
		actions = append(actions, "Run the supported test and follow its instructions before drawing a hardware conclusion.")
	}
	return Assessment{Component: e.Label, Test: e.Label, Status: status, EvidenceKind: kind, Observation: observation, Basis: "Only completed measurement requirements or explicit user confirmation establish test coverage.", Reason: "There is insufficient evidence for a hardware conclusion.", GameplayImpact: "Gameplay impact cannot be determined from this test.", Actions: actions, Limitations: []string{"Unsupported or untested capabilities are not hardware faults."}}
}

func assessStick(t *StickTest, i int, a StickAnalysis, completion Completion, at time.Time) Assessment {
	out := Assessment{Component: t.Layout.StickLabels[i], Test: "Analog sticks / drift", Status: a.Status(), EvidenceKind: "confirmed_observation", Observation: a.Headline, At: at, Reason: "The interpretation uses the measured resting position and its repeatability.", GameplayImpact: "The observed resting input is close to nominal center; this does not verify every game or deadzone.", Actions: []string{"If you notice unwanted movement in a game, repeat the resting test with the controller untouched."}, Limitations: []string{"This test does not determine a physical cause or prove a hardware defect.", "Noise, nominal-center differences, and kernel filtering affect the observations."}}
	ra := a.RestA
	if ra.N > 0 {
		out.Evidence = []Measurement{{"Initial center offset", ra.OffsetPct, "% of travel"}, {"Average resting deviation", ra.AvgDevPct, "% of travel"}, {"Maximum resting deviation", ra.MaxDevPct, "% of travel"}, {"Resting noise", ra.NoisePct, "% of travel"}, {"Rest samples", float64(ra.N), "samples"}}
	}
	if a.HasRestB && a.RestB.N > 0 {
		out.Evidence = append(out.Evidence, Measurement{"Return center offset", a.RestB.OffsetPct, "% of travel"}, Measurement{"Return-to-center shift", a.ShiftPct, "% of travel"}, Measurement{"Movement coverage", 100 * float64(a.Travel.Coverage()) / stickSectors, "%"})
	}
	center, noise := t.Tolerances()
	out.Basis = fmt.Sprintf("Center %.2f%% and noise %.2f%% interpretation heuristics, not manufacturer failure limits. %s", center, noise, t.Layout.Drift.Rationale)
	if a.Verdict == VerdictPossibleDrift && a.HasRestB && a.RestB.N > 0 {
		out.Status = PossibleIssue
		out.EvidenceKind = "possible_issue"
		out.Reason = "An elevated resting offset repeated in two measurements around the movement test."
	}
	if out.Status == Warning || out.Status == PossibleIssue {
		out.GameplayImpact = "Resting input may cause unwanted camera or character movement in games using very small deadzones."
		out.Actions = []string{"Repeat both resting measurements with the controller on a table and the sticks untouched.", "Compare the measured movement with the deadzone preview; the preview does not change game input.", "Check whether the same unwanted movement occurs in a game."}
		out.PossibleCauses = []string{"The test cannot determine a physical cause; contact during sampling or a different nominal center can influence the result."}
	}
	if completion != CompletionDone {
		out.Limitations = append(out.Limitations, "The full movement and return-to-center protocol is incomplete; the available resting observation is only preliminary.")
		if out.Status == Passed {
			out.Status = Incomplete
			out.EvidenceKind = "test_incomplete"
		}
	}
	if a.Verdict == VerdictInconclusive {
		out.EvidenceKind = "inconclusive_evidence"
		out.GameplayImpact = "Gameplay impact is not established by these uncertain readings."
	}
	return out
}

func assessTrigger(t *TriggerTest, i int, a TriggerAnalysis, completion Completion, at time.Time) Assessment {
	out := Assessment{Component: t.Layout.TriggerLabels[i], Test: "Triggers", Status: a.Status(), EvidenceKind: "confirmed_observation", Observation: a.Headline, At: at, Reason: "The test compares observed response, resting baseline, and recovery.", GameplayImpact: "Only the response observed in this mapping was checked.", Actions: []string{"Repeat the test if trigger response in games differs from the observed behavior."}, Limitations: []string{"This test does not determine the physical cause of an unusual response."}}
	out.Evidence = append(out.Evidence, Measurement{"Fresh samples", float64(a.SampleCount), "samples"})
	if a.RestSamples > 0 {
		out.Evidence = append(out.Evidence, Measurement{"Resting value", a.RestPct, "%"})
	}
	if a.ReturnSamples > 0 {
		out.Evidence = append(out.Evidence, Measurement{"Value after release", a.ReturnPct, "%"})
	}
	if t.track[i].rawObserved.N > 0 && t.Kind != device.TriggerDigital {
		out.Evidence = append(out.Evidence, Measurement{"Observed raw minimum", a.MinRaw, "counts"}, Measurement{"Observed raw maximum", a.MaxRaw, "counts"})
	}
	switch t.Kind {
	case device.TriggerDigital:
		out.Basis = "Digital on/off mapping: observe a press and release. Analog range and smoothness are unsupported."
	case device.TriggerSharedAxis:
		out.Basis = "Shared axis: direction learned during the left press; 90% of one half-range is a completion heuristic."
		out.Limitations = append(out.Limitations, "Both triggers share one axis; simultaneous presses cancel and cannot be measured independently.")
	default:
		out.Basis = "Separate analog axes: full press at 95% and resting value within 5% are interpretation heuristics, not manufacturer failure limits."
		if a.Detected {
			out.Evidence = append(out.Evidence, Measurement{"Maximum observed response", a.MaxPct, "%"}, Measurement{"Positions observed", a.CoveragePct, "%"})
		}
	}
	if completion != CompletionDone {
		out.Status = Incomplete
		out.EvidenceKind = "test_incomplete"
		out.Reason = "The required press/release, travel, or baseline evidence was not fully observed. This is not a hardware failure."
		out.GameplayImpact = "The effect on gameplay cannot be established from an incomplete test."
		out.Actions = []string{"Fully release the trigger, press it completely when prompted, then release it again.", "Verify the observed analog/digital mapping and repeat the test."}
		out.PossibleCauses = []string{"The user action may be incomplete or the active input mapping may expose the trigger differently; this test cannot establish a hardware cause."}
	} else if a.Verdict == VerdictPossibleIssue || a.Verdict == VerdictFurtherTesting {
		out.Status = Warning
		out.PossibleCauses = []string{"No physical cause was established by this test; verify the observed mapping and repeat the protocol."}
		out.EvidenceKind = "possible_issue"
		if a.Verdict == VerdictPossibleIssue {
			out.Status = PossibleIssue
		}
		out.GameplayImpact = "A trigger that remains active after release may cause unintended acceleration, aiming, or firing in a game."
		out.Actions = []string{"Repeat the full release/press test without resting a finger on the trigger.", "Verify the input mapping before interpreting the response as a physical issue."}
	}
	if len(t.Layout.AxisNames) > 5 {
		out.Basis += fmt.Sprintf(" Observed axes: %s / %s.", t.Layout.AxisNames[4], t.Layout.AxisNames[5])
	}
	return out
}

func genericAssessment(e ReportEntry) Assessment {
	r := e.Result
	out := Assessment{Component: e.Label, Test: e.Label, Status: e.Status, EvidenceKind: "confirmed_observation", Observation: r.Summary, Evidence: append([]Measurement(nil), r.Measurements...), Basis: "The recorded observations and the test's documented requirements; no additional hardware thresholds are applied.", Reason: "The result describes only what this test observed.", GameplayImpact: "Only the tested capability has been observed; behavior in every game is not guaranteed.", Actions: []string{}, Limitations: append([]string{"An observation or warning is not proof of a hardware defect."}, r.Limitations...), At: r.At}
	if e.Completion != CompletionDone {
		if out.Status == Passed {
			out.Status = Incomplete
		}
		out.EvidenceKind = "inconclusive_evidence"
		out.Actions = append(out.Actions, "Repeat the test and complete all requested steps before drawing a hardware conclusion.")
	}
	if e.Status == Warning || e.Status == Failed {
		out.EvidenceKind = "possible_issue"
		out.PossibleCauses = []string{"No physical cause was established by this test."}
		out.Actions = append(out.Actions, followUp(e.Category)...)
		if len(out.Evidence) == 0 && !r.UserConfirmed {
			out.Status = Inconclusive
			out.EvidenceKind = "inconclusive_evidence"
			out.Reason = "The warning has no measured evidence or explicit user confirmation; its cause is unverified."
		}
	}
	if r.UserConfirmed {
		out.Limitations = append(out.Limitations, "This result relies on what the user felt or heard, not physical sensor feedback.")
	}
	switch e.Category {
	case CatBattery:
		out.Basis = "Device/OS-reported telemetry only. Battery level is not battery health."
		out.GameplayImpact = "A reported charge level describes the current reading; it does not measure battery wear or future runtime."
		out.Limitations = append(out.Limitations, "Battery health, cycles, charging wattage, capacity, and wear are unavailable unless explicitly reported by the device or OS.")
	case CatHaptics:
		out.GameplayImpact = "If vibration cannot be felt during a confirmed test, effects may be absent in games; no motor fault is proven."
	case CatButtons:
		out.GameplayImpact = "Unobserved or skipped buttons have no verified conclusion; a held button may keep an action active."
	case CatMotion:
		out.GameplayImpact = "Unverified motion response may affect games using motion aiming; untested movement is not a dead sensor."
	case CatTouchpad:
		out.GameplayImpact = "Only the observed touch zones and clicks were checked."
	case CatAudioIn, CatAudioOut:
		out.GameplayImpact = "Only the controller's accessible audio endpoint was tested."
	case CatConnection:
		out.GameplayImpact = "Repeated interruptions may interrupt input in games; a single interruption does not prove a faulty controller."
	case CatFirmware:
		out.Basis = "Firmware metadata read from the device or OS; this is not a firmware integrity or update test."
		out.GameplayImpact = "Firmware metadata alone does not establish game compatibility or hardware health."
	}
	return out
}

func followUp(c Category) []string {
	switch c {
	case CatButtons:
		return []string{"Press and release each requested button; leave skipped buttons untested rather than classifying them as defective."}
	case CatConnection:
		return []string{"If interruptions repeat, try a wired connection when supported and another USB port or cable."}
	case CatHaptics:
		return []string{"Run another short vibration test and confirm whether you felt it; check the available force-feedback backend."}
	case CatMotion:
		return []string{"Repeat the stationary phase, then rotate the controller around every requested axis."}
	case CatTouchpad:
		return []string{"Slide over the full pad and click it, then repeat any unobserved zones."}
	case CatAudioIn, CatAudioOut:
		return []string{"Check the selected controller audio endpoint and its mute/volume settings, then repeat the test."}
	default:
		return []string{"Repeat the supported test under the same conditions; the physical cause has not been determined."}
	}
}

func finalizeReport(r *Report) {
	var coverage Coverage
	for _, e := range r.Entries {
		switch e.Completion {
		case CompletionDone:
			coverage.Completed++
		case CompletionPartial, CompletionRunning:
			coverage.Incomplete++
		case CompletionUnsupported:
			coverage.Unsupported++
		case CompletionUnavailable:
			coverage.Unavailable++
		default:
			coverage.NotTested++
		}
		if e.Status == Inconclusive && e.Category != CatDeadzone {
			coverage.Inconclusive++
		}
		if e.Status == Warning || e.Status == PossibleIssue || e.Status == Failed {
			coverage.Warnings++
		}
	}
	coverage.Warnings = len(r.warnings())
	coverage.Partial = coverage.Incomplete+coverage.NotTested+coverage.Unavailable > 0
	switch {
	case coverage.Completed == 0:
		coverage.Summary = "No full diagnostic protocol has been completed. The available observations form a partial report."
	case coverage.Warnings > 0:
		coverage.Summary = "Some observations require follow-up. Read the evidence and repeat the relevant tests; warnings do not establish hardware defects."
	case coverage.Inconclusive > 0:
		coverage.Summary = "Some completed observations remain inconclusive. Review the evidence and repeat the uncertain tests before drawing a hardware conclusion."
	default:
		coverage.Summary = "Completed tests did not reveal unusual behavior. This conclusion applies only to the completed test coverage."
	}
	if len(r.Entries) == 1 && r.Entries[0].Category == CatDeadzone {
		coverage.Summary = "This report describes a software preview. It does not assess hardware health or change game input."
	}
	if len(r.Entries) == 1 && r.Entries[0].Category == CatFirmware {
		coverage.Summary = "Firmware information is reported only when available. This is not a firmware integrity or hardware health assessment."
	}
	r.Coverage = coverage
	r.MissingFields = nil
	if r.Device.Firmware == "" {
		r.MissingFields = append(r.MissingFields, "Firmware version unavailable")
	}
	for _, e := range r.Entries {
		if e.Completion != CompletionDone {
			r.MissingFields = append(r.MissingFields, e.Label+": "+string(e.Completion))
		}
	}
}

// Scope returns an independent category report, retaining device and method facts.
func (r Report) Scope(category Category) Report {
	if category == "" || category == CatHealthReport {
		return r
	}
	out := r
	out.Entries = nil
	out.Events = nil
	for _, e := range r.Entries {
		if e.Category == category {
			out.Entries = append(out.Entries, e)
		}
	}
	finalizeReport(&out)
	return out
}

func validMeasurement(m Measurement) bool { return !math.IsNaN(m.Value) && !math.IsInf(m.Value, 0) }
func warningStatus(s Status) bool         { return s == Warning || s == PossibleIssue || s == Failed }
func (r Report) warnings() []Assessment {
	var out []Assessment
	for _, e := range r.Entries {
		for _, a := range e.Assessments {
			if warningStatus(a.Status) || (warningStatus(e.Status) && a.Status == Inconclusive) {
				out = append(out, a)
			}
		}
	}
	return out
}
func (r Report) SuggestedActions() []string {
	var actions []string
	seen := map[string]bool{}
	for _, e := range r.Entries {
		for _, a := range e.Assessments {
			for _, action := range a.Actions {
				if !seen[action] {
					seen[action] = true
					actions = append(actions, action)
				}
			}
		}
	}
	return actions
}
func (r Report) PotentialIssues() []Assessment { return r.warnings() }
