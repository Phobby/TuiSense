package diag

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/audio"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/xboxmock"
)

var t0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

// fakeActuator records commands and returns a fixed user config.
type fakeActuator struct {
	cfg  device.Config
	cmds []device.Command
}

func (f *fakeActuator) Dispatch(c device.Command)    { f.cmds = append(f.cmds, c) }
func (f *fakeActuator) CurrentConfig() device.Config { return f.cfg }

func dsContext() DeviceContext {
	d := dualsense.Descriptor()
	return DeviceContext{Key: "hw", Desc: d, Caps: d.Capabilities, Info: device.Info{Connected: true, Connection: device.ConnUSB}}
}

func TestButtonGuidedAndLatchedPresses(t *testing.T) {
	layout := xboxmock.Descriptor().Inputs
	bt := NewButtonTest(layout, nil)
	bt.StartGuided()
	now := t0
	for i, b := range layout.Buttons {
		if i == 3 {
			bt.SkipCurrent() // user could not press Y
			continue
		}
		// press happens entirely between polls: only the latched set shows it
		bt.Feed(device.Telemetry{ButtonsLatched: device.ButtonSet(0).With(b.ID)}, now)
		now = now.Add(40 * time.Millisecond)
		bt.Feed(device.Telemetry{}, now)
	}
	if !bt.GuidedDone() {
		t.Fatalf("guided run should be complete")
	}
	res := bt.Result(now, true)
	if res == nil || res.Status != Incomplete || !strings.Contains(strings.Join(res.Details, " "), "Y") {
		t.Fatalf("skipped button must be reported, got %+v", res)
	}
	if bt.Counts[device.ButtonSouth] != 1 {
		t.Fatalf("latched press should count once, got %d", bt.Counts[device.ButtonSouth])
	}
}

func TestButtonStuckObservation(t *testing.T) {
	bt := NewButtonTest(dualsense.Descriptor().Inputs, nil)
	held := device.ButtonSet(0).With(device.ButtonL1)
	for i := 0; i < 200; i++ {
		bt.Feed(device.Telemetry{Buttons: held, ButtonsLatched: held}, t0.Add(time.Duration(i)*40*time.Millisecond))
	}
	if bt.Stuck[device.ButtonL1] < 5*time.Second || bt.Counts[device.ButtonL1] != 1 {
		t.Fatalf("long hold should be observed as possibly stuck and counted once")
	}
}

func TestHapticPatternsBoundedAndStopCleansUp(t *testing.T) {
	act := &fakeActuator{cfg: device.DefaultConfig()}
	ht := NewHapticTest(dualsense.Descriptor().Capabilities, act)
	ht.Intensity = 2
	if !ht.Play("pulse", t0) {
		t.Fatal("pattern not found")
	}
	ht.Tick(t0.Add(350 * time.Millisecond))
	ht.Stop()
	last := act.cmds[len(act.cmds)-1]
	if last.Type != device.CmdSetHaptics || last.Config.Haptics != act.cfg.Haptics {
		t.Fatalf("stop must restore the user's haptics config, got %+v", last)
	}
	for _, c := range act.cmds {
		if c.Type == device.CmdPulseHaptics && (c.PulseDur > 400*time.Millisecond || c.PulseLeft > 230 || !c.PulseAbsolute) {
			t.Fatalf("unbounded pulse: %+v", c)
		}
	}
	if ht.Result(t0, false) != nil {
		t.Fatalf("no result without user feedback")
	}
	ht.Feedback["pulse"] = false
	if r := ht.Result(t0, false); r.Status != Warning || !r.UserConfirmed {
		t.Fatalf("not-felt feedback must warn and be marked user-confirmed")
	}

	xb := NewHapticTest(xboxmock.Descriptor().Capabilities, act)
	found := false
	for _, p := range xb.Patterns {
		found = found || p.ID == "triggers"
	}
	if !found {
		t.Fatalf("Xbox impulse trigger pattern should be offered")
	}
}

