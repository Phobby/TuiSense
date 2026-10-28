package compat

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/system"
)

// UdevRulePath is where TuiSense's own DualSense hidraw rule is installed.
const UdevRulePath = "/etc/udev/rules.d/70-tuisense-dualsense.rules"

// ActionUdevDualSense installs TuiSense's DualSense hidraw access rule.
const ActionUdevDualSense = "install_dualsense_udev"

// Manual action IDs.
const (
	ActionInputGroup  = "manual_input_group"
	ActionXpadneo     = "manual_xpadneo"
	ActionEliteBPF    = "manual_elite_hid_bpf"
	ActionKernelXpad  = "manual_kernel_xpad"
	ActionUdevManual  = "manual_dualsense_udev"
	staging           = "tuisense-udev-"
	stagingSuffix     = ".rules"
	uinputPath        = "/dev/uinput"
	dualSenseFamily   = "dualsense"
	xboxFamily        = "xbox"
	xboxEliteFamily   = "xbox_elite"
	razerFamily       = "razer_wolverine_v3"
	xpadneoProjectURL = "https://github.com/atar-axis/xpadneo"
	hidBPFProjectURL  = "https://gitlab.freedesktop.org/libevdev/udev-hid-bpf"
)

func linuxChecks(env Env, in Input, r *Report) {
	caps, info, desc := in.Caps, in.Info, in.Desc
	r.Backend = caps.Spec(device.CapConnection).Backend
	if info.KernelDriver != "" {
		r.Backend += " · " + info.KernelDriver
	}

	// 1. Which OS driver serves the controller.
	if info.KernelDriver != "" {
		c := Check{ID: "os_driver", Title: "Operating system driver", Level: LevelOK, Detail: "Linux driver", Value: info.KernelDriver}
		if len(desc.LinuxDrivers) > 0 && !has(desc.LinuxDrivers, info.KernelDriver) {
			c.Level = LevelWarning
			c.Detail = "Unexpected driver; this controller is normally served by another one"
			c.Value = info.KernelDriver + " → " + strings.Join(desc.LinuxDrivers, ", ")
			if desc.Family == razerFamily {
				c.ActionID = ActionKernelXpad
				r.Actions = append(r.Actions, kernelXpadAction())
			}
		}
		r.Checks = append(r.Checks, c)
	}

	// 2. Input and output access.
	switch {
	case info.PermissionDenied && desc.Family == dualSenseFamily:
		r.Input = "No permission"
		r.Checks = append(r.Checks, Check{ID: "input", Title: "Input", Level: LevelMissing, ActionID: udevActionID(env),
			Detail: "TuiSense cannot open the controller's hidraw node. A udev rule grants the logged-in user access."})
		r.Actions = append(r.Actions, udevActions(env)...)
	case info.PermissionDenied:
		r.Input = "No permission"
		r.Checks = append(r.Checks, Check{ID: "input", Title: "Input", Level: LevelMissing, ActionID: ActionInputGroup,
			Detail: "TuiSense cannot read the controller's input node.", Value: info.InputDevicePath})
		r.Actions = append(r.Actions, inputGroupAction())
	default:
		r.Input = "Working"
		r.Checks = append(r.Checks, Check{ID: "input", Title: "Input", Level: LevelOK, Detail: "Working"})
	}
	if desc.Family == dualSenseFamily && info.ReadOnly && !info.PermissionDenied {
		r.Checks = append(r.Checks, Check{ID: "output", Title: "Output reports", Level: LevelMissing, ActionID: udevActionID(env),
			Affects: []device.Capability{device.CapLighting, device.CapHaptics, device.CapAdaptiveTriggers, device.CapAudio},
			Detail:  "The hidraw node is read-only, so lighting, vibration and adaptive triggers cannot be written."})
		if _, dup := r.Action(udevActionID(env)); !dup {
			r.Actions = append(r.Actions, udevActions(env)...)
		}
	}

	// 3. Vibration.
	switch hap := caps.Spec(device.CapHaptics); {
	case hap.Accessible():
		r.Checks = append(r.Checks, Check{ID: "vibration", Title: "Vibration", Level: LevelOK, Detail: "Working"})
	case hap.Requires == device.RequiresPermission:
		r.Checks = append(r.Checks, Check{ID: "vibration", Title: "Vibration", Level: LevelWarning, ActionID: ActionInputGroup,
			Detail: hap.Note, Affects: []device.Capability{device.CapHaptics}})
		if _, dup := r.Action(ActionInputGroup); !dup {
			r.Actions = append(r.Actions, inputGroupAction())
		}
	case hap.Requires == device.RequiresDriver:
		c := Check{ID: "vibration", Title: "Vibration", Level: LevelWarning, Detail: hap.Note, Affects: []device.Capability{device.CapHaptics}}
		if (desc.Family == xboxFamily || desc.Family == xboxEliteFamily) && info.Connection == device.ConnBluetooth {
			c.ActionID = ActionXpadneo
			r.Actions = append(r.Actions, xpadneoAction(env))
		}
		r.Checks = append(r.Checks, c)
	}

	// 4. Features present but out of reach (one line each, only if they exist).
	for _, item := range []struct {
		id    string
		title string
		c     device.Capability
	}{
		{"battery", "Battery", device.CapBattery},
		{"audio", "Audio", device.CapAudio},
		{"rear", "Rear buttons and paddles", device.CapRearControls},
	} {
		spec := caps.Spec(item.c)
		if spec.Physical() != device.Present || spec.Accessible() || spec.Presence != device.Present {
			continue
		}
		c := Check{ID: item.id, Title: item.title, Level: LevelInfo, Detail: spec.Note, Affects: []device.Capability{item.c}}
		if item.c == device.CapRearControls && desc.Family == xboxEliteFamily {
			c.Level = LevelWarning
			c.ActionID = ActionEliteBPF
			r.Actions = append(r.Actions, eliteAction(info.Connection))
		}
		r.Checks = append(r.Checks, c)
	}

	// 5. Hardware TuiSense does not implement yet, as one line.
	var missing []string
	for _, c := range []device.Capability{device.CapMotion, device.CapTouchpad, device.CapLighting, device.CapProfiles, device.CapRemapping} {
		if s := caps.Spec(c); s.Presence == device.Present && !s.Accessible() && s.Requires == device.RequiresBackend {
			missing = append(missing, c.Label())
		}
	}
	if len(missing) > 0 {
		r.Checks = append(r.Checks, Check{ID: "not_implemented", Title: "Not implemented by TuiSense", Level: LevelInfo,
			Detail: "The hardware has these features; TuiSense does not drive them yet and never sends unverified vendor commands.", Value: strings.Join(missing, ", ")})
	}

	// 6. Official firmware route (instructions only).
	if fw := caps.Spec(device.CapFirmwareUpdate); fw.Presence == device.Present && fw.Note != "" {
		r.Checks = append(r.Checks, Check{ID: "firmware", Title: "Firmware updates", Level: LevelInfo, Detail: fw.Note})
	}

	// 7. Software input processing (deadzones/remapping that games see).
	detail := "TuiSense does not implement a software input-processing backend; stick and deadzone settings are previews."
	if env.Writable(uinputPath) {
		detail = "TuiSense does not implement a software input-processing backend; stick and deadzone settings are previews. /dev/uinput is accessible on this system."
	}
	r.Checks = append(r.Checks, Check{ID: "software_input", Title: "Software input processing", Level: LevelInfo, Detail: detail})
}

