package diag

import (
	"math"
	"time"
)

// partialObservation exposes only observations already collected by a started
// protocol. Zero sample counts never become invented centered measurements.
func (s *Session) partialObservation(category Category, now time.Time) *Result {
	if !s.Attempted[category] {
		return nil
	}
	r := &Result{Category: category, Status: Incomplete, Summary: "The test was started but its completion requirements were not met.", Simulated: s.Ctx.Simulated, At: now, Limitations: []string{"Partial observations do not establish a hardware defect. Repeat the complete protocol."}}
	switch category {
	case CatSticks:
		if s.Sticks != nil {
			for i, stats := range s.Sticks.Rest {
				if stats.X.N == 0 {
					continue
				}
				name := s.Sticks.Layout.StickLabels[i]
				r.Measurements = append(r.Measurements, Measurement{name + " rest samples", float64(stats.X.N), "samples"}, Measurement{name + " observed center offset", 100 * math.Hypot(stats.X.Mean(), stats.Y.Mean()), "% of travel"}, Measurement{name + " resting noise", 100 * math.Max(stats.X.Std(), stats.Y.Std()), "% of travel"})
			}
		}
	case CatTriggers:
		if s.Triggers != nil {
			for i, track := range s.Triggers.track {
				name := s.Triggers.Layout.TriggerLabels[i]
				if track.restA.N > 0 {
					r.Measurements = append(r.Measurements, Measurement{name + " resting value", 100 * track.restA.Mean(), "%"})
				}
				if track.rawObserved.N > 0 {
					r.Measurements = append(r.Measurements, Measurement{name + " raw minimum", track.rawObserved.Min, "counts"}, Measurement{name + " raw maximum", track.rawObserved.Max, "counts"})
				}
			}
		}
	case CatButtons:
		if s.Buttons != nil {
			for _, button := range s.Buttons.Layout.Buttons {
				r.Measurements = append(r.Measurements, Measurement{button.Label + " observed presses", float64(s.Buttons.Counts[button.ID]), "presses"})
			}
		}
	}
	return r
}
