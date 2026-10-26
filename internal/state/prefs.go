package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Preferences are user choices persisted across sessions in settings.json.
type Preferences struct {
	Mode           string            `json:"mode"`
	Language       string            `json:"language,omitempty"`
	LastDevice     string            `json:"last_device,omitempty"`
	ActiveProfiles map[string]string `json:"active_profiles,omitempty"` // device family -> profile name
	HighContrast   bool              `json:"high_contrast,omitempty"`
	AutoRotate     *bool             `json:"auto_rotate,omitempty"`
	// FirmwareAutoCheck enables the daily read-only firmware check (default on).
	FirmwareAutoCheck *bool `json:"firmware_auto_check,omitempty"`
	// ConfirmedModels remembers the user's controller identifications:
	// non-unique hardware key "vid:pid:version" → model ID.
	ConfirmedModels map[string]string `json:"confirmed_models,omitempty"`
}

// PrefsStore reads and writes Preferences at a fixed path.
type PrefsStore struct {
	path string
}

// NewPrefsStore stores preferences as dir/settings.json.
func NewPrefsStore(dir string) *PrefsStore {
	return &PrefsStore{path: filepath.Join(dir, "settings.json")}
}

// Path returns the settings file location.
func (ps *PrefsStore) Path() string { return ps.path }

// Load returns stored preferences, or defaults if the file is missing.
func (ps *PrefsStore) Load() (Preferences, error) {
	p := Preferences{Mode: string(ModeEZ)}
	data, err := os.ReadFile(ps.path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return Preferences{Mode: string(ModeEZ)}, err
	}
	return p, nil
}

// Save writes preferences atomically.
func (ps *PrefsStore) Save(p Preferences) error {
	if err := os.MkdirAll(filepath.Dir(ps.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := ps.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, ps.path)
}
