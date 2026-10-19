package diag

import "math"

// AxisStats accumulates mean/variance (Welford) and extremes of a series.
type AxisStats struct {
	N        int
	mean, m2 float64
	Min, Max float64
}

// Add records one value.
func (a *AxisStats) Add(v float64) {
	if a.N == 0 {
		a.Min, a.Max = v, v
	}
	a.N++
	d := v - a.mean
	a.mean += d / float64(a.N)
	a.m2 += d * (v - a.mean)
	a.Min = math.Min(a.Min, v)
	a.Max = math.Max(a.Max, v)
}

// Mean returns the arithmetic mean (0 when empty).
func (a AxisStats) Mean() float64 { return a.mean }

// Std returns the population standard deviation.
func (a AxisStats) Std() float64 {
	if a.N < 2 {
		return 0
	}
	return math.Sqrt(a.m2 / float64(a.N))
}

// PeakToPeak returns Max-Min.
func (a AxisStats) PeakToPeak() float64 {
	if a.N == 0 {
		return 0
	}
	return a.Max - a.Min
}

// Ring is a fixed-size rolling window of floats.
type Ring struct {
	buf  []float64
	next int
	full bool
}

// NewRing creates a ring holding n values.
func NewRing(n int) *Ring { return &Ring{buf: make([]float64, n)} }

// Push appends a value, overwriting the oldest when full.
func (r *Ring) Push(v float64) {
	r.buf[r.next] = v
	r.next = (r.next + 1) % len(r.buf)
	if r.next == 0 {
		r.full = true
	}
}

// Values returns the window oldest-first.
func (r *Ring) Values() []float64 {
	if !r.full {
		return append([]float64(nil), r.buf[:r.next]...)
	}
	return append(append([]float64(nil), r.buf[r.next:]...), r.buf[:r.next]...)
}

// Std returns the standard deviation of the window.
func (r *Ring) Std() float64 {
	var s AxisStats
	for _, v := range r.Values() {
		s.Add(v)
	}
	return s.Std()
}

// Reset empties the ring.
func (r *Ring) Reset() { r.next, r.full = 0, false }

// worst returns the most severe status of a set (Failed > Warning > Inconclusive > Passed).
func worst(statuses ...Status) Status {
	rank := map[Status]int{Passed: 1, Inconclusive: 2, Warning: 3, Failed: 4}
	best := NotTested
	for _, s := range statuses {
		if rank[s] > rank[best] {
			best = s
		}
	}
	return best
}
