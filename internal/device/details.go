package device

import "fmt"

// DetailsOf returns a device's identification details: its own when it
// implements DetailsProvider, otherwise the generic facts every driver
// reports through Info. Nothing is inferred.
func DetailsOf(dev Device) []Detail {
	if dev == nil {
		return nil
	}
	if p, ok := dev.(DetailsProvider); ok {
		if d := p.Details(); len(d) > 0 {
			return d
		}
	}
	return BasicDetails(dev.Descriptor(), dev.Info())
}

// BasicDetails builds details from the generic descriptor and Info fields.
func BasicDetails(desc Descriptor, info Info) []Detail {
	var out []Detail
	add := func(g DetailGroup, key, label, value, origin string, adv bool) {
		if value != "" {
			out = append(out, Detail{Group: g, Key: key, Label: label, Value: value, Origin: origin, Advanced: adv})
		}
	}
	add(DetailDevice, "model", "Model", nonEmptyStr(desc.DisplayName, info.Model), "driver", false)
	add(DetailDevice, "manufacturer", "Manufacturer", desc.Brand, "driver", false)
	if info.VendorID != 0 || info.ProductID != 0 {
		add(DetailDevice, "usb_id", "USB VID:PID", fmt.Sprintf("%04X:%04X", info.VendorID, info.ProductID), "device discovery", false)
	}
	add(DetailDevice, "connection", "Connection", string(info.Connection), "driver", false)
	add(DetailDevice, "hidraw", "HID device", nonEmptyStr(info.HIDRawPath, info.InputDevicePath), "device discovery", false)
	add(DetailFirmware, "fw_version", "Firmware version", info.FirmwareVersion, "driver", false)
	add(DetailFirmware, "fw_build", "Firmware build", info.FirmwareBuild, "driver", false)
	add(DetailHardware, "hw_version", "Hardware version", info.HardwareVersion, "driver", false)
	add(DetailDriver, "driver", "Kernel driver", info.KernelDriver, "sysfs", false)
	add(DetailDriver, "sysfs", "sysfs path", info.SysfsPath, "device discovery", true)
	return out
}

func nonEmptyStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
