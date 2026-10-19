// Package audio provides controller audio endpoint discovery, capture, and
// playback behind a platform-neutral Backend interface. The Linux backend talks
// to ALSA directly; macOS (CoreAudio) and Windows (WASAPI) backends can be added
// by implementing Backend.
package audio

import (
	"context"
	"errors"
	"math"

	"github.com/ygzztrk/tuisense/internal/device"
)

// Endpoint is a sound card that may offer capture and/or playback.
type Endpoint struct {
	Backend          string // "alsa", "simulated", ...
	CardIndex        int
	CardID           string // ALSA card id, e.g. "Controller"
	CardName         string // human-readable name
	USBID            string // "054c:0ce6" for USB audio cards, else ""
	PCM              string // explicit device string, e.g. "plughw:CARD=Controller,DEV=0"
	CaptureChannels  int    // 0 = no capture
	PlaybackChannels int    // 0 = no playback
	Simulated        bool
}

// CanCapture reports whether the endpoint has an input stream.
func (e Endpoint) CanCapture() bool { return e.CaptureChannels > 0 }

// CanPlay reports whether the endpoint has an output stream.
func (e Endpoint) CanPlay() bool { return e.PlaybackChannels > 0 }

// Label describes the endpoint for the UI (always shows the exact target).
func (e Endpoint) Label() string {
	if e.Simulated {
		return e.CardName + " (simulated)"
	}
	if e.PCM != "" {
		return e.CardName + " · " + e.PCM
	}
	return e.CardName
}

// Backend enumerates endpoints and streams PCM. All calls must honour ctx and
// return promptly after cancellation.
type Backend interface {
	Name() string
	Endpoints() ([]Endpoint, error)
	// Capture streams mono signed 16-bit samples at rate in small chunks until
	// ctx is cancelled or an error occurs.
	Capture(ctx context.Context, ep Endpoint, rate int, onChunk func([]int16)) error
	// Play writes interleaved signed 16-bit PCM and blocks until done or cancelled.
	Play(ctx context.Context, ep Endpoint, pcm []int16, rate, channels int) error
}

// ErrNoEndpoint means no sound card belonging to the controller was found.
var ErrNoEndpoint = errors.New("no audio endpoint belonging to this controller was found")

// FindControllerEndpoint returns the endpoint matching the controller, never a
// generic/default device. ok=false with err=nil means nothing matched.
func FindControllerEndpoint(b Backend, m device.AudioMatch) (Endpoint, bool, error) {
	if b == nil {
		return Endpoint{}, false, nil
	}
	eps, err := b.Endpoints()
	if err != nil {
		return Endpoint{}, false, err
	}
	for _, ep := range eps {
		if ep.Simulated || m.Matches(ep.USBID, ep.CardName) {
			return ep, true, nil
		}
	}
	return Endpoint{}, false, nil
}

// LevelDBFS returns the RMS level of samples in dBFS (-120 for silence).
func LevelDBFS(samples []int16) float64 {
	if len(samples) == 0 {
		return -120
	}
	var sum float64
	for _, s := range samples {
		v := float64(s) / 32768
		sum += v * v
	}
	rms := math.Sqrt(sum / float64(len(samples)))
	if rms <= 1e-6 {
		return -120
	}
	return 20 * math.Log10(rms)
}

// TestChime synthesizes a short, gentle three-note chime: mono samples at the
// given amplitude (0..1), with fade-in/out to avoid clicks.
func TestChime(rate int, amplitude float64) []int16 {
	notes := []float64{660, 880, 1320}
	noteLen := rate * 18 / 100 // 180 ms per note
	fade := rate / 200         // 5 ms
	out := make([]int16, 0, noteLen*len(notes))
	for _, f := range notes {
		for i := 0; i < noteLen; i++ {
			env := 1.0
			if i < fade {
				env = float64(i) / float64(fade)
			} else if i > noteLen-fade {
				env = float64(noteLen-i) / float64(fade)
			}
			v := amplitude * env * math.Sin(2*math.Pi*f*float64(i)/float64(rate))
			out = append(out, int16(v*32767))
		}
	}
	return out
}

// Interleave places mono samples on the first `audible` channels of a
// `channels`-wide frame and leaves the rest silent. On the DualSense, channels
// 3–4 of the USB audio stream drive the haptic actuators, so test audio must
// only ever use channels 1–2.
func Interleave(mono []int16, channels, audible int) []int16 {
	if channels <= 0 {
		channels = 1
	}
	if audible > channels {
		audible = channels
	}
	out := make([]int16, len(mono)*channels)
	for i, s := range mono {
		for c := 0; c < audible; c++ {
			out[i*channels+c] = s
		}
	}
	return out
}
