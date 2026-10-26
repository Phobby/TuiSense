package diag

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
)

// ReportDevice identifies the controller without unique identifiers by default.
type ReportDevice struct {
	Model         string            `json:"model"`
	ModelID       string            `json:"model_id"`
	Brand         string            `json:"brand"`
	Firmware      string            `json:"firmware,omitempty"`
	FirmwareBuild string            `json:"firmware_build,omitempty"`
	Hardware      string            `json:"hardware,omitempty"`
	Connection    string            `json:"connection"`
	Simulated     bool              `json:"simulated"`
	Identifiers   map[string]string `json:"identifiers,omitempty"` // only with explicit opt-in
}

// ReportEntry is one category in a report.
type ReportEntry struct {
	RequiredConnection string                `json:"required_connection,omitempty"`
	KeyMeasurements    []Measurement         `json:"key_measurements,omitempty"`
	Completion         Completion            `json:"completion"`
	Components         []ComponentCompletion `json:"components,omitempty"`
	Assessments        []Assessment          `json:"assessments"`
	Category           Category              `json:"category"`
	Label              string                `json:"label"`
	Status             Status                `json:"status"`
	Reason             string                `json:"reason,omitempty"` // for unsupported/unavailable
	Result             *Result               `json:"result,omitempty"`
}

// Report is the exportable diagnostics summary.
type Report struct {
	InputMapping   ReportInputMapping `json:"input_mapping"`
	SchemaVersion  string             `json:"test_schema_version"`
	Coverage       Coverage           `json:"coverage"`
	MissingFields  []string           `json:"missing_or_unsupported_fields"`
	Algorithm      string             `json:"algorithm_version"`
	Mapping        string             `json:"mapping_fingerprint"`
	Method         string             `json:"method"`
	InputMode      string             `json:"input_mode"`
	Events         []Event            `json:"events,omitempty"`
	Tool           string             `json:"tool"`
	Version        string             `json:"version"`
	GeneratedAt    time.Time          `json:"generated_at"`
	Device         ReportDevice       `json:"device"`
	SupportedTests []Category         `json:"supported_tests"`
	Entries        []ReportEntry      `json:"entries"`
	Notes          []string           `json:"notes"`
}

// BuildReport assembles a report. Raw audio is never included; unique
// identifiers (MAC, device path) only when includeIDs is true.
func BuildReport(s *Session, version string, includeIDs bool, now time.Time) Report {
	info := s.Ctx.Info
	if version == "" {
		version = s.SoftwareVersion
	}
	method := "buttons,sticks,triggers"
	if s.Sticks != nil {
		method = fmt.Sprintf("rest=%s; circularity=%v; preliminary=%v; event_driven=%v; language=%s", s.Sticks.Duration, s.Sticks.CircularityOnly, s.Sticks.Preliminary(), info.EventDriven, i18n.Current())
	}
	r := Report{
		SchemaVersion: ReportSchemaVersion, Algorithm: AlgorithmVersion, Mapping: MappingFingerprint(s.Ctx.Desc.Inputs), Method: method, InputMode: info.InputMode, Events: append([]Event(nil), s.Events...),
		Tool: i18n.T("TuiSense"), Version: version, GeneratedAt: now,
		Device: ReportDevice{
			Model: s.Ctx.Desc.DisplayName, ModelID: s.Ctx.Desc.ModelID, Brand: s.Ctx.Desc.Brand,
			Firmware: info.FirmwareVersion, FirmwareBuild: info.FirmwareBuild, Hardware: info.HardwareVersion,
			Connection: string(info.Connection), Simulated: s.Ctx.Simulated,
		},
		Notes: []string{
			i18n.T("Statuses are only 'Passed' when a measurement or explicit user confirmation was performed."),
			i18n.T("Results marked user_confirmed rely on what the user felt or heard."),
			i18n.T("No microphone recordings are stored or included."),
		},
	}
	layout := s.Ctx.Desc.Inputs
	triggerMapping := "unsupported"
	if layout.HasTriggers() {
		triggerMapping = string(layout.TriggerKind)
		if layout.TriggerKind == device.TriggerAnalog {
			triggerMapping = "analog"
		}
	}
	r.InputMapping = ReportInputMapping{TriggerKind: triggerMapping, Buttons: append([]device.ButtonDef(nil), layout.Buttons...), Source: "Driver-declared layout; nominal ranges and centers are not hardware calibration."}
	if layout.HasSticks() && s.Ctx.Caps.Supports(device.CapSticks) {
		for i := 0; i < 4; i++ {
			r.InputMapping.Axes = append(r.InputMapping.Axes, MappedAxis{Label: layout.StickLabels[i/2], Name: layout.AxisNames[i], Range: layout.StickAxis(i), Valid: layout.StickAxis(i).Valid()})
		}
	}
	if layout.HasTriggers() && layout.TriggerKind != device.TriggerDigital {
		for i := 0; i < 2; i++ {
			r.InputMapping.Axes = append(r.InputMapping.Axes, MappedAxis{Label: layout.TriggerLabels[i], Name: layout.AxisNames[i+4], Range: layout.TriggerAxis(i), Valid: layout.TriggerAxis(i).Valid()})
		}
	}
	if s.Ctx.Info.EventDriven {
		r.Notes = append(r.Notes, "Event-driven input can include held states. Resting observation counts are not necessarily independent hardware samples; kernel filtering can hide small noise. Fresh-event frequency is not a hardware polling rate.")
	}
	if s.Ctx.Simulated {
		r.Notes = append(r.Notes, i18n.T("SIMULATED DEVICE: all values are synthetic mock data, not hardware measurements."))
	}
	if includeIDs {
		r.Device.Identifiers = map[string]string{"mac": info.MACAddress, "path": info.HIDRawPath}
	}
	for _, c := range AllCategories {
		availability := s.Avail[c]
		entry := ReportEntry{Category: c, Label: c.Label(), Status: s.Status(c), Reason: availability.Reason}
		if (c == CatAudioIn || c == CatAudioOut) && entry.Status == Unavailable && s.Ctx.Info.Connection == device.ConnBluetooth && s.Ctx.Desc.Inputs.Audio.RequiresUSB {
			entry.RequiredConnection = "USB"
		}
		if availability.Available() {
			r.SupportedTests = append(r.SupportedTests, c)
		}
		if result, ok := s.Results[c]; ok {
			clone := result
			clone.Details = append([]string(nil), result.Details...)
			clone.Limitations = append([]string(nil), result.Limitations...)
			clone.Measurements = nil
			for _, m := range result.Measurements {
				if validMeasurement(m) {
					clone.Measurements = append(clone.Measurements, m)
				} else {
					clone.Limitations = append(clone.Limitations, "An invalid numeric observation was excluded; this measurement is unavailable.")
				}
			}
			entry.Result = &clone
		}
		if availability.Available() {
			if entry.Result == nil {
				entry.Result = s.partialObservation(c, now)
				if entry.Result != nil && !s.Running(c) {
					entry.Status = Incomplete
				}
			}
			if observed := s.reportObservation(c, now); observed != nil {
				entry.Result = observed
				entry.Status = observed.Status
			}
		}
		assessEntry(s, &entry)
		setKeyMeasurements(s, &entry)
		r.Entries = append(r.Entries, entry)
	}
	finalizeReport(&r)
	if !includeIDs {
		redact := func(text string) string {
			for _, secret := range []string{info.MACAddress, info.ID, info.SysfsPath} {
				if len(secret) >= 4 {
					text = strings.ReplaceAll(text, secret, "[redacted]")
				}
			}
			return reportSerial.ReplaceAllString(reportMAC.ReplaceAllString(text, "[redacted]"), "identifier: [redacted]")
		}
		redactStrings(reflect.ValueOf(&r).Elem(), redact)
	}

	return r
}

