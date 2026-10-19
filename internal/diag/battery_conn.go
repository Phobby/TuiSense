package diag

import (
	"fmt"
	"strings"
	"time"

	"github.com/ygzztrk/tuisense/internal/device"
)

// ---- Battery ---------------------------------------------------------------

// BatteryObservation is one recorded change of reported battery state.
type BatteryObservation struct {
	At       time.Time `json:"at"`
	Percent  int       `json:"percent"`
	Charging bool      `json:"charging"`
}

// BatteryMonitor shows reported battery telemetry and records changes over
// time. It never derives health, capacity, wear, or cycle counts.
type BatteryMonitor struct {
	Step         int
	Percent      int
	Charging     bool
	Full         bool
	Observations []BatteryObservation
	Details      []device.BatteryAttribute
	detailsAt    time.Time
	provider     device.BatteryDetailsProvider
	samples      int
}

// NewBatteryMonitor creates the monitor.
func NewBatteryMonitor(step int, provider device.BatteryDetailsProvider) *BatteryMonitor {
	return &BatteryMonitor{Step: step, provider: provider}
}

// Feed records the reported level; observations are appended on change only.
func (b *BatteryMonitor) Feed(tel device.Telemetry, now time.Time) {
	b.samples++
	b.Percent, b.Charging, b.Full = tel.BatteryPercent, tel.Charging, tel.FullyCharged
	n := len(b.Observations)
	if n == 0 || b.Observations[n-1].Percent != tel.BatteryPercent || b.Observations[n-1].Charging != tel.Charging {
		b.Observations = append(b.Observations, BatteryObservation{At: now, Percent: tel.BatteryPercent, Charging: tel.Charging})
		if len(b.Observations) > 500 {
			b.Observations = b.Observations[1:]
		}
	}
	if b.provider != nil && now.Sub(b.detailsAt) > 5*time.Second {
		b.Details, b.detailsAt = b.provider.BatteryDetails(), now
	}
}

// Health returns a reported state-of-health value only if the device/OS
// genuinely exposes one; otherwise ok=false.
func (b *BatteryMonitor) Health() (string, bool) {
	for _, d := range b.Details {
		if n := strings.ToLower(d.Name); n == "health" || n == "state_of_health" {
			return d.Value + " (" + d.Source + ")", true
		}
	}
	return "", false
}

// ObservationSummary describes observed change (not a health measurement).
func (b *BatteryMonitor) ObservationSummary(now time.Time) string {
	if len(b.Observations) == 0 {
		return "No observations yet."
	}
	first := b.Observations[0]
	d := now.Sub(first.At).Round(time.Second)
	return fmt.Sprintf("%+d%% over %s (%d change(s) observed) — an observation, not a health measurement.",
		b.Percent-first.Percent, d, len(b.Observations)-1)
}

// HealthUnavailable is the exact wording required when no health value exists.
const HealthUnavailable = "Battery health: Not available from this device"

// Result reports what the device actually exposes.
func (b *BatteryMonitor) Result(now time.Time, simulated bool) *Result {
	if b.samples < 2 {
		return nil
	}
	res := &Result{Category: CatBattery, Simulated: simulated, At: now, Status: Passed}
	state := "discharging"
	switch {
	case b.Full:
		state = "full"
	case b.Charging:
		state = "charging"
	}
	res.Summary = fmt.Sprintf("Battery telemetry is reported: %d%%, %s.", b.Percent, state)
	res.Measurements = []Measurement{{"Reported level", float64(b.Percent), "%"}}
	if h, ok := b.Health(); ok {
		res.Details = append(res.Details, "Battery health (reported): "+h)
	} else {
		res.Details = append(res.Details, HealthUnavailable)
	}
	for _, d := range b.Details {
		res.Details = append(res.Details, fmt.Sprintf("%s = %s (%s)", d.Name, d.Value, d.Source))
	}
	res.Details = append(res.Details, "Observed change: "+b.ObservationSummary(now))
	if b.Step > 1 {
		res.Limitations = append(res.Limitations, fmt.Sprintf("The controller reports battery level in %d%% steps.", b.Step))
	}
	res.Limitations = append(res.Limitations, "Capacity, cycle count, voltage, current, and temperature are shown only when the device or OS actually reports them.")
	return res
}

// ---- Connection ------------------------------------------------------------

