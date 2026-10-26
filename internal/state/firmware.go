package state

import (
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/firmware"
)

// FirmwareSection selects the part of the Device Details panel that is shown.
type FirmwareSection string

const (
	FirmwareStatus  FirmwareSection = "status"
	FirmwareGuide   FirmwareSection = "guide"
	FirmwareLinks   FirmwareSection = "links"
	FirmwareDetails FirmwareSection = "details"
)

// FirmwareView is the Device Details panel's snapshot for the active device:
// identification data plus the read-only firmware update check. It is filled
// by the application on the render goroutine; views only read it.
type FirmwareView struct {
	// Connected is false when no controller is attached.
	Connected bool
	// Supported is true when a firmware update check exists for the device.
	Supported bool
	Checking  bool
	Installed string           // installed version as read from the device ("" = unknown)
	Result    *firmware.Result // latest result in this session
	Last      *firmware.Record // last successful check (persisted)
	Guide     firmware.Guide
	Section   FirmwareSection
	// DetailGroup selects the group shown in Advanced details.
	DetailGroup device.DetailGroup
	// SelectedDetail is the key of the reading explained below the list.
	SelectedDetail string
	AutoCheck      bool
	// Details are the device's identification readings with their origin.
	Details []device.Detail
}

// Detail returns the reading with the given key.
func (v FirmwareView) Detail(key string) (device.Detail, bool) {
	for _, d := range v.Details {
		if d.Key == key {
			return d, true
		}
	}
	return device.Detail{}, false
}
