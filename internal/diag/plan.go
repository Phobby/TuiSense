package diag

import (
	"github.com/Phobby/TuiSense/internal/audio"
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
)

// AudioProbe is the result of looking for the controller's own audio endpoint.
type AudioProbe struct {
	Endpoint audio.Endpoint
	Found    bool
	Err      error
}

// DeviceContext is everything the planner needs about the active device.
type DeviceContext struct {
	Key       string
	Desc      device.Descriptor
	Caps      device.CapabilitySet
	Info      device.Info
	Simulated bool
	Audio     AudioProbe
}

// Plan decides, per category, whether a test is available, unsupported by the
// model, or unavailable right now. It is purely capability/descriptor driven.
func Plan(dc DeviceContext) map[Category]Availability {
	in := dc.Desc.Inputs
	caps := dc.Caps
	out := map[Category]Availability{}
	set := func(c Category, st Status, reason string) {
		out[c] = Availability{Category: c, Status: st, Reason: reason}
	}
	need := func(c Category, ok bool, reason string) {
		if ok {
			set(c, NotTested, "")
		} else {
			set(c, Unsupported, reason)
		}
	}

	need(CatDeadzone, in.HasSticks() && caps.Supports(device.CapSticks), "This controller exposes no stick input for a deadzone preview.")
	need(CatFirmware, caps.Supports(device.CapFirmwareInfo), "Firmware information is not exposed by this controller.")
	need(CatSticks, in.HasSticks() && caps.Supports(device.CapSticks), i18n.T("This controller has no analog sticks."))
	need(CatButtons, len(in.Buttons) > 0, i18n.T("No buttons are declared for this controller."))
	need(CatTriggers, in.HasTriggers(), i18n.T("This controller has no analog triggers."))
	need(CatTouchpad, in.TouchpadContacts > 0, i18n.T("This controller has no touchpad."))
	need(CatMotion, caps.Supports(device.CapMotion), noteOr(caps, device.CapMotion, i18n.T("This controller has no motion sensors.")))
	need(CatHaptics, caps.Supports(device.CapHaptics), noteOr(caps, device.CapHaptics, i18n.T("This controller has no vibration motors.")))
	need(CatBattery, caps.Supports(device.CapBattery), i18n.T("This controller reports no battery telemetry (wired-only or unsupported)."))
	set(CatConnection, NotTested, "")

	audioCat := func(c Category, feats []device.Feature, wantCapture bool) {
		has := false
		for _, f := range feats {
			has = has || caps.HasFeature(device.CapAudio, f)
		}
		if !has {
			// Over Bluetooth the runtime set may have dropped the feature; explain that precisely.
			if base := dc.Desc.Capabilities; base.Supports(device.CapAudio) && dc.Info.Connection == device.ConnBluetooth && in.Audio.RequiresUSB {
				set(c, Unavailable, i18n.T("Controller audio is only exposed as a USB audio device; connect it with a USB cable."))
				return
			}
			set(c, Unsupported, map[bool]string{true: i18n.T("This controller has no microphone."), false: i18n.T("This controller has no audio output.")}[wantCapture])
			return
		}
		switch {
		case !dc.Info.Connected:
			set(c, Unavailable, i18n.T("Controller is disconnected."))
		case dc.Info.Connection == device.ConnBluetooth && in.Audio.RequiresUSB:
			set(c, Unavailable, i18n.T("Controller audio is only exposed as a USB audio device; connect it with a USB cable."))
		case dc.Audio.Err != nil:
			set(c, Unavailable, i18n.Tf("Audio system error: %s", dc.Audio.Err.Error()))
		case !dc.Audio.Found:
			set(c, Unavailable, i18n.Tf("No sound card belonging to this controller was found (looked for USB ID %s).", joinIDs(in.Audio.USBIDs)))
		case wantCapture && !dc.Audio.Endpoint.CanCapture():
			set(c, Unavailable, i18n.Tf("%s exposes no capture stream.", dc.Audio.Endpoint.CardName))
		case !wantCapture && !dc.Audio.Endpoint.CanPlay():
			set(c, Unavailable, i18n.Tf("%s exposes no playback stream.", dc.Audio.Endpoint.CardName))
		default:
			set(c, NotTested, "")
		}
	}
	audioCat(CatAudioIn, []device.Feature{device.FeatMicrophone}, true)
	audioCat(CatAudioOut, []device.Feature{device.FeatSpeaker, device.FeatHeadset}, false)

	if !dc.Info.Connected {
		for c, a := range out {
			if a.Status == NotTested && c != CatConnection {
				set(c, Unavailable, i18n.T("Controller is disconnected."))
			}
		}
	}
	if out[CatSticks].Available() {
		for i := 0; i < 4; i++ {
			if !in.StickAxis(i).Valid() {
				set(CatSticks, Unavailable, "Axis mapping unavailable: a declared stick range is invalid.")
			}
		}
	}
	if out[CatTriggers].Available() && in.TriggerKind != device.TriggerDigital {
		for i := 0; i < 2; i++ {
			if !in.TriggerAxis(i).Valid() {
				set(CatTriggers, Unavailable, "Axis mapping unavailable: a declared trigger range is invalid.")
			}
		}
	}
	return out
}

func noteOr(caps device.CapabilitySet, c device.Capability, def string) string {
	if n := caps.Spec(c).Note; n != "" {
		return i18n.T(n)
	}
	return def
}

func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return i18n.T("(none declared)")
	}
	out := ids[0]
	for _, id := range ids[1:] {
		out += ", " + id
	}
	return out
}