func udevActionID(env Env) string {
	if _, err := env.LookPath("pkexec"); err != nil {
		return ActionUdevManual
	}
	return ActionUdevDualSense
}

// udevActions offers TuiSense's own, reviewed udev rule: executable through
// polkit when pkexec exists, otherwise as manual commands.
func udevActions(env Env) []Action {
	if env.Exists(UdevRulePath) {
		return []Action{{ID: ActionUdevManual, Title: "Reconnect the controller", Manual: true,
			Explanation:  "TuiSense's udev rule is already installed.",
			Instructions: []string{"Unplug and reconnect the controller (or re-pair it over Bluetooth) so udev applies the rule."}}}
	}
	manual := Action{ID: ActionUdevManual, Title: "Install the DualSense udev rule (manual)", Manual: true, Reconnect: true,
		Explanation: "Grants the logged-in user access to DualSense hidraw nodes (uaccess).",
		Changes:     []string{"Creates " + UdevRulePath},
		Instructions: []string{
			"tuisense --print-udev | sudo tee " + UdevRulePath,
			"sudo udevadm control --reload-rules",
			"Reconnect the controller.",
		},
		Rollback: []string{"sudo rm " + UdevRulePath, "sudo udevadm control --reload-rules"}}
	if _, err := env.LookPath("pkexec"); err != nil {
		return []Action{manual}
	}
	return []Action{{
		ID: ActionUdevDualSense, Title: "Install the DualSense udev rule", Privileged: true, Reconnect: true,
		Explanation: "Grants the logged-in user access to DualSense hidraw nodes (uaccess). You will be asked for your password by polkit.",
		Changes:     []string{"Creates " + UdevRulePath + " (the rule printed by tuisense --print-udev)", "Reloads udev rules"},
		Commands: [][]string{
			{"pkexec", "install", "-m", "0644", StagedRulePath(), UdevRulePath},
			{"pkexec", "udevadm", "control", "--reload-rules"},
		},
		Rollback: []string{"sudo rm " + UdevRulePath, "sudo udevadm control --reload-rules"},
	}}
}

