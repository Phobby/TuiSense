package gamepad

import (
	"sync"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// Confidence is how sure a model match is.
type Confidence int

const (
	NoMatch    Confidence = iota
	Probable              // indirect evidence (e.g. a community-documented VID/PID/version)
	Confirmed             // the user confirmed the model for this hardware key
	Identified            // the device names the model itself (USB vendor/product strings)
)

// Model identifies one controller model among evdev gamepads and describes it.
type Model struct {
	ID string // descriptor ModelID
	// Static is the descriptor listed in the registry before any device is seen.
	Static device.Descriptor
	// Identify inspects a discovered device. It must not open the device.
	Identify func(evdev.InputDevice) (Confidence, string)
	// Describe builds the runtime descriptor for the observed mapping and
	// what the backend could access.
	Describe func(m Mapping, d evdev.InputDevice, rt Runtime) device.Descriptor
	// ModeHint explains the observed input mode in model terms ("" = generic text).
	ModeHint func(m Mapping, d evdev.InputDevice) string
	// Spec is the data-only description when the model was built from one.
	Spec *ModelSpec
}

// GenericModelID is the descriptor ID of unidentified gamepads. Storing it as
// an identity override means "this is not one of the known models".
const GenericModelID = device.GenericGamepadModelID

// overrides remembers the user's model confirmations by hardware key.
var overrides = struct {
	sync.RWMutex
	m map[string]string
}{m: map[string]string{}}

// SetOverrides replaces all remembered confirmations (hardware key → model ID).
func SetOverrides(m map[string]string) {
	overrides.Lock()
	defer overrides.Unlock()
	overrides.m = map[string]string{}
	for k, v := range m {
		overrides.m[k] = v
	}
}

// SetOverride remembers (or with modelID "" forgets) one confirmation.
func SetOverride(hwKey, modelID string) {
	overrides.Lock()
	defer overrides.Unlock()
	if modelID == "" {
		delete(overrides.m, hwKey)
		return
	}
	overrides.m[hwKey] = modelID
}

// Overrides returns a copy of the remembered confirmations.
func Overrides() map[string]string {
	overrides.RLock()
	defer overrides.RUnlock()
	out := make(map[string]string, len(overrides.m))
	for k, v := range overrides.m {
		out[k] = v
	}
	return out
}

func override(hwKey string) (string, bool) {
	overrides.RLock()
	defer overrides.RUnlock()
	v, ok := overrides.m[hwKey]
	return v, ok
}

// Resolution is the identification outcome for one device.
type Resolution struct {
	Model      *Model // nil = generic
	Confidence Confidence
	Reason     string
	// Candidate is a model the device might be (offered for confirmation)
	// when it was not identified; nil when there is nothing to offer.
	Candidate *Model
}

// Resolve picks the best model for d, honoring the user's confirmations.
func Resolve(models []Model, d evdev.InputDevice) Resolution {
	ov, hasOv := override(d.HardwareKey())
	var res Resolution
	for i := range models {
		m := &models[i]
		conf, reason := m.Identify(d)
		if hasOv {
			switch ov {
			case m.ID:
				conf, reason = Confirmed, "Confirmed by you for this USB ID"
			case GenericModelID:
				if conf < Identified {
					conf = NoMatch
				}
			}
		}
		if conf > res.Confidence {
			res = Resolution{Model: m, Confidence: conf, Reason: reason}
		}
	}
	if res.Model == nil && len(models) > 0 && !(hasOv && ov == GenericModelID) {
		res.Candidate = &models[0]
	}
	return res
}
