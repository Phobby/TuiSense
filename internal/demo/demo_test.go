package demo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/ui/visual"
)

func TestRegistryHasTenSimulatedControllersAndNoDriver(t *testing.T) {
	r := NewRegistry()
	if len(r.Drivers()) != 0 {
		t.Fatal("the web demo must not register any hardware driver")
	}
	vs := r.Virtuals()
	if len(vs) != 10 || len(Models()) != 10 {
		t.Fatalf("expected 10 simulated controllers, got %d", len(vs))
	}
	seen := map[string]bool{}
	for i, v := range vs {
		if v.Source != device.SourceSimulated || !v.AutoAttach || !IsDemoKey(v.Key) || v.Key != Models()[i].Key {
			t.Fatalf("unexpected virtual %+v", v)
		}
		if seen[v.Descriptor.VisualID] {
			t.Fatalf("visual %s used twice", v.Descriptor.VisualID)
		}
		seen[v.Descriptor.VisualID] = true
		if _, ok := visual.Spec(v.Descriptor.VisualID); !ok {
			t.Fatalf("%s: no 3D model %q", v.Key, v.Descriptor.VisualID)
		}
	}
}

func TestSimulatedDevicesNeverNameARealNode(t *testing.T) {
	for _, v := range NewRegistry().Virtuals() {
		d := v.New(nil)
		info := d.Info()
		for _, p := range []string{info.HIDRawPath, info.InputDevicePath, info.SysfsPath} {
			if strings.HasPrefix(p, "/dev/") || strings.HasPrefix(p, "/sys/") {
				t.Fatalf("%s names a host path %q", v.Key, p)
			}
		}
		tel, err := d.Poll(context.Background())
		if err != nil || !tel.Simulated {
			t.Fatalf("%s: poll must return simulated telemetry (%v)", v.Key, err)
		}
		if !d.Capabilities().Supports(device.CapBattery) && tel.BatteryPercent >= 0 {
			t.Fatalf("%s reports a battery level without a battery", v.Key)
		}
		_ = d.Close()
	}
}

func TestScenariosDriveEveryDeclaredButton(t *testing.T) {
	for _, v := range NewRegistry().Virtuals() {
		d := v.New(nil)
		sim, ok := d.(device.ScenarioSimulator)
		if !ok {
			t.Fatalf("%s cannot switch input scenarios", v.Key)
		}
		sim.SetInputScenario(device.ScenarioButtons)
		pressed := device.ButtonSet(0)
		deadline := time.Now().Add(400 * time.Millisecond)
		for time.Now().Before(deadline) {
			tel, _ := d.Poll(context.Background())
			pressed |= tel.Buttons
			time.Sleep(20 * time.Millisecond)
		}
		if pressed == 0 {
			t.Fatalf("%s: the button scenario pressed nothing", v.Key)
		}
	}
}

func TestPadCommandsStayInMemory(t *testing.T) {
	for _, v := range NewRegistry().Virtuals() {
		pad, ok := v.New(nil).(*Pad)
		if !ok {
			continue
		}
		err := pad.ExecuteCommand(context.Background(), device.Command{Type: device.CmdSetTrigger})
		if err == nil {
			t.Fatalf("%s accepted an adaptive-trigger command", v.Key)
		}
		if err := pad.ExecuteCommand(context.Background(), device.Command{Type: device.CmdPulseHaptics, PulseLeft: 10, PulseRight: 10}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(pad.Telemetry().LastOutputHex, "simulated") {
			t.Fatalf("%s: output must be labelled simulated", v.Key)
		}
	}
}

func TestHostViewIsFixedAndRefusesCommands(t *testing.T) {
	var env CompatEnv
	if env.GOOS() != "linux" || env.Exists("/etc/passwd") || env.ModuleLoaded("xpad") || env.Writable("/tmp") {
		t.Fatal("the demo host view must never reflect the server")
	}
	if _, err := env.LookPath("sh"); err == nil {
		t.Fatal("no program is ever found")
	}
	if err := (Runner{}).Run([]string{"/bin/true"}); err != ErrDisabled {
		t.Fatal("the runner must refuse")
	}
}