type rxSample struct {
	at time.Time
	rx uint64
}

// ConnMonitor measures input report rate and counts interruptions.
type ConnMonitor struct {
	started       time.Time
	window        []rxSample
	RateHz        float64
	Interruptions int
	Reconnections int
	Dropped       uint64
	ReadErrors    uint64
	Stalls        int // gaps > 0.5 s without new telemetry while connected
	lastUpdate    time.Time
	connected     bool
	seen          bool
	Info          device.Info
}

// NewConnMonitor creates the monitor.
func NewConnMonitor(now time.Time) *ConnMonitor { return &ConnMonitor{started: now} }

// Feed processes one sample.
func (c *ConnMonitor) Feed(tel device.Telemetry, info device.Info, now time.Time) {
	c.Info = info
	if c.seen && c.connected && !info.Connected {
		c.Interruptions++
	}
	if c.seen && !c.connected && info.Connected {
		c.Reconnections++
	}
	c.seen, c.connected = true, info.Connected
	if !info.Connected {
		return
	}
	if !c.lastUpdate.IsZero() && !tel.UpdatedAt.IsZero() && tel.UpdatedAt.Sub(c.lastUpdate) > 500*time.Millisecond {
		c.Stalls++
	}
	if !tel.UpdatedAt.IsZero() {
		c.lastUpdate = tel.UpdatedAt
	}
	c.Dropped, c.ReadErrors = tel.ReportsDropped, tel.ReadErrors
	c.window = append(c.window, rxSample{now, tel.PacketsRx})
	for len(c.window) > 2 && now.Sub(c.window[0].at) > 2*time.Second {
		c.window = c.window[1:]
	}
	if n := len(c.window); n >= 2 {
		dt := c.window[n-1].at.Sub(c.window[0].at).Seconds()
		if dt > 0 {
			c.RateHz = float64(c.window[n-1].rx-c.window[0].rx) / dt
		}
	}
}

// Observed returns how long the monitor has been running.
func (c *ConnMonitor) Observed(now time.Time) time.Duration { return now.Sub(c.started) }

// MarkDisconnected records an interruption reported by the session layer.
func (c *ConnMonitor) MarkDisconnected() {
	if c.connected {
		c.Interruptions++
	}
	c.connected = false
}

// Result is available after ≥3 s of observation.
func (c *ConnMonitor) Result(now time.Time, simulated bool) *Result {
	if c.Observed(now) < 3*time.Second || !c.seen {
		return nil
	}
	res := &Result{Category: CatConnection, Simulated: simulated, At: now}
	res.Measurements = []Measurement{
		{"Input report rate", c.RateHz, "Hz"},
		{"Reports missing in sequence", float64(c.Dropped), ""},
		{"Read errors", float64(c.ReadErrors), ""},
		{"Interruptions", float64(c.Interruptions), ""},
		{"Reconnections", float64(c.Reconnections), ""},
		{"Telemetry stalls > 0.5 s", float64(c.Stalls), ""},
	}
	var issues []string
	if c.Dropped > 0 {
		issues = append(issues, fmt.Sprintf("%d input reports missing (sequence gaps) — typical of Bluetooth interference or a loose cable", c.Dropped))
	}
	if c.ReadErrors > 0 {
		issues = append(issues, fmt.Sprintf("%d read errors", c.ReadErrors))
	}
	if c.Interruptions > 0 {
		issues = append(issues, fmt.Sprintf("%d interruption(s)", c.Interruptions))
	}
	switch {
	case !c.connected:
		res.Status, res.Summary = Failed, "Controller is currently disconnected."
	case c.RateHz <= 0:
		res.Status, res.Summary = Warning, "Connected, but no input reports were received during observation."
	case len(issues) > 0:
		res.Status, res.Summary = Warning, "Connection issues observed: "+strings.Join(issues, "; ")+"."
	default:
		res.Status = Passed
		res.Summary = fmt.Sprintf("Stable %s connection, receiving input reports at ~%.0f Hz.", c.Info.Connection, c.RateHz)
	}
	res.Limitations = []string{
		"Latency is not measured: meaningful end-to-end latency needs external timing hardware.",
		"Wireless signal strength and USB power draw are not reported by the controller and are not estimated.",
		"Report rate is counted on the host; with simulated devices it reflects the poll rate.",
	}
	return res
}
