package update

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Status is the updater's state.
type Status string

const (
	StatusIdle        Status = "idle"
	StatusDisabled    Status = "disabled" // automatic checks are off
	StatusDev         Status = "dev"      // development build: no version to compare
	StatusChecking    Status = "checking"
	StatusUpToDate    Status = "up_to_date"
	StatusAvailable   Status = "available"   // newer release found, not downloaded
	StatusDownloading Status = "downloading" // verified download in progress
	StatusReady       Status = "ready"       // downloaded, verified and staged
	StatusOffline     Status = "offline"     // the check failed (network); TuiSense works normally
	StatusFailed      Status = "failed"      // download or verification failed; staged files discarded
	StatusManaged     Status = "managed"     // installed by a package manager
)

// Settings are the user's update preferences.
type Settings struct {
	Check        bool    // automatic background checks (default on)
	AutoDownload bool    // download and stage automatically (default off)
	Channel      Channel // stable unless the user opts into pre-releases
}

// Snapshot is what the UI shows.
type Snapshot struct {
	Status    Status
	Current   string
	Latest    string
	Notes     string
	Progress  float64 // 0…1 while downloading
	Err       string
	CheckedAt time.Time
	Ready     *Staged
	Settings  Settings
	// CanStage is false when this build cannot verify updates (no pinned key)
	// or the installation is managed/read-only.
	CanStage bool
}

// Manager runs checks and downloads in the background. It never blocks the
// UI, never executes anything it downloads, and only stages updates whose
// signed metadata verifies against the pinned keys.
type Manager struct {
	Source   Source
	Dir      string // staging directory (user data)
	Exe      string // the installed executable
	Keys     []TrustedKey
	GOOS     string
	GOARCH   string
	Interval time.Duration // minimum time between automatic checks
	OnChange func()        // called (from any goroutine) after every state change

	mu      sync.Mutex
	current Semver
	dev     bool
	snap    Snapshot
	rel     *Release
	busy    bool
	recheck bool // check at start-up regardless of Interval (see RestoreLatest)
}

// NewManager prepares a manager for the running version.
func NewManager(current string, settings Settings, lastCheck time.Time, dir, exe string) *Manager {
	m := &Manager{Dir: dir, Exe: exe, Keys: TrustedKeys(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Interval: time.Hour}
	v, err := ParseSemver(current)
	m.current, m.dev = v, err != nil
	m.snap = Snapshot{Status: StatusIdle, Current: current, Settings: settings, CheckedAt: lastCheck}
	if st, ok := LoadStaged(dir); ok && !m.dev {
		if sv, err := ParseSemver(st.Version); err == nil && sv.Compare(v) > 0 {
			m.snap.Status, m.snap.Ready, m.snap.Latest = StatusReady, &st, st.Version
		} else {
			_ = Discard(dir) // already installed or older
		}
	}
	m.snap.CanStage = m.canStage()
	switch {
	case m.dev:
		m.snap.Status = StatusDev
	case Managed(exe):
		m.snap.Status = StatusManaged
	case !settings.Check && m.snap.Status == StatusIdle:
		m.snap.Status = StatusDisabled
	}
	return m
}

// RestoreLatest shows the result of the previous check (the newest release
// it found) until the next check runs, so a start-up inside Interval still
// says whether TuiSense is up to date. Anything else — a newer release, a
// different running version (just updated) or no usable record — makes the
// next automatic check run at once: downloading needs fresh release metadata,
// and the user should hear about an update now.
func (m *Manager) RestoreLatest(latest string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.snap.Status != StatusIdle {
		return
	}
	v, err := ParseSemver(latest)
	if err != nil || v.Compare(m.current) != 0 {
		m.recheck = true
		return
	}
	m.snap.Status, m.snap.Latest = StatusUpToDate, latest
}

func (m *Manager) canStage() bool { return len(m.Keys) > 0 && !Managed(m.Exe) && !m.dev }

// Snapshot returns the current state.
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snap
}

// SetSettings updates preferences (the caller persists them).
func (m *Manager) SetSettings(s Settings) {
	m.mu.Lock()
	m.snap.Settings = s
	if !s.Check && m.snap.Status == StatusIdle {
		m.snap.Status = StatusDisabled
	} else if s.Check && m.snap.Status == StatusDisabled {
		m.snap.Status = StatusIdle
	}
	m.mu.Unlock()
	m.changed()
}

func (m *Manager) changed() {
	if m.OnChange != nil {
		m.OnChange()
	}
}

func (m *Manager) set(fn func(*Snapshot)) {
	m.mu.Lock()
	fn(&m.snap)
	m.mu.Unlock()
	m.changed()
}

