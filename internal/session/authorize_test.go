package session

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
)

// batterylessPad is a recording device with MSI-like capabilities: sticks
// and rumble only — no lighting, adaptive triggers or audio.
type batterylessPad struct {
	*recordingDevice
	caps device.CapabilitySet
}

func (b *batterylessPad) Capabilities() device.CapabilitySet { return b.caps }

// Unsupported commands are refused in the command path itself: the backend
// never sees them, whatever the UI shows.
func TestDispatcherRefusesUnsupportedOperations(t *testing.T) {
	drv := &fakeDriver{}
	drv.set("hw:pad")
	m := NewManager(newRegistry(drv), Options{}, nil, nil)
	hw := m.Device("hw:pad").(*recordingDevice)
	pad := &batterylessPad{recordingDevice: hw, caps: device.NewCapabilitySet(
		device.CapabilitySpec{ID: device.CapSticks, Support: device.Limited, Access: device.AccessRead | device.AccessTest},
		device.CapabilitySpec{ID: device.CapHaptics, Support: device.Limited, Access: device.AccessTest, Features: []device.Feature{device.FeatTestPulse}},
		device.AbsentSpec(device.CapAdaptiveTriggers), device.AbsentSpec(device.CapLighting), device.AbsentSpec(device.CapBattery),
	)}
	m.mu.Lock()
	m.findLocked("hw:pad").dev = pad
	m.mu.Unlock()

	var mu sync.Mutex
	results := map[device.CommandType]error{}
	d := NewDispatcher(m, nil, func(cmd device.Command, err error) {
		mu.Lock()
		results[cmd.Type] = err
		mu.Unlock()
	})
	l := device.DefaultConfig().Lighting
	for _, cmd := range []device.Command{
		{Type: device.CmdSetTrigger, Config: device.DefaultConfig()},
		{Type: device.CmdSetLighting, Lighting: &l},
		{Type: device.CmdSetSpeakerVol},
		{Type: device.CmdPulseHaptics, PulseLeft: 10, PulseRight: 10},
	} {
		d.Dispatch(cmd)
	}
	d.flushBatch(context.Background())

	hw.mu.Lock()
	defer hw.mu.Unlock()
	if len(hw.cmds) != 1 || hw.cmds[0] != device.CmdPulseHaptics {
		t.Fatalf("only the supported rumble pulse may reach the backend, got %v", hw.cmds)
	}
	for _, c := range []device.CommandType{device.CmdSetTrigger, device.CmdSetLighting, device.CmdSetSpeakerVol} {
		if !errors.Is(results[c], device.ErrOperationNotSupported) {
			t.Errorf("%s: expected ErrOperationNotSupported, got %v", c, results[c])
		}
	}
	if results[device.CmdPulseHaptics] != nil {
		t.Fatalf("supported pulse failed: %v", results[device.CmdPulseHaptics])
	}
}
