package diag

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/audio"
)

// MicRate is the capture sample rate used for microphone diagnostics.
const MicRate = 16000

// RecordSeconds bounds the optional in-memory recording.
const RecordSeconds = 3

// RecState is the state of the optional record/playback check.
type RecState int

const (
	RecNone RecState = iota
	RecRecording
	RecReady
	RecPlaying
)

// MicTest monitors the controller microphone. Capture starts only from an
// explicit user action; recordings live in memory and are discarded on stop.
type MicTest struct {
	Backend  audio.Backend
	In       audio.Endpoint
	Out      audio.Endpoint // controller output used for playback (never system default)
	HasOut   bool
	OnChange func()

	mu         sync.Mutex
	running    bool
	cancel     context.CancelFunc
	LevelDB    float64
	PeakDB     float64
	MinDB      float64
	Chunks     int
	ZeroChunks int
	started    time.Time
	Err        error
	History    *Ring
	rec        []int16
	recState   RecState
	playCancel context.CancelFunc
}

// NewMicTest creates the microphone test.
func NewMicTest(b audio.Backend, in, out audio.Endpoint, hasOut bool) *MicTest {
	return &MicTest{Backend: b, In: in, Out: out, HasOut: hasOut, History: NewRing(60), LevelDB: -120, PeakDB: -120, MinDB: 0}
}

func (m *MicTest) changed() {
	if m.OnChange != nil {
		m.OnChange()
	}
}

// Start begins live monitoring (explicit user action).
func (m *MicTest) Start(now time.Time) {
	m.mu.Lock()
	if m.running || m.Backend == nil {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.running, m.cancel, m.started, m.Err = true, cancel, now, nil
	m.Chunks, m.ZeroChunks, m.PeakDB, m.MinDB, m.LevelDB = 0, 0, -120, 0, -120
	m.History.Reset()
	m.mu.Unlock()

	go func() {
		err := m.Backend.Capture(ctx, m.In, MicRate, m.onChunk)
		m.mu.Lock()
		m.running = false
		if err != nil {
			m.Err = err
		}
		if m.recState == RecRecording {
			m.recState = RecNone
			m.rec = nil
		}
		m.mu.Unlock()
		m.changed()
	}()
	m.changed()
}

func (m *MicTest) onChunk(samples []int16) {
	db := audio.LevelDBFS(samples)
	zero := true
	for _, s := range samples {
		if s != 0 {
			zero = false
			break
		}
	}
	m.mu.Lock()
	m.LevelDB = db
	m.Chunks++
	if zero {
		m.ZeroChunks++
	}
	if db > m.PeakDB {
		m.PeakDB = db
	}
	if db < m.MinDB {
		m.MinDB = db
	}
	m.History.Push(db)
	if m.recState == RecRecording {
		m.rec = append(m.rec, samples...)
		if len(m.rec) >= RecordSeconds*MicRate {
			m.rec = m.rec[:RecordSeconds*MicRate]
			m.recState = RecReady
		}
	}
	m.mu.Unlock()
	m.changed()
}

// Stop ends monitoring and discards any recording.
func (m *MicTest) Stop() {
	m.mu.Lock()
	cancel, pc := m.cancel, m.playCancel
	m.cancel, m.playCancel = nil, nil
	m.rec, m.recState = nil, RecNone
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if pc != nil {
		pc()
	}
}

// Snapshot returns a consistent copy of the live state for rendering.
func (m *MicTest) Snapshot() (running bool, level, peak float64, chunks int, err error, rec RecState, history []float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running, m.LevelDB, m.PeakDB, m.Chunks, m.Err, m.recState, m.History.Values()
}

// Record captures RecordSeconds of audio into memory (requires monitoring).
func (m *MicTest) Record() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running || m.recState == RecRecording || m.recState == RecPlaying {
		return false
	}
	m.rec, m.recState = make([]int16, 0, RecordSeconds*MicRate), RecRecording
	return true
}

