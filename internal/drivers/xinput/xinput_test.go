package xinput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/ui/visual"
)

type fakeAPI struct {
	states  map[uint32]State
	battery map[uint32][2]uint8
	sets    [][3]uint32
}

func (f *fakeAPI) GetState(slot uint32) (State, error) {
	if s, ok := f.states[slot]; ok {
		return s, nil
	}
	return State{}, ErrNotConnected
}
func (f *fakeAPI) SetState(slot uint32, l, r uint16) error {
	f.sets = append(f.sets, [3]uint32{slot, uint32(l), uint32(r)})
	return nil
}
func (f *fakeAPI) Battery(slot uint32) (uint8, uint8, error) {
	if b, ok := f.battery[slot]; ok {
		return b[0], b[1], nil
	}
	return BatteryUnknown, 0, errors.New("no info")
}

func TestDiscoverAndReadInput(t *testing.T) {
	api := &fakeAPI{states: map[uint32]State{
		0: {Packet: 7, Gamepad: Gamepad{Buttons: 0x1000 | 0x0001 | 0x0100, LeftTrigger: 255, ThumbLX: 32767, ThumbLY: 32767}},
		2: {},
	}, battery: map[uint32][2]uint8{0: {BatteryNiMH, 2}, 2: {BatteryWired, 0}}}
	d := NewDriverWith(api)
	cands, err := d.Discover()
	if err != nil || len(cands) != 2 || cands[0].Key != "xinput:0" || cands[1].Key != "xinput:2" {
		t.Fatalf("expected slots 1 and 3: %+v %v", cands, err)
	}
	dev, err := d.Open(cands[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	tel, err := dev.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !tel.Buttons.Has(device.ButtonSouth) || !tel.Buttons.Has(device.ButtonDpadUp) || !tel.Buttons.Has(device.ButtonL1) || tel.Buttons.Has(device.ButtonEast) {
		t.Fatalf("buttons %b", tel.Buttons)
	}
	// XInput Y up is positive; TuiSense uses negative Y = up.
	if tel.LeftStickX < 0.99 || tel.LeftStickY > -0.99 || tel.L2Pressure != 1 {
		t.Fatalf("axes %+v", tel)
	}
	if !dev.Capabilities().Supports(device.CapBattery) || tel.BatteryPercent != 60 || dev.Info().Connection != device.ConnWireless {
		t.Fatalf("wireless pads report battery: %d %s", tel.BatteryPercent, dev.Info().Connection)
	}
	wired, _ := d.Open(cands[1], nil)
	if wired.Capabilities().Supports(device.CapBattery) || wired.Info().Connection != device.ConnUSBWired {
		t.Fatal("wired pads have no battery capability")
	}
	for _, c := range []device.Capability{device.CapLighting, device.CapMotion, device.CapAdaptiveTriggers, device.CapAudio, device.CapRearControls} {
		if dev.Capabilities().Supports(c) {
			t.Errorf("XInput cannot provide %s", c)
		}
	}
	delete(api.states, 0)
	if _, err := dev.Poll(context.Background()); err == nil || dev.Info().Connected {
		t.Fatal("unplugging must disconnect")
	}
}

func TestRumbleIsBoundedAndStops(t *testing.T) {
	api := &fakeAPI{states: map[uint32]State{0: {}}}
	dev, _ := NewDriverWith(api).Open(device.Candidate{Key: "xinput:0", Payload: uint32(0)}, nil)
	if err := dev.ExecuteCommand(context.Background(), device.Command{Type: device.CmdPulseHaptics, PulseLeft: 255, PulseRight: 128, PulseDur: time.Millisecond, PulseAbsolute: true}); err != nil {
		t.Fatal(err)
	}
	if api.sets[0] != [3]uint32{0, 65535, 32896} {
		t.Fatalf("pulse %v", api.sets[0])
	}
	time.Sleep(2 * time.Millisecond)
	_, _ = dev.Poll(context.Background())
	if last := api.sets[len(api.sets)-1]; last[1] != 0 || last[2] != 0 {
		t.Fatal("an expired pulse must stop the motors")
	}
	_ = dev.Close()
	if last := api.sets[len(api.sets)-1]; last[1] != 0 {
		t.Fatal("closing stops the motors")
	}
	// Commands without a capability never reach this backend (command gate).
	if err := device.Authorize(dev.Capabilities(), device.Command{Type: device.CmdSetTrigger}); !errors.Is(err, device.ErrOperationNotSupported) {
		t.Fatal("adaptive triggers must be refused")
	}
}

func TestNoXInputMeansNoDevices(t *testing.T) {
	if c, err := NewDriverWith(nil).Discover(); c != nil || err != nil {
		t.Fatal("without XInput nothing is discovered")
	}
}

// The generic visual has a component for every region and button.
func TestDescriptorMatchesItsVisual(t *testing.T) {
	d := Descriptor(true)
	spec, ok := visual.Spec(d.VisualID)
	if !ok {
		t.Fatal("visual missing")
	}
	regions, buttons := map[device.RegionID]bool{}, map[device.ButtonID]bool{}
	for _, c := range spec.Components {
		regions[c.Region] = true
		if c.Bound {
			buttons[c.Button] = true
		}
	}
	for _, r := range d.Regions {
		if !regions[r.ID] {
			t.Errorf("region %s has no component", r.ID)
		}
	}
	for _, b := range d.Inputs.Buttons {
		if !buttons[b.ID] {
			t.Errorf("button %s has no component", b.Label)
		}
	}
}
