// Package winpad resolves which controller model is behind a Windows input
// device, and lists HID game controllers that XInput cannot see.
//
// Identity rules (shared by the XInput and HID backends):
//   - Evidence is the HID identity Windows reports: vendor/product/version
//     IDs and USB strings, matched by the same model matchers as on Linux.
//   - A model name, brand and 3D model are shown only when the match is
//     exact (Identified) or the user confirmed it. A probable match (e.g. the
//     shared Xbox 360 ID 045e:028e) stays a generic controller and is only
//     offered for confirmation.
//   - Nothing about a model is ever assumed from the button layout, the
//     XInput API or a generic Windows device name.
package winpad

import (
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/system/winhid"
)

// Identity is the identification outcome for one controller.
type Identity struct {
	HID        winhid.Device // identity evidence; zero when there is none
	HasHID     bool
	Resolution gamepad.Resolution
	// Note explains a missing identity ("" when HID evidence exists).
	Note string
}

// Resolve identifies one HID game controller.
func Resolve(models []gamepad.Model, d winhid.Device) Identity {
	return Identity{HID: d, HasHID: true, Resolution: gamepad.Resolve(models, d.AsInputDevice())}
}

// ForXInput identifies XInput slots. XInput does not say which HID entry
// belongs to which slot, so the identity is used only when it is the same for
// every slot: as many XInput-class HID controllers as connected slots, all
// with one vendor/product/version. Two identical controllers therefore stay
// identified (and separately selectable by slot); a mix stays unidentified.
func ForXInput(models []gamepad.Model, all []winhid.Device, slots int) Identity {
	var xi []winhid.Device
	for _, d := range winhid.GameControllers(all) {
		if d.XInput() {
			xi = append(xi, d)
		}
	}
	switch {
	case len(xi) == 0:
		return Identity{Note: "Windows lists no XInput-class HID entry for this controller, so its vendor and product IDs are unknown."}
	case len(xi) != slots:
		return Identity{Note: "The number of XInput slots and XInput-class HID entries differs; Windows does not link them, so the model is not inferred."}
	}
	for _, d := range xi[1:] {
		if d.HardwareKey() != xi[0].HardwareKey() {
			return Identity{Note: "Different XInput controllers are connected; Windows does not say which slot is which, so no model is inferred."}
		}
	}
	return Resolve(models, xi[0])
}

// Exact returns the model when it is identified or confirmed by the user.
func (id Identity) Exact() *gamepad.Model {
	r := id.Resolution
	if r.Model != nil && (r.Confidence == gamepad.Identified || r.Confidence == gamepad.Confirmed) {
		return r.Model
	}
	return nil
}

// Endpoint changes whenever the resolved identity changes, so the session
// re-opens the device with the new descriptor (e.g. after a confirmation).
func (id Identity) Endpoint() string {
	model := device.GenericGamepadModelID
	if m := id.Exact(); m != nil {
		model = m.ID
	}
	if !id.HasHID {
		return model
	}
	return id.HID.HardwareKey() + "#" + model
}

// Apply writes the identity into a backend's descriptor and info. Only an
// exact identity changes the name, brand and visual; the backend's own
// capabilities stay authoritative and every other physical feature of the
// model is declared inaccessible with note (never as working).
func (id Identity) Apply(desc *device.Descriptor, info *device.Info, why device.Requirement, note string) {
	if id.HasHID {
		info.VendorID, info.ProductID = id.HID.VendorID, id.HID.ProductID
		info.IdentityKey = id.HID.HardwareKey()
		if id.HID.Product != "" {
			info.Name = id.HID.Product // raw name from the device, kept apart from the model registry
		}
		desc.VendorID, desc.ProductIDs = id.HID.VendorID, []uint16{id.HID.ProductID}
	}
	r := id.Resolution
	info.Identification = r.Reason
	switch {
	case !id.HasHID:
		info.IdentityState, info.Identification = "unidentified", id.Note
	case r.Confidence == gamepad.Identified:
		info.IdentityState = "identified"
	case r.Confidence == gamepad.Confirmed:
		info.IdentityState = "confirmed"
	case r.Confidence == gamepad.Probable:
		info.IdentityState = "probable"
		info.IdentityCandidateID, info.IdentityCandidateName = r.Model.ID, r.Model.Static.DisplayName
	default:
		info.IdentityState, info.Identification = "unidentified", "Not identified as a known model"
		if r.Candidate != nil {
			info.IdentityCandidateID, info.IdentityCandidateName = r.Candidate.ID, r.Candidate.Static.DisplayName
		}
	}
	m := id.Exact()
	if m == nil {
		info.Model = desc.DisplayName
		return
	}
	s := m.Static
	desc.ModelID, desc.Family, desc.Brand = s.ModelID, s.Family, s.Brand
	desc.DisplayName, desc.ShortName, desc.VisualID, desc.Accent = s.DisplayName, s.ShortName, s.VisualID, s.Accent
	desc.ModeGuide = s.ModeGuide
	desc.Capabilities = desc.Capabilities.WithModelHardware(s.Capabilities, why, note)
	info.Model = s.DisplayName
}