// Playback plays the in-memory recording on the controller's own output.
func (m *MicTest) Playback() error {
	m.mu.Lock()
	if m.recState != RecReady || !m.HasOut {
		m.mu.Unlock()
		return fmt.Errorf("no recording or no controller output available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), (RecordSeconds+3)*time.Second)
	m.playCancel = cancel
	m.recState = RecPlaying
	pcm := audio.Interleave(m.rec, m.Out.PlaybackChannels, 2)
	out := m.Out
	m.mu.Unlock()
	go func() {
		err := m.Backend.Play(ctx, out, pcm, MicRate, out.PlaybackChannels)
		cancel()
		m.mu.Lock()
		if m.recState == RecPlaying {
			m.recState = RecReady
		}
		if err != nil {
			m.Err = err
		}
		m.mu.Unlock()
		m.changed()
	}()
	return nil
}

// Discard deletes the in-memory recording.
func (m *MicTest) Discard() {
	m.mu.Lock()
	if m.recState != RecPlaying {
		m.rec, m.recState = nil, RecNone
	}
	m.mu.Unlock()
}

// Result evaluates the captured signal after ≥2 s of monitoring.
func (m *MicTest) Result(now time.Time, simulated bool, muteWarning string) *Result {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return &Result{Category: CatAudioIn, Status: Failed, Simulated: simulated, At: now,
			Summary: "Could not capture from the controller microphone.", Details: []string{m.Err.Error()}}
	}
	if m.started.IsZero() || now.Sub(m.started) < 2*time.Second || m.Chunks < 20 {
		return nil
	}
	res := &Result{Category: CatAudioIn, Simulated: simulated, At: now}
	res.Measurements = []Measurement{
		{"Peak level", m.PeakDB, "dBFS"},
		{"Quietest level", m.MinDB, "dBFS"},
		{"Audio chunks received", float64(m.Chunks), "× 50 ms"},
	}
	switch {
	case m.ZeroChunks == m.Chunks:
		res.Status = Warning
		res.Summary = "The microphone stream opened but delivered only digital silence."
		res.Details = append(res.Details, "The controller may have muted its microphone (mute button / mute LED, or the profile's mic mute).")
	case m.PeakDB >= -50 && m.PeakDB-m.MinDB >= 10:
		res.Status = Passed
		res.Summary = fmt.Sprintf("Audio input received from the controller microphone (peak %.0f dBFS).", m.PeakDB)
	default:
		res.Status = Inconclusive
		res.Summary = "Audio is arriving but stays very quiet — speak close to the controller and retest."
	}
	if muteWarning != "" {
		res.Details = append(res.Details, muteWarning)
	}
	res.Details = append(res.Details, "Device: "+m.In.Label())
	res.Limitations = []string{"Recordings are kept in memory only and discarded when the test stops; nothing is saved."}
	return res
}

// SpeakerTest plays a short chime on the controller's own audio output.
type SpeakerTest struct {
	Backend  audio.Backend
	Out      audio.Endpoint
	OnChange func()

	mu      sync.Mutex
	playing bool
	cancel  context.CancelFunc
	Played  int
	Err     error
	Heard   *bool
}

// SpeakerAmplitude is the moderate default level (≈ -12 dBFS).
const SpeakerAmplitude = 0.25

// SpeakerRate is the playback sample rate.
const SpeakerRate = 48000

// NewSpeakerTest creates the speaker test.
func NewSpeakerTest(b audio.Backend, out audio.Endpoint) *SpeakerTest {
	return &SpeakerTest{Backend: b, Out: out}
}

// Play plays the chime (explicit user action) on channels 1–2 only.
func (s *SpeakerTest) Play() {
	s.mu.Lock()
	if s.playing || s.Backend == nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	s.playing, s.cancel, s.Err = true, cancel, nil
	s.mu.Unlock()
	pcm := audio.Interleave(audio.TestChime(SpeakerRate, SpeakerAmplitude), s.Out.PlaybackChannels, 2)
	go func() {
		err := s.Backend.Play(ctx, s.Out, pcm, SpeakerRate, max(s.Out.PlaybackChannels, 1))
		cancel()
		s.mu.Lock()
		s.playing = false
		s.Played++
		s.Err = err
		s.mu.Unlock()
		if s.OnChange != nil {
			s.OnChange()
		}
	}()
}

// Stop cancels playback.
func (s *SpeakerTest) Stop() {
	s.mu.Lock()
	c := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// PlayCount returns how many times the chime finished playing.
func (s *SpeakerTest) PlayCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Played
}

// LastError returns the most recent playback error.
func (s *SpeakerTest) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Err
}

// Playing reports whether the chime is playing.
func (s *SpeakerTest) Playing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.playing
}

// Result uses the user's confirmation; playback errors are failures.
func (s *SpeakerTest) Result(now time.Time, simulated bool, volumeNote string) *Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return &Result{Category: CatAudioOut, Status: Failed, Simulated: simulated, At: now,
			Summary: "Could not play audio on the controller.", Details: []string{s.Err.Error()}}
	}
	if s.Heard == nil {
		return nil
	}
	res := &Result{Category: CatAudioOut, Simulated: simulated, UserConfirmed: true, At: now}
	if *s.Heard {
		res.Status, res.Summary = Passed, "Test sound heard from the controller."
	} else {
		res.Status = Warning
		res.Summary = "Playback succeeded but no sound was heard."
		res.Details = append(res.Details,
			"If a headset is plugged into the controller, sound goes to the headset jack.",
			"The controller firmware decides speaker/headphone routing; TuiSense does not change it.")
	}
	if volumeNote != "" {
		res.Details = append(res.Details, volumeNote)
	}
	res.Details = append(res.Details, "Device: "+s.Out.Label())
	res.Limitations = []string{"Only audio channels 1–2 are used; channels 3–4 (haptics on DualSense) stay silent."}
	return res
}
