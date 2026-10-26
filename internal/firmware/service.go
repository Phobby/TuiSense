package firmware

import (
	"context"
	"sync"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
)

// DefaultTimeout bounds one complete check (metadata fetch included).
const DefaultTimeout = 15 * time.Second

// Service runs checks off the UI goroutine, persists the last successful
// result per controller, and decides when the user should be notified.
type Service struct {
	providers []Provider
	store     *Store
	// Timeout bounds each check (DefaultTimeout when zero).
	Timeout time.Duration
	// Now is the clock (time.Now when nil; tests override it).
	Now func() time.Time

	mu      sync.Mutex
	running map[string]bool
}

// NewService builds a service over the given providers and store.
func NewService(store *Store, providers ...Provider) *Service {
	if store == nil {
		store = NewStore("")
	}
	return &Service{providers: providers, store: store, running: map[string]bool{}}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Store returns the persistence layer.
func (s *Service) Store() *Store { return s.store }

// ProviderFor returns the provider for a device, or nil. Only devices that
// declare CapFirmwareCheck are eligible, so firmware checking is advertised
// solely where a driver and a provider both implement it.
func (s *Service) ProviderFor(sub Subject) Provider {
	if sub.Key == "" || !sub.Caps.Supports(device.CapFirmwareCheck) {
		return nil
	}
	for _, p := range s.providers {
		if p.Handles(sub) {
			return p
		}
	}
	return nil
}

// Supported reports whether a firmware check exists for the device.
func (s *Service) Supported(sub Subject) bool { return s.ProviderFor(sub) != nil }

// Running reports whether a check for the device key is in progress.
func (s *Service) Running(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running[key]
}

// Start runs a check in the background and delivers the result to done
// (called on the worker goroutine). It returns false without starting when a
// check for the same device is already running.
func (s *Service) Start(ctx context.Context, sub Subject, done func(Result)) bool {
	s.mu.Lock()
	if s.running[sub.Key] {
		s.mu.Unlock()
		return false
	}
	s.running[sub.Key] = true
	s.mu.Unlock()
	go func() {
		r := s.Check(ctx, sub)
		s.mu.Lock()
		delete(s.running, sub.Key)
		s.mu.Unlock()
		if done != nil {
			done(r)
		}
	}()
	return true
}

// Check performs one check synchronously (use Start from UI code).
func (s *Service) Check(ctx context.Context, sub Subject) Result {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r := s.check(ctx, sub)
	r.Key, r.StoreKey, r.Simulated, r.CheckedAt = sub.Key, sub.StoreKey(), sub.Simulated, s.now()
	r.Connection = sub.Info.Connection
	if sub.Key == "" {
		r.StoreKey = ""
		return r
	}
	if r.CatalogOK {
		s.record(&r)
	}
	return r
}

func (s *Service) check(ctx context.Context, sub Subject) Result {
	switch {
	case sub.Key == "":
		return Result{Status: StatusUndetermined, Summary: i18n.T("No controller is connected.")}
	case !sub.Caps.Supports(device.CapFirmwareCheck):
		return Result{Status: StatusUnsupported, Summary: i18n.T("Firmware update checks are not available for this controller.")}
	}
	p := s.ProviderFor(sub)
	if p == nil {
		return Result{Status: StatusUnsupported, Summary: i18n.T("Firmware update checks are not available for this controller.")}
	}
	if !sub.Info.Connected {
		return Result{Status: StatusUndetermined, Provider: p.Name(), Summary: i18n.T("The controller is disconnected.")}
	}
	r := p.Check(ctx, sub)
	if r.Provider == "" {
		r.Provider = p.Name()
	}
	return r
}

// record persists a successful check and decides whether to notify: an
// available version is announced once per controller, never for simulated
// devices.
func (s *Service) record(r *Result) {
	prev, _ := s.store.Get(r.StoreKey)
	rec := Record{
		CheckedAt: r.CheckedAt, Status: r.Status, Installed: r.Installed,
		Available: r.Available, Target: r.Target, Source: r.Source, Notified: prev.Notified,
	}
	if r.Status == StatusUpdateAvailable && !r.Simulated && r.Available != "" && prev.Notified != r.Available {
		r.Notify = true
		rec.Notified = r.Available
	}
	_ = s.store.Put(r.StoreKey, rec)
}

// LastSuccess returns the persisted record for a device.
func (s *Service) LastSuccess(sub Subject) (Record, bool) {
	if sub.Key == "" {
		return Record{}, false
	}
	return s.store.Get(sub.StoreKey())
}

// Due reports whether an automatic check should run: no successful check
// within interval.
func (s *Service) Due(sub Subject, interval time.Duration) bool {
	rec, ok := s.LastSuccess(sub)
	return !ok || s.now().Sub(rec.CheckedAt) >= interval
}