func TestAudioEndpointSelectionNeverPicksBuiltIn(t *testing.T) {
	root := t.TempDir()
	write := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		_ = os.WriteFile(filepath.Join(root, p), []byte(s), 0o644)
	}
	write("cards", ` 0 [Generic        ]: HDA-Intel - HD-Audio Generic
                      HD-Audio Generic at 0xf6500000 irq 100
 1 [Webcam         ]: USB-Audio - USB Webcam
                      Generic USB Webcam
 2 [Controller     ]: USB-Audio - Wireless Controller
                      Sony Interactive Entertainment Wireless Controller at usb-0000:00:14.0-2, full speed
`)
	write("card0/pcm0c/info", "x")
	write("card0/pcm0p/info", "x")
	write("card1/usbid", "046d:0825\n")
	write("card1/stream0", "Capture:\n  Channels: 1\n")
	write("card2/usbid", "054c:0ce6\n")
	write("card2/stream0", "Playback:\n  Status: Stop\n  Interface 1\n    Channels: 4\nCapture:\n  Interface 2\n    Channels: 2\n")

	alsa := &audio.ALSA{ProcRoot: root}
	m := dualsense.Descriptor().Inputs.Audio
	ep, ok, err := audio.FindControllerEndpoint(alsa, m)
	if err != nil || !ok {
		t.Fatalf("expected the DualSense card, err=%v", err)
	}
	if ep.CardID != "Controller" || ep.PCM != "plughw:CARD=Controller,DEV=0" || ep.PlaybackChannels != 4 || ep.CaptureChannels != 2 {
		t.Fatalf("wrong endpoint selected: %+v", ep)
	}

	write("card2/usbid", "1234:5678\n") // no DualSense present
	if _, ok, _ := audio.FindControllerEndpoint(alsa, m); ok {
		t.Fatalf("must not fall back to the built-in or another USB card")
	}
}

func TestAudioInterleaveKeepsHapticChannelsSilent(t *testing.T) {
	pcm := audio.Interleave([]int16{100, 200}, 4, 2)
	if len(pcm) != 8 || pcm[0] != 100 || pcm[1] != 100 || pcm[2] != 0 || pcm[3] != 0 || pcm[6] != 0 {
		t.Fatalf("channels 3–4 must stay silent: %v", pcm)
	}
}

func TestPlanCapabilityDriven(t *testing.T) {
	ds := dsContext()
	p := Plan(ds)
	for _, c := range []Category{CatSticks, CatButtons, CatTriggers, CatTouchpad, CatMotion, CatHaptics, CatBattery, CatConnection} {
		if !p[c].Available() {
			t.Errorf("DualSense should support %s: %+v", c, p[c])
		}
	}
	if p[CatAudioIn].Status != Unavailable || !strings.Contains(p[CatAudioIn].Reason, "054c:0ce6") {
		t.Errorf("without an audio card the mic must be unavailable with the searched USB ID, got %+v", p[CatAudioIn])
	}

	bt := dsContext()
	bt.Info.Connection = device.ConnBluetooth
	bt.Caps = bt.Desc.Capabilities.WithoutFeature(device.CapAudio, device.FeatSpeaker, "")
	p = Plan(bt)
	if p[CatAudioIn].Status != Inaccessible || !p[CatAudioIn].Hidden() || !strings.Contains(p[CatAudioIn].Reason, "USB") {
		t.Errorf("over Bluetooth audio exists but is out of reach: hidden, with the USB requirement kept for Driver & Compatibility, got %+v", p[CatAudioIn])
	}

	x := xboxmock.Descriptor()
	p = Plan(DeviceContext{Desc: x, Caps: x.Capabilities, Info: device.Info{Connected: true}, Simulated: true})
	for _, c := range []Category{CatTouchpad, CatMotion, CatAudioIn} {
		if p[c].Status != Unsupported {
			t.Errorf("Xbox test device must not offer %s, got %+v", c, p[c])
		}
	}
	if !p[CatSticks].Available() || !p[CatTriggers].Available() || !p[CatButtons].Available() {
		t.Errorf("Xbox test device should offer sticks, triggers and buttons")
	}

	off := dsContext()
	off.Info.Connected = false
	p = Plan(off)
	if p[CatSticks].Status != Unavailable {
		t.Errorf("disconnected controller: tests must be unavailable")
	}
}

