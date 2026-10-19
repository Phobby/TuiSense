package diag

import (
	"fmt"
	"math"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
)

// ---- Touchpad --------------------------------------------------------------

const touchCols, touchRows = 4, 3

// TouchTest tracks contacts, coverage, and clicks — touch detection and the
// physical click switch are reported separately.
type TouchTest struct {
	Layout       device.InputLayout
	Contacts     [2]device.TouchPoint
	Trails       [2][][2]float64 // normalized 0..1 positions
	Visited      [touchRows][touchCols]bool
	MaxSimul     int
	Touches      int // new contact events
	Clicks       int
	ClickHeld    bool
	ClickNoTouch int // clicks registered while no contact was reported
	prevActive   [2]bool
	prevClick    bool
}

// NewTouchTest creates the touchpad test.
func NewTouchTest(layout device.InputLayout) *TouchTest { return &TouchTest{Layout: layout} }

// Reset clears observations.
func (t *TouchTest) Reset() { *t = TouchTest{Layout: t.Layout} }

// Feed processes one sample.
func (t *TouchTest) Feed(tel device.Telemetry) {
	w, h := float64(max(t.Layout.TouchpadSize[0], 1)), float64(max(t.Layout.TouchpadSize[1], 1))
	n := 0
	for i := 0; i < min(2, t.Layout.TouchpadContacts); i++ {
		c := tel.Touch[i]
		t.Contacts[i] = c
		if c.Active {
			n++
			if !t.prevActive[i] {
				t.Touches++
			}
			x, y := math.Min(float64(c.X)/w, 0.999), math.Min(float64(c.Y)/h, 0.999)
			t.Visited[int(y*touchRows)][int(x*touchCols)] = true
			t.Trails[i] = append(t.Trails[i], [2]float64{x, y})
			if len(t.Trails[i]) > 60 {
				t.Trails[i] = t.Trails[i][1:]
			}
		}
		t.prevActive[i] = c.Active
	}
	t.MaxSimul = max(t.MaxSimul, n)
	click := (tel.ButtonsLatched | tel.Buttons).Has(device.ButtonTouchpadClick)
	if click && !t.prevClick {
		t.Clicks++
		if n == 0 {
			t.ClickNoTouch++
		}
	}
	t.ClickHeld = tel.Buttons.Has(device.ButtonTouchpadClick)
	t.prevClick = click
}

// Coverage returns visited cells / total.
func (t *TouchTest) Coverage() (int, int) {
	v := 0
	for _, row := range t.Visited {
		for _, c := range row {
			if c {
				v++
			}
		}
	}
	return v, touchRows * touchCols
}

// Result is produced once any touch or click was observed.
func (t *TouchTest) Result(now time.Time, simulated bool) *Result {
	if t.Touches == 0 && t.Clicks == 0 {
		return nil
	}
	v, total := t.Coverage()
	res := &Result{Category: CatTouchpad, Simulated: simulated, At: now}
	res.Measurements = []Measurement{
		{"Area covered", float64(v), fmt.Sprintf("of %d zones", total)},
		{"Max simultaneous contacts", float64(t.MaxSimul), fmt.Sprintf("of %d supported", t.Layout.TouchpadContacts)},
		{"Touch events", float64(t.Touches), ""},
		{"Clicks", float64(t.Clicks), ""},
	}
	switch {
	case v == total && t.Clicks > 0:
		res.Status, res.Summary = Passed, "Touch detected across the whole surface and the click switch works."
	case v < total:
		res.Status, res.Summary = Inconclusive, fmt.Sprintf("Touch works; %d of %d zones covered so far — slide over the whole pad.", v, total)
	default:
		res.Status, res.Summary = Inconclusive, "Touch works across the pad; the click switch was not pressed yet."
	}
	if t.MaxSimul >= 2 {
		res.Details = append(res.Details, "Multi-touch: two simultaneous contacts detected.")
	}
	if t.ClickNoTouch > 0 {
		res.Details = append(res.Details, fmt.Sprintf("%d click(s) registered without a reported contact (normal if pressed at the very edge).", t.ClickNoTouch))
	}
	return res
}

// ---- Motion ----------------------------------------------------------------

// MotionPhase is a step of the motion test.
type MotionPhase int

const (
	MotionIdle  MotionPhase = iota
	MotionStill             // resting on a flat surface
	MotionMove              // rotate on every axis
	MotionDone
)

const (
	motionStillDuration = 3 * time.Second
	motionMoveDuration  = 8 * time.Second
)

// MotionTest measures stillness noise/bias and axis response, in raw counts.
type MotionTest struct {
	Layout  device.InputLayout
	Gyro    [3]float64
	Accel   [3]float64
	History [3]*Ring // gyro raw per axis for graphs

	Phase      MotionPhase
	start, end time.Time
	stillGyro  [3]AxisStats
	stillAccel [3]AxisStats
	moveGyro   [3]float64 // max |gyro - bias|
	moveAccel  [3]AxisStats

	scenario func(device.InputScenario)
}

// NewMotionTest creates the motion test.
func NewMotionTest(layout device.InputLayout, scenario func(device.InputScenario)) *MotionTest {
	t := &MotionTest{Layout: layout, scenario: scenario}
	for i := range t.History {
		t.History[i] = NewRing(60)
	}
	return t
}

