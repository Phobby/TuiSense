package diag

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
)

// AlgorithmVersion changes whenever interpretation or measurement semantics change.
const AlgorithmVersion = "diagnostics-2.2.0"
const historyLimit = 256

// MappingFingerprint binds comparisons to the exact declared mapping.
func MappingFingerprint(layout device.InputLayout) string {
	data, _ := json.Marshal(layout)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HistoryEntry holds a private local snapshot, never raw identifiers or audio.
type HistoryEntry struct {
	Report     *Report   `json:"report,omitempty"`
	Identity   string    `json:"local_identity"`
	Model      string    `json:"model"`
	Version    string    `json:"software_version"`
	Algorithm  string    `json:"algorithm_version"`
	Mapping    string    `json:"mapping"`
	Method     string    `json:"method"`
	Mode       string    `json:"input_mode"`
	Connection string    `json:"connection"`
	Simulated  bool      `json:"simulated"`
	At         time.Time `json:"at"`
	Results    []Result  `json:"results"`
}

// HistoryStore is explicit opt-in storage: only Save writes snapshots.
// Its salt and snapshots are private to this installation.
type HistoryStore struct{ Dir string }

func (h HistoryStore) identity(key string) (string, error) {
	if key == "" {
		return "", errors.New("no local device identity available")
	}
	if err := os.MkdirAll(h.Dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(h.Dir, "identity.salt")
	salt, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		salt = make([]byte, 32)
		if _, err = rand.Read(salt); err != nil {
			return "", err
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(e, os.ErrExist) {
			salt, err = os.ReadFile(path)
		} else if e != nil {
			return "", e
		} else {
			_, err = f.Write(salt)
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return "", err
	}
	if len(salt) != 32 {
		return "", errors.New("invalid local identity salt")
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (h HistoryStore) Load() ([]HistoryEntry, error) {
	file, err := os.Open(filepath.Join(h.Dir, "history.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 8<<20 {
		return nil, errors.New("history file exceeds size limit")
	}
	var entries []HistoryEntry
	err = json.Unmarshal(data, &entries)
	if len(entries) > historyLimit {
		entries = entries[len(entries)-historyLimit:]
	}
	return entries, err
}

// Snapshot computes an installation-specific identity. Call off the UI thread.
func (h HistoryStore) Snapshot(r Report, key string) (HistoryEntry, error) {
	id, err := h.identity(r.Device.ModelID + ":" + key)
	entry := HistoryEntry{Report: &r, Identity: id, Model: r.Device.Model, Version: r.Version, Algorithm: r.Algorithm, Mapping: r.Mapping, Method: r.Method, Mode: r.InputMode, Connection: r.Device.Connection, Simulated: r.Device.Simulated, At: r.GeneratedAt}
	for _, e := range r.Entries {
		if e.Result != nil && e.Status != Testing {
			result := *e.Result
			if e.Completion != "" && e.Completion != CompletionDone {
				result.Status = Incomplete
			}
			entry.Results = append(entry.Results, result)
		}
	}
	return entry, err
}
func (h HistoryStore) Save(entry HistoryEntry) error {
	if len(entry.Results) == 0 && entry.Report == nil {
		return errors.New("no completed observations to save")
	}
	entries, err := h.Load()
	if err != nil {
		return err
	}
	entries = append(entries, entry)
	if len(entries) > historyLimit {
		entries = entries[len(entries)-historyLimit:]
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	for len(data) > 8<<20 && len(entries) > 1 {
		entries = entries[max(1, len(entries)/4):]
		data, err = json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return err
		}
	}
	if len(data) > 8<<20 {
		return errors.New("diagnostic snapshot exceeds history size limit")
	}
	if err = os.MkdirAll(h.Dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(h.Dir, "history-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(h.Dir, "history.json"))
}
func (h HistoryStore) Clear() error {
	err := os.Remove(filepath.Join(h.Dir, "history.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// CompareHistory compares only matching methods and actual shared measurements.
// A pair of observations does not establish hardware deterioration.
func CompareHistory(previous, current HistoryEntry) []string {
	if previous.Identity != current.Identity || previous.Simulated != current.Simulated {
		return []string{"Different local device or simulated source; comparison unavailable."}
	}
	if previous.Algorithm != current.Algorithm || previous.Mapping != current.Mapping || previous.Method != current.Method || previous.Mode != current.Mode || previous.Connection != current.Connection {
		return []string{"Method, algorithm, mapping, input mode, or connection changed; historical values are not directly comparable."}
	}
	lines := []string{"Previous: " + previous.At.Format(time.RFC3339) + ". Two observations do not establish a deterioration trend."}
	for _, cur := range current.Results {
		for _, prev := range previous.Results {
			if cur.Category != prev.Category || (cur.Status != Passed && cur.Status != Warning) || (prev.Status != Passed && prev.Status != Warning) {
				continue
			}
			for _, cm := range cur.Measurements {
				for _, pm := range prev.Measurements {
					if cm.Name == pm.Name && cm.Unit == pm.Unit {
						lines = append(lines, fmt.Sprintf("%s: previous %.3f → current %.3f; difference %+.3f %s", cm.Name, pm.Value, cm.Value, cm.Value-pm.Value, cm.Unit))
					}
				}
			}
		}
	}
	if len(lines) == 1 {
		lines = append(lines, "No comparable completed measurements. Warnings and incomplete tests require a controlled repeat.")
	}
	return lines
}

// StoredReport preserves the original assessment and algorithm. Legacy snapshots
// are shown as limited observations; new rules are never applied retroactively.
func (e HistoryEntry) StoredReport() Report {
	if e.Report != nil {
		return *e.Report
	}
	r := Report{SchemaVersion: "legacy", Tool: "TuiSense", Version: e.Version, Algorithm: e.Algorithm, Mapping: e.Mapping, Method: e.Method, InputMode: e.Mode, GeneratedAt: e.At, Device: ReportDevice{Model: e.Model, Connection: e.Connection, Simulated: e.Simulated}, Notes: []string{"Legacy history did not store protocol completion or assessments. Its measurements cannot establish complete test coverage."}}
	for _, result := range e.Results {
		result := result
		entry := ReportEntry{Category: result.Category, Label: result.Category.Label(), Status: Inconclusive, Result: &result, Completion: CompletionPartial}
		entry.Assessments = []Assessment{genericAssessment(entry)}
		entry.Assessments[0].Status = Inconclusive
		entry.Assessments[0].EvidenceKind = "inconclusive_evidence"
		entry.Assessments[0].Limitations = append(entry.Assessments[0].Limitations, r.Notes[0])
		r.Entries = append(r.Entries, entry)
	}
	finalizeReport(&r)
	return r
}