func TestMicTestDetectsSignalAndSilence(t *testing.T) {
	sim := audio.NewSimulated("Test mic", 1, 2)
	ep := sim.Eps[0]
	mic := NewMicTest(sim, ep, ep, true)
	start := time.Now()
	mic.Start(start)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r := mic.Result(time.Now(), true, ""); r != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !mic.Record() {
		t.Fatalf("recording should start while monitoring")
	}
	time.Sleep(3200 * time.Millisecond)
	if _, _, _, _, _, rec, _ := mic.Snapshot(); rec != RecReady {
		t.Fatalf("expected a ready recording, got %v", rec)
	}
	if err := mic.Playback(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3200 * time.Millisecond)
	r := mic.Result(time.Now(), true, "")
	mic.Stop()
	if r == nil || r.Status != Passed {
		t.Fatalf("simulated speech should pass, got %+v", r)
	}
	if sim.PlayedCount() != 1 {
		t.Fatalf("playback should go to the controller endpoint once")
	}
	if _, _, _, _, _, rec, _ := mic.Snapshot(); rec != RecNone {
		t.Fatalf("stop must discard the recording")
	}

	silent := audio.NewSimulated("Muted mic", 1, 0)
	silent.Signal = func(float64) float64 { return 0 }
	m2 := NewMicTest(silent, silent.Eps[0], audio.Endpoint{}, false)
	m2.Start(time.Now())
	time.Sleep(2300 * time.Millisecond)
	r2 := m2.Result(time.Now(), true, "")
	m2.Stop()
	if r2 == nil || r2.Status != Warning {
		t.Fatalf("digital silence should warn, got %+v", r2)
	}
}

func TestSessionDisconnectSuspendsEffects(t *testing.T) {
	act := &fakeActuator{cfg: device.DefaultConfig()}
	s := NewSession(dsContext(), act, nil, nil, nil, t0)
	s.Haptics.Play("both", t0)
	s.Triggers.StartResistance(t0)
	s.Sticks.Start(t0)
	n := len(act.cmds)
	s.Feed(device.Telemetry{}, device.Info{Connected: false, Connection: device.ConnUSB}, t0.Add(time.Second))
	if s.Triggers.ResistanceActive || s.Haptics.Running != nil || s.Sticks.Running() {
		t.Fatalf("disconnect must stop every active effect and measurement")
	}
	if len(act.cmds) <= n {
		t.Fatalf("cleanup commands should have been issued")
	}
	if s.Status(CatSticks) != Unavailable {
		t.Fatalf("tests must become unavailable while disconnected, got %v", s.Status(CatSticks))
	}
}

func TestSessionStatusesAndGuidedRun(t *testing.T) {
	x := xboxmock.Descriptor()
	s := NewSession(DeviceContext{Desc: x, Caps: x.Capabilities, Info: device.Info{Connected: true}, Simulated: true}, &fakeActuator{}, nil, nil, nil, t0)
	if s.Status(CatSticks) != NotTested || s.Status(CatTouchpad) != Unsupported {
		t.Fatalf("unexpected initial statuses")
	}
	for _, c := range s.Visible() {
		if c == CatTouchpad || c == CatMotion {
			t.Fatalf("unsupported categories must not be listed: %s", c)
		}
	}
	s.StartGuided()
	if c, _ := s.Guided.Current(); c != CatConnection || s.Active != CatConnection {
		t.Fatalf("guided run should start with connection")
	}
	for s.Guided != nil {
		s.NextGuided()
	}
	if s.Active != CatHealthReport || s.FinalReport == nil {
		t.Fatalf("finished guided run should display a health report")
	}
	// Nothing was measured: nothing may be marked passed.
	for _, c := range AllCategories {
		if s.Status(c) == Passed {
			t.Fatalf("%s marked passed without a verification", c)
		}
	}
}

