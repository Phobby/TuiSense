package device

import (
	"errors"
	"fmt"
)

// Scope is what a configuration operation actually changes.
type Scope string

const (
	ScopeDevice   Scope = "device"   // written to the controller hardware
	ScopeSoftware Scope = "software" // changes input through a software backend
	ScopePreview  Scope = "preview"  // TuiSense-only; games and hardware are unaffected
	ScopeTest     Scope = "test"     // a momentary diagnostic effect
)

// Operation declares one command: what it needs and what it changes. The
// table below is the single source of truth used both to decide what the UI
// offers and to refuse anything else before it reaches a backend.
type Operation struct {
	Command    CommandType
	Capability Capability
	Feature    Feature // optional sub-feature
	// Needs is the access that permits the command (any bit suffices).
	Needs      Access
	Scope      Scope
	Persistent bool // survives a disconnect
	Revertible bool // TuiSense can restore the previous state
	Summary    string
}

// Operations lists every command TuiSense can send. CmdApplyFullConfig is
// handled specially: backends apply only the sections their capabilities allow.
var Operations = map[CommandType]Operation{
	CmdSetLighting: {Command: CmdSetLighting, Capability: CapLighting, Needs: AccessConfigDevice, Scope: ScopeDevice, Revertible: true,
		Summary: "Light bar / LED colour and brightness"},
	CmdSetMicLED: {Command: CmdSetMicLED, Capability: CapLighting, Feature: FeatMicLED, Needs: AccessConfigDevice, Scope: ScopeDevice, Revertible: true,
		Summary: "Microphone LED"},
	CmdSetTrigger: {Command: CmdSetTrigger, Capability: CapAdaptiveTriggers, Needs: AccessConfigDevice | AccessTest, Scope: ScopeDevice, Revertible: true,
		Summary: "Adaptive trigger resistance"},
	CmdSetHaptics: {Command: CmdSetHaptics, Capability: CapHaptics, Needs: AccessConfigDevice | AccessTest, Scope: ScopeDevice, Revertible: true,
		Summary: "Vibration motor intensities (or stopping a vibration test)"},
	CmdPulseHaptics: {Command: CmdPulseHaptics, Capability: CapHaptics, Needs: AccessTest | AccessConfigDevice, Scope: ScopeTest, Revertible: true,
		Summary: "Short vibration test pulse"},
	CmdSetSpeakerVol: {Command: CmdSetSpeakerVol, Capability: CapAudio, Needs: AccessConfigDevice, Scope: ScopeDevice, Revertible: true,
		Summary: "Speaker, headset and microphone levels"},
	CmdApplyFullConfig: {Command: CmdApplyFullConfig, Scope: ScopeDevice, Revertible: true,
		Summary: "Profile: every supported device setting at once"},
}

// ErrOperationNotSupported is returned when a command is not permitted by
// the device's current capabilities (or is unknown).
var ErrOperationNotSupported = errors.New("operation not supported by this controller")

// Authorize checks a command against a device's current (connection-aware)
// capabilities. It is enforced in the command path, not only in the UI.
func Authorize(caps CapabilitySet, cmd Command) error {
	op, ok := Operations[cmd.Type]
	if !ok {
		return fmt.Errorf("%w: unknown command %q", ErrOperationNotSupported, cmd.Type)
	}
	if op.Capability == "" {
		return nil
	}
	spec := caps.Spec(op.Capability)
	if !spec.Accessible() || spec.Access&op.Needs == 0 {
		return fmt.Errorf("%w: %s needs %s", ErrOperationNotSupported, cmd.Type, op.Capability)
	}
	if op.Feature != "" && !spec.HasFeature(op.Feature) {
		return fmt.Errorf("%w: %s needs feature %s", ErrOperationNotSupported, cmd.Type, op.Feature)
	}
	return nil
}

// SettingScope reports how a capability's settings take effect, for labelling
// panels: device writes win over software processing over previews.
func SettingScope(caps CapabilitySet, c Capability) (Scope, bool) {
	s := caps.Spec(c)
	switch {
	case !s.Accessible():
		return "", false
	case s.Access.Has(AccessConfigDevice):
		return ScopeDevice, true
	case s.Access.Has(AccessConfigSoftware):
		return ScopeSoftware, true
	case s.Access.Has(AccessPreview):
		return ScopePreview, true
	}
	return "", false
}
