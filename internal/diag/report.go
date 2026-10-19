package diag

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReportDevice identifies the controller without unique identifiers by default.
type ReportDevice struct {
	Model       string            `json:"model"`
	ModelID     string            `json:"model_id"`
	Brand       string            `json:"brand"`
	Firmware    string            `json:"firmware,omitempty"`
	Hardware    string            `json:"hardware,omitempty"`
	Connection  string            `json:"connection"`
	Simulated   bool              `json:"simulated"`
	Identifiers map[string]string `json:"identifiers,omitempty"` // only with explicit opt-in
}

// ReportEntry is one category in a report.
type ReportEntry struct {
	Category Category `json:"category"`
	Label    string   `json:"label"`
	Status   Status   `json:"status"`
	Reason   string   `json:"reason,omitempty"` // for unsupported/unavailable
	Result   *Result  `json:"result,omitempty"`
}

// Report is the exportable diagnostics summary.
type Report struct {
	Tool           string        `json:"tool"`
	Version        string        `json:"version"`
	GeneratedAt    time.Time     `json:"generated_at"`
	Device         ReportDevice  `json:"device"`
	SupportedTests []Category    `json:"supported_tests"`
	Entries        []ReportEntry `json:"entries"`
	Notes          []string      `json:"notes"`
}

// BuildReport assembles a report. Raw audio is never included; unique
// identifiers (MAC, device path) only when includeIDs is true.
func BuildReport(s *Session, version string, includeIDs bool, now time.Time) Report {
	info := s.Ctx.Info
	r := Report{
		Tool: "TuiSense", Version: version, GeneratedAt: now,
		Device: ReportDevice{
			Model: s.Ctx.Desc.DisplayName, ModelID: s.Ctx.Desc.ModelID, Brand: s.Ctx.Desc.Brand,
			Firmware: info.FirmwareVersion, Hardware: info.HardwareVersion,
			Connection: string(info.Connection), Simulated: s.Ctx.Simulated,
		},
		Notes: []string{
			"Statuses are only 'Passed' when a measurement or explicit user confirmation was performed.",
			"Results marked user_confirmed rely on what the user felt or heard.",
			"No microphone recordings are stored or included.",
		},
	}
	if s.Ctx.Simulated {
		r.Notes = append(r.Notes, "SIMULATED DEVICE: all values are synthetic mock data, not hardware measurements.")
	}
	if includeIDs {
		r.Device.Identifiers = map[string]string{"mac": info.MACAddress, "path": info.HIDRawPath}
	}
	for _, c := range AllCategories {
		a := s.Avail[c]
		e := ReportEntry{Category: c, Label: c.Label(), Status: s.Status(c), Reason: a.Reason}
		if a.Available() {
			r.SupportedTests = append(r.SupportedTests, c)
			if res, ok := s.Results[c]; ok {
				res := res
				e.Result = &res
			}
		}
		r.Entries = append(r.Entries, e)
	}
	return r
}

// WriteJSON writes the report as indented JSON.
func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteText writes a human-readable report.
func (r Report) WriteText(w io.Writer) error {
	var b strings.Builder
	fmt.Fprintf(&b, "TuiSense %s — Controller Diagnostics Report\n", r.Version)
	fmt.Fprintf(&b, "Generated: %s\n\n", r.GeneratedAt.Format(time.RFC1123))
	fmt.Fprintf(&b, "Controller: %s (%s, %s)\n", r.Device.Model, r.Device.Brand, r.Device.ModelID)
	fmt.Fprintf(&b, "Connection: %s\n", r.Device.Connection)
	if r.Device.Firmware != "" {
		fmt.Fprintf(&b, "Firmware:   %s (hardware %s)\n", r.Device.Firmware, r.Device.Hardware)
	}
	if r.Device.Simulated {
		b.WriteString("\n*** SIMULATED DEVICE — values are mock data, not hardware measurements ***\n")
	}
	b.WriteString("\n")
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "[%s] %-20s %s\n", e.Status.Icon(), e.Label, e.Status)
		if e.Reason != "" && e.Result == nil {
			fmt.Fprintf(&b, "      %s\n", e.Reason)
		}
		if e.Result != nil {
			fmt.Fprintf(&b, "      %s\n", e.Result.Summary)
			for _, d := range e.Result.Details {
				fmt.Fprintf(&b, "      - %s\n", d)
			}
			for _, m := range e.Result.Measurements {
				fmt.Fprintf(&b, "      · %s\n", m)
			}
			for _, l := range e.Result.Limitations {
				fmt.Fprintf(&b, "      (limitation) %s\n", l)
			}
		}
	}
	b.WriteString("\nNotes:\n")
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "  - %s\n", n)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// Export writes JSON and text versions into dir and returns their paths.
func (r Report) Export(dir string) (jsonPath, textPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	model := strings.NewReplacer(".", "-", " ", "-").Replace(r.Device.ModelID)
	base := filepath.Join(dir, fmt.Sprintf("tuisense-diag-%s-%s", model, r.GeneratedAt.Format("20060102-150405")))
	write := func(path string, fn func(io.Writer) error) error {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return fn(f)
	}
	jsonPath, textPath = base+".json", base+".txt"
	if err := write(jsonPath, r.WriteJSON); err != nil {
		return "", "", err
	}
	if err := write(textPath, r.WriteText); err != nil {
		return "", "", err
	}
	return jsonPath, textPath, nil
}