// Due reports whether an automatic check should run now.
func (m *Manager) Due(now time.Time) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snap.Settings.Check && !m.dev && !m.busy && m.snap.Status != StatusReady &&
		(m.recheck || m.snap.CheckedAt.IsZero() || now.Sub(m.snap.CheckedAt) >= m.Interval)
}

// CheckAsync checks for a newer release in the background. Automatic
// checks honour the setting and the interval; a manual check always runs.
// When automatic downloads are enabled a newer release is staged too.
func (m *Manager) CheckAsync(manual bool) bool {
	m.mu.Lock()
	if m.busy || m.dev || (!manual && (!m.snap.Settings.Check || m.snap.Status == StatusReady)) {
		m.mu.Unlock()
		return false
	}
	if !manual && !m.recheck && !m.snap.CheckedAt.IsZero() && time.Since(m.snap.CheckedAt) < m.Interval {
		m.mu.Unlock()
		return false
	}
	m.busy, m.recheck = true, false
	m.snap.Status, m.snap.Err = StatusChecking, ""
	m.mu.Unlock()
	m.changed()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		m.check(ctx)
	}()
	return true
}

func (m *Manager) check(ctx context.Context) {
	m.mu.Lock()
	ch := m.snap.Settings.Channel
	m.mu.Unlock()
	rel, err := m.Source.Latest(ctx, ch)
	now := time.Now()
	if err != nil {
		m.set(func(s *Snapshot) {
			s.Status, s.Err, s.CheckedAt = StatusOffline, err.Error(), now
		})
		m.done()
		return
	}
	if rel.Version.Compare(m.current) <= 0 {
		m.set(func(s *Snapshot) {
			s.Status, s.Latest, s.Notes, s.CheckedAt = StatusUpToDate, rel.Version.String(), rel.Notes, now
		})
		m.done()
		return
	}
	m.mu.Lock()
	m.rel = &rel
	auto := m.snap.Settings.AutoDownload
	m.mu.Unlock()
	status := StatusAvailable
	if Managed(m.Exe) {
		status = StatusManaged
	}
	m.set(func(s *Snapshot) {
		s.Status, s.Latest, s.Notes, s.CheckedAt = status, rel.Version.String(), rel.Notes, now
	})
	if auto && status == StatusAvailable && m.canStage() {
		m.download(ctx)
	}
	m.done()
}

func (m *Manager) done() {
	m.mu.Lock()
	m.busy = false
	m.mu.Unlock()
	m.changed()
}

// DownloadAsync stages the release found by the last check.
func (m *Manager) DownloadAsync() bool {
	m.mu.Lock()
	if m.busy || m.rel == nil || m.snap.Status != StatusAvailable {
		m.mu.Unlock()
		return false
	}
	m.busy = true
	m.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		m.download(ctx)
		m.done()
	}()
	return true
}

func (m *Manager) download(ctx context.Context) {
	m.mu.Lock()
	rel := *m.rel
	m.mu.Unlock()
	if !m.canStage() {
		m.set(func(s *Snapshot) { s.Status, s.Err = StatusFailed, ErrNoTrustedKey.Error() })
		return
	}
	m.set(func(s *Snapshot) { s.Status, s.Progress, s.Err = StatusDownloading, 0, "" })
	st, err := m.Source.Stage(ctx, rel, m.Dir, m.GOOS, m.GOARCH, m.Keys, func(done, total int64) {
		if total > 0 {
			m.mu.Lock()
			m.snap.Progress = float64(done) / float64(total)
			m.mu.Unlock()
		}
	})
	if err != nil {
		_ = Discard(m.Dir)
		m.set(func(s *Snapshot) { s.Status, s.Err = StatusFailed, err.Error() })
		return
	}
	m.set(func(s *Snapshot) { s.Status, s.Ready, s.Progress = StatusReady, &st, 1 })
}

// ApplyPending installs a staged update. TuiSense calls it after the UI has
// exited normally (and the `tuisense update` command calls it directly);
// it returns the installed version, or "" when nothing was pending.
func (m *Manager) ApplyPending(validate Validator) (string, error) {
	st, ok := LoadStaged(m.Dir)
	if !ok || m.dev {
		return "", nil
	}
	if v, err := ParseSemver(st.Version); err != nil || v.Compare(m.current) <= 0 {
		_ = Discard(m.Dir)
		return "", nil
	}
	if err := Apply(st, m.Exe, validate); err != nil {
		if errors.Is(err, ErrManaged) || errors.Is(err, ErrReadOnly) {
			return "", err // keep the staged files; the user may move the install
		}
		_ = Discard(m.Dir)
		return "", err
	}
	_ = Discard(m.Dir)
	return st.Version, nil
}

// NotesExcerpt is the first lines of the release notes for the UI.
func NotesExcerpt(notes string, lines int) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		out = append(out, l)
		if len(out) == lines {
			break
		}
	}
	return out
}