// StagedRulePath is the user-owned file the rule is copied from.
func StagedRulePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "tuisense", staging+"dualsense"+stagingSuffix)
}

// StageRule writes the reviewed rule content for the privileged copy.
func StageRule() error {
	p := StagedRulePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(system.UdevRulesContent), 0o600)
}

// allowlist validates every command of an executable action.
var allowlist = map[string][]func([]string) bool{
	ActionUdevDualSense: {
		func(a []string) bool {
			if len(a) != 6 || a[0] != "pkexec" || a[1] != "install" || a[2] != "-m" || a[3] != "0644" || a[5] != UdevRulePath {
				return false
			}
			// The staged file must be TuiSense's own rule, byte for byte.
			if a[4] != StagedRulePath() {
				return false
			}
			b, err := os.ReadFile(a[4])
			return err == nil && string(b) == system.UdevRulesContent
		},
		func(a []string) bool {
			return len(a) == 4 && a[0] == "pkexec" && a[1] == "udevadm" && a[2] == "control" && a[3] == "--reload-rules"
		},
	},
}

func inputGroupAction() Action {
	return Action{ID: ActionInputGroup, Title: "Grant access to game controllers (manual)", Manual: true,
		Explanation: "Desktop sessions normally grant controller access to the logged-in user (systemd-logind uaccess). Remote or minimal sessions may need the 'input' group.",
		Changes:     []string{"Adds your user to the 'input' group (affects every input device)"},
		Instructions: []string{
			"sudo usermod -aG input $USER",
			"Log out and back in.",
		},
		Rollback: []string{"sudo gpasswd -d $USER input"}}
}

func xpadneoAction(env Env) Action {
	a := Action{ID: ActionXpadneo, Title: "Optional: xpadneo for Xbox controllers over Bluetooth (manual)", Manual: true,
		Explanation: "Only if rumble or battery is missing over Bluetooth. xpadneo is an out-of-tree kernel driver that replaces hid-microsoft for Xbox pads. TuiSense never installs kernel modules.",
		Changes:     []string{"Installs a DKMS kernel module (hid-xpadneo)"},
		Instructions: []string{
			"Prefer your distribution's package (e.g. xpadneo-dkms) if it exists.",
			"Otherwise follow the official instructions at " + xpadneoProjectURL + " (DKMS install script from a reviewed checkout, not a piped download).",
			"Reconnect the controller.",
		},
		Rollback: []string{"Uninstall with the same package manager, or run the project's uninstall script."},
		Links:    []string{xpadneoProjectURL}, Reconnect: true}
	if env.ModuleLoaded("hid_xpadneo") {
		a.Explanation = "xpadneo is already loaded; reconnect the controller so it binds."
		a.Instructions = []string{"Reconnect the controller."}
		a.Changes = nil
	}
	return a
}

func eliteAction(conn device.ConnectionType) Action {
	if conn == device.ConnBluetooth {
		return Action{ID: ActionEliteBPF, Title: "Elite Series 2 paddles over Bluetooth (manual)", Manual: true,
			Explanation:  "Over Bluetooth the paddles are reported only with the kernel's HID-BPF fix for this controller (in-tree in recent kernels, or from the udev-hid-bpf project).",
			Instructions: []string{"Update to a kernel that includes the Xbox Elite 2 HID-BPF fix, or install udev-hid-bpf from your distribution or " + hidBPFProjectURL + ".", "Reconnect the controller."},
			Links:        []string{hidBPFProjectURL}, Reconnect: true}
	}
	return Action{ID: ActionEliteBPF, Title: "Elite Series 2 paddles over USB (manual)", Manual: true,
		Explanation:  "Linux xpad reports the four paddles since Linux 6.1. On older kernels, or while an on-board profile remaps the paddles, they are not reported separately.",
		Instructions: []string{"Use Linux 6.1 or newer.", "Select a profile without paddle mappings (profile button) and reconnect."}, Reconnect: true}
}

func kernelXpadAction() Action {
	return Action{ID: ActionKernelXpad, Title: "Kernel support for this controller (manual)", Manual: true,
		Explanation:  "Linux xpad gained this controller's IDs in Linux 7.0. Older kernels may bind a generic driver.",
		Instructions: []string{"Update to Linux 7.0 or newer through your distribution, then reconnect the controller."}}
}
