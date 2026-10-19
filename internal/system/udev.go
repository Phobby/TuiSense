package system

import "fmt"

// UdevRulesContent contains the recommended udev rules for rootless DualSense access on Linux.
const UdevRulesContent = `# Sony DualSense Wireless Controller (USB & Bluetooth)
KERNEL=="hidraw*", ATTRS{idVendor}=="054c", ATTRS{idProduct}=="0ce6", MODE="0660", TAG+="uaccess"
KERNEL=="hidraw*", KERNELS=="*054C:0CE6*", MODE="0660", TAG+="uaccess"

# Sony DualSense Edge Wireless Controller (USB & Bluetooth)
KERNEL=="hidraw*", ATTRS{idVendor}=="054c", ATTRS{idProduct}=="0df2", MODE="0660", TAG+="uaccess"
KERNEL=="hidraw*", KERNELS=="*054C:0DF2*", MODE="0660", TAG+="uaccess"
`

// PermissionExplanation returns an actionable remediation message when /dev/hidraw*
// denies read or write access to the current unprivileged user.
func PermissionExplanation(devPath string) string {
	return fmt.Sprintf(
		"Permission denied on %s. Install udev rules: sudo cp contrib/70-tuisense-dualsense.rules /etc/udev/rules.d/ && sudo udevadm control --reload-rules && sudo udevadm trigger",
		devPath,
	)
}
