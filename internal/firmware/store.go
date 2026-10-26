package firmware

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record is the persisted outcome of the last successful check of one controller.
type Record struct {
	CheckedAt time.Time `json:"checked_at"`
	Status    Status    `json:"status"`
	Installed string    `json:"installed,omitempty"`
	Available string    `json:"available,omitempty"`
	Target    string    `json:"target,omitempty"`
	Source    string    `json:"source,omitempty"`
	// Notified is the available version the user was last notified about,
	// so the same version is announced only once.
	Notified string `json:"notified,omitempty"`
}

type storeFile struct {
	Version int               `json:"version"`
	Devices map[string]Record `json:"devices"`
}

// Store persists check records in a small JSON file (firmware.json).
// A nil *Store or an empty path keeps records in memory only.
type Store struct {
	mu      sync.Mutex
	path    string
	records map[string]Record
	loaded  bool
}

// NewStore keeps records in dir/firmware.json ("" = memory only).
func NewStore(dir string) *Store {
	st := &Store{records: map[string]Record{}}
	if dir != "" {
		st.path = filepath.Join(dir, "firmware.json")
	}
	return st
}

// Path returns the file location ("" for memory-only stores).
func (st *Store) Path() string { return st.path }

func (st *Store) load() {
	if st.loaded {
		return
	}
	st.loaded = true
	if st.path == "" {
		return
	}
	data, err := os.ReadFile(st.path)
	if err != nil {
		return
	}
	var f storeFile
	if json.Unmarshal(data, &f) == nil && f.Devices != nil {
		st.records = f.Devices
	}
}

// Get returns the record for a controller.
func (st *Store) Get(key string) (Record, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.load()
	r, ok := st.records[key]
	return r, ok
}

// Put stores a record and writes the file atomically.
func (st *Store) Put(key string, r Record) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.load()
	st.records[key] = r
	return st.save()
}

func (st *Store) save() error {
	if st.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(storeFile{Version: 1, Devices: st.records}, "", "  ")
	if err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, st.path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}