var reportMAC = regexp.MustCompile(`(?i)\b[0-9a-f]{2}(?:[:-][0-9a-f]{2}){5}\b`)
var reportSerial = regexp.MustCompile(`(?i)\b(?:serial(?: number)?|mac(?: address)?)\s*[:=]\s*[^\s"\\]+`)

// WriteJSON writes the report as indented JSON.
func (r Report) WriteJSON(w io.Writer) error {
	r.Events = boundedReportEvents(r.Events)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// WriteText writes the full six-section report using plain text.
func (r Report) WriteText(w io.Writer) error { return r.writeDocument(w, false) }

// Export writes JSON, Markdown, and plain-text versions into dir and returns their paths.
func (r Report) Export(dir string) (jsonPath, textPath string, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	model := strings.NewReplacer(".", "-", " ", "-", "/", "-", "\\", "-").Replace(r.Device.ModelID)
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", "", err
	}
	base := filepath.Join(dir, fmt.Sprintf("tuisense-diag-%s-%s-%x", model, r.GeneratedAt.Format("20060102-150405.000000000"), nonce))
	write := func(path string, fn func(io.Writer) error) error {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		writeErr := fn(f)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	jsonPath, textPath = base+".json", base+".md"
	if err := write(jsonPath, r.WriteJSON); err != nil {
		return "", "", err
	}
	if err := write(textPath, r.WriteMarkdown); err != nil {
		return "", "", err
	}
	if err := write(base+".txt", r.WriteText); err != nil {
		return "", "", err
	}
	return jsonPath, textPath, nil
}

// WriteMarkdown writes a six-section report suitable for support and issues.
func (r Report) WriteMarkdown(w io.Writer) error { return r.writeDocument(w, true) }

func redactStrings(value reflect.Value, redact func(string) string) {
	if !value.IsValid() {
		return
	}
	switch value.Kind() {
	case reflect.String:
		if value.CanSet() {
			value.SetString(redact(value.String()))
		}
	case reflect.Pointer:
		if !value.IsNil() {
			redactStrings(value.Elem(), redact)
		}
	case reflect.Struct:
		if value.Type() == reflect.TypeOf(time.Time{}) {
			return
		}
		for i := 0; i < value.NumField(); i++ {
			if value.Field(i).CanSet() {
				redactStrings(value.Field(i), redact)
			}
		}
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			redactStrings(value.Index(i), redact)
		}
	}
}

func available(s string) string {
	if s == "" {
		return "Unavailable / not measured"
	}
	return s
}
func mdCell(s string) string { return strings.NewReplacer("|", "\\|", "\n", " ").Replace(s) }

// ReportInputMapping stores observed driver declarations, never estimated axes.
type ReportInputMapping struct {
	TriggerKind string             `json:"trigger_kind"`
	Axes        []MappedAxis       `json:"axes,omitempty"`
	Buttons     []device.ButtonDef `json:"buttons,omitempty"`
	Source      string             `json:"source"`
}
type MappedAxis struct {
	Label string           `json:"label"`
	Name  string           `json:"name,omitempty"`
	Range device.AxisRange `json:"range"`
	Valid bool             `json:"valid"`
}