func TestReportContentAndPrivacy(t *testing.T) {
	dc := dsContext()
	dc.Info.MACAddress, dc.Info.FirmwareVersion = "AA:BB:CC:DD:EE:FF", "0458"
	s := NewSession(dc, &fakeActuator{}, nil, nil, nil, t0)
	s.Results[CatSticks] = Result{Category: CatSticks, Status: Passed, Summary: "ok", Measurements: []Measurement{{"x", 1, "%"}}}
	r := BuildReport(s, "0.6.0", false, t0)

	var buf bytes.Buffer
	if err := r.WriteJSON(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "AA:BB:CC") {
		t.Fatalf("unique identifiers must be excluded by default")
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("report must be valid JSON: %v", err)
	}
	for _, want := range []string{`"status": "Passed"`, `"firmware": "0458"`, `"supported_tests"`, `"connection": "USB-C Wired"`} {
		if !strings.Contains(out, want) {
			t.Errorf("report JSON missing %s", want)
		}
	}
	withIDs := BuildReport(s, "0.6.0", true, t0)
	if withIDs.Device.Identifiers["mac"] != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("explicit opt-in should include identifiers")
	}

	jp, tp, err := r.Export(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	txt, _ := os.ReadFile(tp)
	if _, err := os.Stat(jp); err != nil || !strings.Contains(string(txt), "Analog çubuklar") {
		t.Fatalf("export should write JSON and text reports")
	}
}

func TestBatteryNeverFabricatesHealth(t *testing.T) {
	b := NewBatteryMonitor(10, nil)
	for i, pct := range []int{85, 85, 75} {
		b.Feed(device.Telemetry{BatteryPercent: pct}, t0.Add(time.Duration(i)*time.Minute))
	}
	r := b.Result(t0.Add(3*time.Minute), false)
	joined := strings.Join(r.Details, "\n")
	if !strings.Contains(joined, HealthUnavailable) || len(b.Observations) != 2 {
		t.Fatalf("health must be reported as unavailable and only changes observed: %v", r.Details)
	}
	for _, m := range r.Measurements {
		if strings.Contains(strings.ToLower(m.Name), "health") || strings.Contains(strings.ToLower(m.Name), "cycle") {
			t.Fatalf("fabricated measurement: %s", m.Name)
		}
	}
}

func TestConnectionRateAndInterruptions(t *testing.T) {
	c := NewConnMonitor(t0)
	info := device.Info{Connected: true, Connection: device.ConnUSB}
	for i := 0; i <= 100; i++ {
		now := t0.Add(time.Duration(i) * 40 * time.Millisecond)
		c.Feed(device.Telemetry{PacketsRx: uint64(i * 10), UpdatedAt: now}, info, now)
	}
	if math.Abs(c.RateHz-250) > 5 {
		t.Fatalf("expected ~250 Hz, got %.1f", c.RateHz)
	}
	r := c.Result(t0.Add(4*time.Second), false)
	if r == nil || r.Status != Passed {
		t.Fatalf("stable connection should pass: %+v", r)
	}
	c.Feed(device.Telemetry{}, device.Info{Connected: false}, t0.Add(5*time.Second))
	c.Feed(device.Telemetry{PacketsRx: 2000, ReportsDropped: 3}, info, t0.Add(6*time.Second))
	r = c.Result(t0.Add(7*time.Second), false)
	if c.Interruptions != 1 || c.Reconnections != 1 || r.Status != Warning {
		t.Fatalf("interruption and drops should be reported, got %+v", r)
	}
}
