package audio

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"
)

// Simulated is an in-memory backend for mock/test devices and automated tests.
// It never touches real audio hardware; its endpoints are flagged Simulated.
type Simulated struct {
	mu        sync.Mutex
	Eps       []Endpoint
	Signal    func(t float64) float64 // capture waveform in [-1,1]; nil = speech-like noise
	Played    [][]int16
	CaptureFn func() error // optional: inject capture errors
	rng       *rand.Rand
}

// NewSimulated returns a backend exposing one simulated endpoint.
func NewSimulated(name string, capture, playback int) *Simulated {
	return &Simulated{
		Eps: []Endpoint{{Backend: "simulated", CardName: name, CardID: "SIM",
			CaptureChannels: capture, PlaybackChannels: playback, Simulated: true}},
		rng: rand.New(rand.NewSource(3)),
	}
}

func (s *Simulated) Name() string                   { return "Simulated" }
func (s *Simulated) Endpoints() ([]Endpoint, error) { return s.Eps, nil }

// Capture produces 50 ms chunks in real time until cancelled.
func (s *Simulated) Capture(ctx context.Context, ep Endpoint, rate int, onChunk func([]int16)) error {
	if s.CaptureFn != nil {
		if err := s.CaptureFn(); err != nil {
			return err
		}
	}
	n := rate / 20
	t := 0.0
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		chunk := make([]int16, n)
		s.mu.Lock()
		for i := range chunk {
			var v float64
			if s.Signal != nil {
				v = s.Signal(t)
			} else {
				// syllable-like bursts of noise
				env := 0.5 + 0.5*math.Sin(t*2*math.Pi*3)
				v = 0.15 * env * s.rng.NormFloat64()
			}
			chunk[i] = int16(math.Max(-1, math.Min(1, v)) * 32767)
			t += 1 / float64(rate)
		}
		s.mu.Unlock()
		onChunk(chunk)
	}
}

// Play records the PCM and waits for its duration.
func (s *Simulated) Play(ctx context.Context, ep Endpoint, pcm []int16, rate, channels int) error {
	s.mu.Lock()
	s.Played = append(s.Played, append([]int16(nil), pcm...))
	s.mu.Unlock()
	d := time.Duration(len(pcm)/max(channels, 1)) * time.Second / time.Duration(max(rate, 1))
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
	return nil
}

// PlayedCount returns how many buffers were played (thread-safe).
func (s *Simulated) PlayedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Played)
}