func (t *MotionTest) setScenario(s device.InputScenario) {
	if t.scenario != nil {
		t.scenario(s)
	}
}

// Feed processes one sample (raw values only).
func (t *MotionTest) Feed(tel device.Telemetry, now time.Time) {
	if !tel.Raw.Valid {
		return
	}
	for i := 0; i < 3; i++ {
		g, a := float64(tel.Raw.Gyro[i]), float64(tel.Raw.Accel[i])
		t.Gyro[i], t.Accel[i] = g, a
		t.History[i].Push(g)
		switch t.Phase {
		case MotionStill:
			t.stillGyro[i].Add(g)
			t.stillAccel[i].Add(a)
		case MotionMove:
			t.moveGyro[i] = math.Max(t.moveGyro[i], math.Abs(g-t.stillGyro[i].Mean()))
			t.moveAccel[i].Add(a)
		}
	}
	t.Tick(now)
}

// Start begins the still step.
func (t *MotionTest) Start(now time.Time) {
	t.stillGyro, t.stillAccel, t.moveAccel = [3]AxisStats{}, [3]AxisStats{}, [3]AxisStats{}
	t.moveGyro = [3]float64{}
	t.Phase, t.start, t.end = MotionStill, now, now.Add(motionStillDuration)
	t.setScenario(device.ScenarioStill)
}

// Cancel aborts the test.
func (t *MotionTest) Cancel() {
	if t.Running() {
		t.Phase = MotionIdle
		t.setScenario(device.ScenarioDemo)
	}
}

// Running reports whether a step is active.
func (t *MotionTest) Running() bool { return t.Phase == MotionStill || t.Phase == MotionMove }

// Progress returns 0..1 of the current step.
func (t *MotionTest) Progress(now time.Time) float64 {
	total := t.end.Sub(t.start)
	if total <= 0 {
		return 0
	}
	return math.Min(1, float64(now.Sub(t.start))/float64(total))
}

// Tick advances steps.
func (t *MotionTest) Tick(now time.Time) {
	if !t.Running() || now.Before(t.end) {
		return
	}
	if t.Phase == MotionStill {
		t.Phase, t.start, t.end = MotionMove, now, now.Add(motionMoveDuration)
		t.setScenario(device.ScenarioMotion)
		return
	}
	t.Phase = MotionDone
	t.setScenario(device.ScenarioDemo)
}

// axisThreshold is the movement needed to call a gyro axis responsive:
// well above the measured resting noise, with a floor for very quiet sensors.
func (t *MotionTest) axisThreshold(i int) float64 {
	return math.Max(10*t.stillGyro[i].Std(), 200)
}

// Result returns the motion result after both steps.
func (t *MotionTest) Result(now time.Time, simulated bool) *Result {
	if t.Phase != MotionDone {
		return nil
	}
	res := &Result{Category: CatMotion, Simulated: simulated, At: now}
	names := [3]string{"X (pitch)", "Y (yaw)", "Z (roll)"}
	responsive := 0
	var dead []string
	for i := 0; i < 3; i++ {
		res.Measurements = append(res.Measurements,
			Measurement{"Gyro " + names[i] + " resting bias", t.stillGyro[i].Mean(), "raw counts"},
			Measurement{"Gyro " + names[i] + " resting noise σ", t.stillGyro[i].Std(), "raw counts"},
			Measurement{"Gyro " + names[i] + " peak response", t.moveGyro[i], "raw counts"},
		)
		if t.moveGyro[i] > t.axisThreshold(i) {
			responsive++
		} else {
			dead = append(dead, names[i])
		}
	}
	if g := t.Layout.AccelLSBPerG; g > 0 {
		mag := math.Sqrt(sq(t.stillAccel[0].Mean())+sq(t.stillAccel[1].Mean())+sq(t.stillAccel[2].Mean())) / g
		res.Measurements = append(res.Measurements, Measurement{"Resting acceleration magnitude (nominal scale)", mag, "g"})
		if mag > 0.85 && mag < 1.15 {
			res.Details = append(res.Details, fmt.Sprintf("Accelerometer at rest reads ≈%.2f g — consistent with gravity.", mag))
		} else {
			res.Details = append(res.Details, fmt.Sprintf("Accelerometer at rest reads %.2f g using the nominal (uncalibrated) scale; this alone does not prove a fault.", mag))
		}
	}
	switch {
	case responsive == 3:
		res.Status, res.Summary = Passed, "All three gyroscope axes respond to movement."
	case responsive == 0:
		res.Status, res.Summary = Inconclusive, "No rotation detected — was the controller moved during step 2?"
	default:
		res.Status, res.Summary = Warning, fmt.Sprintf("Gyro axes without a clear response: %v — rotate on every axis and retest.", dead)
	}
	res.Limitations = []string{
		"Values are raw sensor counts; factory calibration data is not read, so TuiSense does not claim the sensors are calibrated.",
		"A responding axis shows the sensor reacts; it does not verify accuracy.",
	}
	return res
}

func sq(v float64) float64 { return v * v }
